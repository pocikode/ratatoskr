package environment_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/environment"
	"github.com/pocikode/ratatoskr/internal/secret"
	"github.com/pocikode/ratatoskr/internal/store"
)

func openRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func openProject(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.OpenProject(t.Context(), openRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func setup(t *testing.T) (*environment.Service, *store.DB, *store.DB) {
	t.Helper()
	root := openRoot(t)
	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inst.Close() })
	box, _, err := secret.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	proj := openProject(t)
	return environment.New(environment.Deps{Instance: inst, Project: func() *store.DB { return proj }, Secrets: box}), inst, proj
}

func requireCode(t *testing.T, err error, code apperr.Code) {
	t.Helper()
	if err == nil || apperr.CodeOf(err) != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func TestCRUDAndNames(t *testing.T) {
	t.Parallel()
	s, _, _ := setup(t)
	ctx := t.Context()
	es, err := s.List(ctx)
	if err != nil || len(es) != 1 || !es[0].IsGlobal || es[0].Selected || es[0].Variables == nil {
		t.Fatalf("initial list = %+v, %v", es, err)
	}
	b, err := s.Create(ctx, " Zulu ")
	if err != nil || b.Name != "Zulu" {
		t.Fatalf("create = %+v, %v", b, err)
	}
	a, err := s.Create(ctx, "Alpha")
	if err != nil {
		t.Fatal(err)
	}
	es, err = s.List(ctx)
	if err != nil || len(es) != 3 || es[0].Name != "Global" || es[1].ID != a.ID || es[2].ID != b.ID {
		t.Fatalf("list = %+v, %v", es, err)
	}
	for _, name := range []string{"", " \t ", strings.Repeat("a", 201), "a\nb", "a\x00b"} {
		_, err := s.Create(ctx, name)
		requireCode(t, err, apperr.BadInput)
	}
	for _, name := range []string{"Global", " Global ", "Alpha"} {
		_, err := s.Create(ctx, name)
		requireCode(t, err, apperr.Conflict)
	}
	if _, err := s.Create(ctx, strings.Repeat("界", 200)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(ctx, a.ID, " Alpha "); err != nil {
		t.Fatal(err)
	}
	_, err = s.Rename(ctx, a.ID, "Zulu")
	requireCode(t, err, apperr.Conflict)
	_, err = s.Rename(ctx, 0, "Other")
	requireCode(t, err, apperr.Conflict)
	requireCode(t, s.Delete(ctx, 0), apperr.Conflict)
	_, err = s.Rename(ctx, 999, "Other")
	requireCode(t, err, apperr.NotFound)
	requireCode(t, s.Delete(ctx, 999), apperr.NotFound)
	if err := s.Delete(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(ctx, b.ID)
	requireCode(t, err, apperr.NotFound)
}

func TestVariablesSealedOrderedAndAtomic(t *testing.T) {
	t.Parallel()
	s, inst, proj := setup(t)
	ctx := t.Context()
	e, err := s.Create(ctx, "Dev")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{0, e.ID} {
		t.Run(map[bool]string{true: "global", false: "project"}[id == 0], func(t *testing.T) {
			vars := []environment.Variable{{Name: "z", Value: "super-secret-value", Secret: true}, {Name: "a", Value: "visible"}}
			if _, err := s.SetVariables(ctx, id, vars); err != nil {
				t.Fatal(err)
			}
			got, err := s.Get(ctx, id)
			if err != nil || !reflect.DeepEqual(got.Variables, vars) {
				t.Fatalf("get = %+v, %v", got, err)
			}
			db, table, filter := proj, "env_vars", " WHERE env_id = ?"
			args := []any{id}
			if id == 0 {
				db, table, filter, args = inst, "env_global", "", nil
			}
			rows, err := db.Reader().QueryContext(ctx, `SELECT name,value FROM `+table+filter, args...)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var name, value string
				if err := rows.Scan(&name, &value); err != nil {
					t.Fatal(err)
				}
				if name == "z" && strings.Contains(value, vars[0].Value) {
					t.Fatal("plaintext secret in raw row")
				}
				if name == "a" && value != "visible" {
					t.Fatal("plain value was sealed")
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			for _, bad := range [][]environment.Variable{
				{{Name: ""}}, {{Name: "1bad"}}, {{Name: "bad name"}}, {{Name: "é"}}, {{Name: strings.Repeat("a", 101)}},
				{{Name: "dup"}, {Name: "dup"}}, {{Name: "huge", Value: strings.Repeat("x", (1<<20)+1)}},
			} {
				_, err := s.SetVariables(ctx, id, bad)
				requireCode(t, err, apperr.BadInput)
				if !strings.Contains(err.Error(), bad[len(bad)-1].Name) {
					t.Fatalf("error does not name variable: %v", err)
				}
			}
			// Fail after the delete and first insert to verify transaction rollback.
			if err := db.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, `CREATE TRIGGER fail_env BEFORE INSERT ON `+table+` WHEN NEW.name = 'fail' BEGIN SELECT RAISE(ABORT,'test failure'); END`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SetVariables(ctx, id, []environment.Variable{{Name: "first"}, {Name: "fail"}}); err == nil {
				t.Fatal("trigger did not fail")
			}
			got, err = s.Get(ctx, id)
			if err != nil || !reflect.DeepEqual(got.Variables, vars) {
				t.Fatalf("failed replace changed values: %+v, %v", got, err)
			}
			if _, err := s.SetVariables(ctx, id, []environment.Variable{{Name: "z", Value: "now plain"}, {Name: "_.-09", Value: strings.Repeat("x", 1<<20)}, {Name: strings.Repeat("n", 100)}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SetVariables(ctx, id, nil); err != nil {
				t.Fatal(err)
			}
			got, err = s.Get(ctx, id)
			if err != nil || len(got.Variables) != 0 || got.Variables == nil {
				t.Fatalf("clear = %+v, %v", got, err)
			}
		})
	}
	_, err = s.SetVariables(ctx, 999, nil)
	requireCode(t, err, apperr.NotFound)
}

func TestSelectionAndProjectIsolation(t *testing.T) {
	t.Parallel()
	_, inst, p1 := setup(t)
	p2 := openProject(t)
	current := p1
	// Reopen the same instance secret box, as T-109 will do for app wiring.
	root, err := os.OpenRoot(strings.TrimSuffix(inst.Path(), "/instance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	box, _, err := secret.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	s := environment.New(environment.Deps{Instance: inst, Project: func() *store.DB { return current }, Secrets: box})
	ctx := t.Context()
	if _, err := s.SetVariables(ctx, 0, []environment.Variable{{Name: "a", Value: "global"}, {Name: "b", Value: "fallback"}}); err != nil {
		t.Fatal(err)
	}
	e, err := s.Create(ctx, "Dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetVariables(ctx, e.ID, []environment.Variable{{Name: "a", Value: "project", Secret: true}}); err != nil {
		t.Fatal(err)
	}
	selected, err := s.Select(ctx, &e.ID)
	if err != nil || selected == nil || !selected.Selected {
		t.Fatalf("select = %+v, %v", selected, err)
	}
	if vars, err := s.Vars(ctx); err != nil || vars["a"] != "project" || vars["b"] != "fallback" {
		t.Fatalf("vars = %+v, %v", vars, err)
	}
	other, err := s.Create(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(ctx, &other.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, e.ID)
	if err != nil || got.Selected {
		t.Fatalf("old selection = %+v, %v", got, err)
	}
	bad, global := int64(999), int64(0)
	_, err = s.Select(ctx, &bad)
	requireCode(t, err, apperr.NotFound)
	_, err = s.Select(ctx, &global)
	requireCode(t, err, apperr.BadInput)
	got, err = s.Get(ctx, other.ID)
	if err != nil || !got.Selected {
		t.Fatal("invalid selection cleared current selection")
	}
	current = p2
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Global" {
		t.Fatalf("second project = %+v, %v", list, err)
	}
	if vars, err := s.Vars(ctx); err != nil || vars["a"] != "global" {
		t.Fatalf("second project vars = %+v, %v", vars, err)
	}
	if _, err := s.Create(ctx, "Dev"); err != nil {
		t.Fatal(err)
	}
	current = p1
	if err := s.Delete(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if vars, err := s.Vars(ctx); err != nil || vars["a"] != "global" {
		t.Fatalf("delete selected vars = %+v, %v", vars, err)
	}
	if _, err := s.Select(ctx, &e.ID); err != nil {
		t.Fatal(err)
	}
	if selected, err := s.Select(ctx, nil); err != nil || selected != nil {
		t.Fatalf("deselect = %+v, %v", selected, err)
	}
	if vars, err := s.Vars(ctx); err != nil || vars["a"] != "global" {
		t.Fatalf("deselected vars = %+v, %v", vars, err)
	}
}

func TestConcurrentCreateAndSelection(t *testing.T) {
	t.Parallel()
	s, _, _ := setup(t)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Go(func() { _, err := s.Create(t.Context(), "same"); results <- err })
	}
	wg.Wait()
	close(results)
	created := 0
	for err := range results {
		if err == nil {
			created++
		} else {
			requireCode(t, err, apperr.Conflict)
		}
	}
	if created != 1 {
		t.Fatalf("created %d duplicate environments", created)
	}
	a, err := s.Create(t.Context(), "A")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(t.Context(), "B")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		id := a.ID
		if i%2 == 0 {
			id = b.ID
		}
		wg.Go(func() {
			if _, err := s.Select(t.Context(), &id); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	es, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range es {
		if e.Selected {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("selected count = %d", n)
	}
}

func TestStorageErrorsPropagate(t *testing.T) {
	t.Parallel()
	s, _, proj := setup(t)
	e, err := s.Create(t.Context(), "Dev")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetVariables(t.Context(), e.ID, []environment.Variable{{Name: "token", Value: "secret", Secret: true}}); err != nil {
		t.Fatal(err)
	}
	if err := proj.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE env_vars SET value = 'broken'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), e.ID); err == nil {
		t.Fatal("corrupted secret accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestInterpolate(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"a": "A", "b": "B", "nested": "{{b}}", "empty": "", "a.b-c_0": "valid"}
	for _, tc := range []struct{ in, want string }{
		{"{{a}}", "A"}, {"{{ a }}", "A"}, {"{{\t a \r\n}}", "A"},
		{"{{unknown}}", "{{unknown}}"}, {"{{ unknown }}", "{{ unknown }}"},
		{"{{a}}{{b}}", "AB"}, {"{{{a}}}", "{A}"}, {"{{nested}}", "{{b}}"},
		{"\xff\x00{{a}}\xfe", "\xff\x00A\xfe"}, {"{{empty}}", ""}, {"{{a.b-c_0}}", "valid"},
		{"{{1bad}} {{a}", "{{1bad}} {{a}"}, {"{{a}}\x00{{missing}}", "A\x00{{missing}}"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			raw := []byte(tc.in)
			got := environment.Interpolate(raw, vars)
			if string(got) != tc.want || string(raw) != tc.in {
				t.Fatalf("got %q, want %q (input %q)", got, tc.want, raw)
			}
		})
	}
	if got := environment.Interpolate([]byte("{{a}}"), nil); string(got) != "{{a}}" {
		t.Fatalf("nil vars = %q", got)
	}
}
