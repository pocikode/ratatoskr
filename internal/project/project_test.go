package project_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/store"
)

type env struct {
	root *os.Root
	inst *store.DB
}

func newEnv(t *testing.T) *env {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { root.Close() })

	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { inst.Close() })

	return &env{root: root, inst: inst}
}

func (e *env) open(t *testing.T) *project.Service {
	t.Helper()

	s, err := project.Open(t.Context(), project.Deps{Instance: e.inst, Root: e.root})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { s.Close() })

	return s
}

func mustCode(t *testing.T, err error, want apperr.Code) {
	t.Helper()

	if got := apperr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %q), want code %q", err, got, want)
	}
}

func list(t *testing.T, s *project.Service) []project.Project {
	t.Helper()

	ps, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	return ps
}

func TestOpenCreatesTemporaryProject(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)

	cur, err := s.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if !cur.Temporary || !cur.Selected || cur.Default || s.DB() == nil {
		t.Errorf("current = %+v, db = %v", cur, s.DB())
	}

	if cur.DBSize == 0 {
		t.Error("DBSize = 0 for an opened project")
	}
}

func TestOpenSelectsDefault(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t)

	p, err := s.Create(t.Context(), "  Work ", false)
	if err != nil {
		t.Fatal(err)
	}

	if p.Name != "Work" {
		t.Errorf("name = %q, want trimmed", p.Name)
	}

	if _, err := s.SetDefault(t.Context(), &p.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2 := e.open(t)

	cur, err := s2.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if cur.ID != p.ID || cur.Temporary {
		t.Errorf("current = %+v, want default %d", cur, p.ID)
	}

	if n := len(list(t, s2)); n != 1 {
		t.Errorf("%d projects after restart, want 1 (temp removed)", n)
	}
}

func TestCreateRenameValidation(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)

	for _, name := range []string{"", "   ", "a\x00b", string(make([]rune, 201))} {
		if _, err := s.Create(t.Context(), name, false); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("Create(%q) = %v, want BAD_INPUT", name, err)
		}
	}

	_, err := s.Rename(t.Context(), 9999, "x")
	mustCode(t, err, apperr.NotFound)

	p, err := s.Create(t.Context(), "a", false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Rename(t.Context(), p.ID, " "); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("Rename blank = %v", err)
	}

	got, err := s.Rename(t.Context(), p.ID, "b")
	if err != nil || got.Name != "b" {
		t.Errorf("Rename = %+v, %v", got, err)
	}
}

func TestRenameSavesTemporary(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t)

	cur, _ := s.Current(t.Context())

	saved, err := s.Rename(t.Context(), cur.ID, "Kept")
	if err != nil {
		t.Fatal(err)
	}

	if saved.Temporary {
		t.Error("still temporary after rename")
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	ps := list(t, e.open(t))
	if len(ps) != 2 { // the saved one plus the fresh temp of the second open
		t.Fatalf("projects = %+v", ps)
	}
}

func TestTemporaryRemovedOnClose(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t)

	cur, _ := s.Current(t.Context())
	dir := filepath.Join("projects", itoa(cur.ID))

	if _, err := e.root.Stat(dir); err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := e.root.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("temp dir still there: %v", err)
	}

	var n int
	if err := e.inst.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM projects`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows = %d, %v", n, err)
	}
}

func TestOpenPurgesCrashLeftovers(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t) // temp project, never closed cleanly

	cur, _ := s.Current(t.Context())

	if err := e.root.MkdirAll("projects/777", 0o700); err != nil { // orphan dir
		t.Fatal(err)
	}

	// Simulate a crash: drop the handle without Close.
	if err := s.DB().Close(); err != nil {
		t.Fatal(err)
	}

	s2 := e.open(t)

	if _, err := e.root.Stat("projects/777"); !os.IsNotExist(err) {
		t.Errorf("orphan dir kept: %v", err)
	}

	// IDs may be reused, so check the registry holds only the fresh temp project.
	if ps := list(t, s2); len(ps) != 1 || !ps[0].Temporary || !ps[0].Selected {
		t.Errorf("projects = %+v, cur was %d", ps, cur.ID)
	}
}

func TestSelectClosesOldHandle(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)

	old := s.DB()

	p, err := s.Create(t.Context(), "next", false)
	if err != nil {
		t.Fatal(err)
	}

	sel, err := s.Select(t.Context(), p.ID)
	if err != nil || !sel.Selected || sel.LastOpenedAt == nil {
		t.Fatalf("Select = %+v, %v", sel, err)
	}

	if s.DB() == old || s.DB() == nil {
		t.Fatal("handle not swapped")
	}

	if err := old.Reader().PingContext(t.Context()); err == nil {
		t.Error("old reader pool still open")
	}

	if err := old.Write(t.Context(), func(*sql.Tx) error { return nil }); err == nil {
		t.Error("old writer still open")
	}

	if err := s.DB().Reader().PingContext(t.Context()); err != nil {
		t.Errorf("new handle: %v", err)
	}

	_, err = s.Select(t.Context(), 9999)
	mustCode(t, err, apperr.NotFound)
}

func TestSelectSameIsNoop(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)
	ch := s.Subscribe(t.Context())
	cur, _ := s.Current(t.Context())
	db := s.DB()

	if _, err := s.Select(t.Context(), cur.ID); err != nil {
		t.Fatal(err)
	}

	if s.DB() != db {
		t.Error("handle replaced")
	}

	select {
	case c := <-ch:
		t.Errorf("unexpected event %+v", c)
	default:
	}
}

func TestListOrderAndSetDefault(t *testing.T) {
	t.Parallel()

	var tick time.Time

	e := newEnv(t)
	now := func() time.Time { tick = tick.Add(time.Second); return tick.Add(time.Hour) }

	s, err := project.Open(t.Context(), project.Deps{Instance: e.inst, Root: e.root, Now: now})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { s.Close() })

	a, _ := s.Create(t.Context(), "a", false)
	b, _ := s.Create(t.Context(), "b", false)

	if _, err := s.Select(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}

	ps := list(t, s)
	if ps[0].ID != a.ID || ps[1].ID != b.ID {
		t.Errorf("order = %v, %v; want a then b first", ps[0].Name, ps[1].Name)
	}

	got, err := s.SetDefault(t.Context(), &a.ID)
	if err != nil || got == nil || !got.Default {
		t.Fatalf("SetDefault a = %+v, %v", got, err)
	}

	if got, err = s.SetDefault(t.Context(), &b.ID); err != nil || !got.Default {
		t.Fatalf("SetDefault b = %+v, %v", got, err)
	}

	n := 0

	for _, p := range list(t, s) {
		if p.Default {
			n++

			if p.ID != b.ID {
				t.Errorf("default = %d, want b", p.ID)
			}
		}
	}

	if n != 1 {
		t.Errorf("%d defaults", n)
	}

	if got, err = s.SetDefault(t.Context(), nil); err != nil || got != nil {
		t.Errorf("clear = %+v, %v", got, err)
	}

	for _, p := range list(t, s) {
		if p.Default {
			t.Errorf("%s still default", p.Name)
		}
	}

	tmp := firstTemp(t, s)
	_, err = s.SetDefault(t.Context(), &tmp.ID)
	mustCode(t, err, apperr.BadInput)

	missing := int64(9999)
	_, err = s.SetDefault(t.Context(), &missing)
	mustCode(t, err, apperr.NotFound)
}

func firstTemp(t *testing.T, s *project.Service) project.Project {
	t.Helper()

	for _, p := range list(t, s) {
		if p.Temporary {
			return p
		}
	}

	t.Fatal("no temporary project")

	return project.Project{}
}

func TestDeleteSelected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		deleteIsDef bool // the selected project is the default
		wantTemp    bool // fallback is a new temporary project
	}{
		{"falls back to the default", false, false},
		{"new temp when deleted one was default", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			s := e.open(t)

			def, _ := s.Create(t.Context(), "def", false)
			victim, _ := s.Create(t.Context(), "victim", true)

			target := def
			if tt.deleteIsDef {
				target = victim
			}

			if _, err := s.SetDefault(t.Context(), &target.ID); err != nil {
				t.Fatal(err)
			}

			ch := s.Subscribe(t.Context())

			if err := s.Delete(t.Context(), victim.ID); err != nil {
				t.Fatal(err)
			}

			if _, err := e.root.Stat(filepath.Join("projects", itoa(victim.ID))); !os.IsNotExist(err) {
				t.Errorf("directory kept: %v", err)
			}

			cur, err := s.Current(t.Context())
			if err != nil {
				t.Fatal(err)
			}

			if cur.ID == victim.ID || cur.Temporary != tt.wantTemp || (!tt.wantTemp && cur.ID != def.ID) {
				t.Errorf("current after delete = %+v", cur)
			}

			if s.DB() == nil || s.DB().Reader().PingContext(t.Context()) != nil {
				t.Error("no usable DB after delete")
			}

			var kinds []project.Kind
			for len(ch) > 0 {
				kinds = append(kinds, (<-ch).Kind)
			}

			last := kinds[len(kinds)-1]
			if last != project.Deleted {
				t.Errorf("events = %v, want trailing DELETED", kinds)
			}

			if err := s.Delete(t.Context(), victim.ID); apperr.CodeOf(err) != apperr.NotFound {
				t.Errorf("second delete = %v", err)
			}
		})
	}
}

func TestDeleteOnlyProjectCreatesTemp(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)
	cur, _ := s.Current(t.Context())

	if err := s.Delete(t.Context(), cur.ID); err != nil {
		t.Fatal(err)
	}

	next, err := s.Current(t.Context())
	if err != nil || next.ID == cur.ID || !next.Temporary {
		t.Errorf("current = %+v, %v", next, err)
	}
}

func TestDuplicate(t *testing.T) {
	t.Parallel()

	for _, selected := range []bool{true, false} {
		t.Run(map[bool]string{true: "selected source", false: "closed source"}[selected], func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			s := e.open(t)

			src, _ := s.Create(t.Context(), "Src", selected)

			// Data in the DB and an extra file.
			if selected {
				err := s.DB().Write(t.Context(), func(tx *sql.Tx) error {
					_, err := tx.ExecContext(t.Context(), `INSERT INTO ui_state (key, value_json) VALUES ('k', '"v"')`)
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}

			if err := e.root.MkdirAll(filepath.Join("projects", itoa(src.ID), "files"), 0o700); err != nil {
				t.Fatal(err)
			}

			if err := e.root.WriteFile(filepath.Join("projects", itoa(src.ID), "files", "a.txt"), []byte("hi"), 0o600); err != nil {
				t.Fatal(err)
			}

			cp, err := s.Duplicate(t.Context(), src.ID, "")
			if err != nil {
				t.Fatal(err)
			}

			if cp.Name != "Src (copy)" || cp.Selected || cp.Temporary || cp.ID == src.ID {
				t.Errorf("copy = %+v", cp)
			}

			b, err := e.root.ReadFile(filepath.Join("projects", itoa(cp.ID), "files", "a.txt"))
			if err != nil || string(b) != "hi" {
				t.Errorf("copied file = %q, %v", b, err)
			}

			if selected {
				// Open the copy and look for the row.
				if _, err := s.Select(t.Context(), cp.ID); err != nil {
					t.Fatal(err)
				}

				var v string
				err := s.DB().Reader().QueryRowContext(t.Context(), `SELECT value_json FROM ui_state WHERE key = 'k'`).Scan(&v)
				if err != nil || v != `"v"` {
					t.Errorf("copied row = %q, %v", v, err)
				}
			}

			named, err := s.Duplicate(t.Context(), src.ID, "Named")
			if err != nil || named.Name != "Named" {
				t.Errorf("named copy = %+v, %v", named, err)
			}

			_, err = s.Duplicate(t.Context(), 9999, "")
			mustCode(t, err, apperr.NotFound)
		})
	}
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)

	ctx, cancel := context.WithCancel(t.Context())
	ch := s.Subscribe(ctx)

	p, _ := s.Create(t.Context(), "x", false)
	if _, err := s.Rename(t.Context(), p.ID, "y"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Select(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}

	var kinds []project.Kind
	for len(ch) > 0 {
		c := <-ch
		kinds = append(kinds, c.Kind)

		if (c.Kind == project.Deleted) != (c.Project == nil) {
			t.Errorf("%s event project = %v", c.Kind, c.Project)
		}
	}

	want := []project.Kind{
		project.Created, project.Updated, project.Selected,
		project.Created, project.Selected, project.Deleted, // delete of the selected one: new temp, select, delete
	}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}

	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}

	cancel()

	if _, open := <-ch; open {
		t.Error("channel open after cancel")
	}
}

func TestSlowSubscriberIsClosed(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)
	slow := s.Subscribe(t.Context())
	cur, _ := s.Current(t.Context())

	for i := range 100 {
		if _, err := s.Rename(t.Context(), cur.ID, "n"+itoa(int64(i))); err != nil {
			t.Fatal(err)
		}
	}

	n := 0
	for range slow {
		n++
	}

	if n != 64 {
		t.Errorf("drained %d events, want 64 then close", n)
	}
}

func TestCloseEndsSubscriptions(t *testing.T) {
	t.Parallel()

	s := newEnv(t).open(t)
	ch := s.Subscribe(t.Context())

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if _, open := <-ch; open {
		t.Error("subscription survived Close")
	}

	if _, open := <-s.Subscribe(t.Context()); open {
		t.Error("Subscribe after Close is open")
	}

	if _, err := s.Create(t.Context(), "x", false); err == nil {
		t.Error("Create after Close succeeded")
	}

	if s.DB() != nil {
		t.Error("DB() after Close")
	}
}

func TestProjectDirsAreIsolated(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t)

	a, _ := s.Create(t.Context(), "a", true)
	b, _ := s.Create(t.Context(), "b", true)

	paths := map[int64]string{}

	for _, p := range []project.Project{a, b} {
		if _, err := e.root.Stat(filepath.Join("projects", itoa(p.ID), "project.db")); err != nil {
			t.Errorf("project %d: %v", p.ID, err)
		}

		paths[p.ID] = itoa(p.ID)
	}

	if len(paths) != 2 {
		t.Error("ids collide")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestOpenRecreatesMissingDefaultDir(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	s := e.open(t)

	p, err := s.Create(t.Context(), "Work", false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.SetDefault(t.Context(), &p.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if err := e.root.RemoveAll("projects/" + strconv.FormatInt(p.ID, 10)); err != nil {
		t.Fatal(err)
	}

	s2 := e.open(t)

	cur, err := s2.Current(t.Context())
	if err != nil || cur.ID != p.ID {
		t.Fatalf("current = %+v, err = %v; want project %d", cur, err, p.ID)
	}
}
