package scope_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"

	_ "modernc.org/sqlite" // driver for the migration test

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

func newService(t *testing.T) *scope.Service {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { root.Close() })

	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return scope.New(func() *store.DB { return db })
}

func TestServiceCRUD(t *testing.T) {
	t.Parallel()

	s, ctx := newService(t), t.Context()

	p, err := s.Create(ctx, scope.Preset{Name: " Main ", Allow: []string{"*.a.com"}})
	if err != nil || p.Name != "Main" || p.ID == 0 {
		t.Fatalf("Create = %+v, %v", p, err)
	}

	if _, err := s.Create(ctx, scope.Preset{Name: "Main"}); apperr.CodeOf(err) != apperr.Conflict {
		t.Errorf("duplicate name err = %v", err)
	}

	if _, err := s.Create(ctx, scope.Preset{Name: "Bad", Allow: []string{"a.com:0"}}); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("bad entry err = %v", err)
	}

	p.Deny = []string{"x.a.com"}
	if _, err := s.Update(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get(ctx, p.ID)
	if err != nil || !slices.Equal(got.Deny, []string{"x.a.com"}) || !slices.Equal(got.Allow, p.Allow) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	m, err := got.Matcher()
	if err != nil || m.InScope("x.a.com", 80, "/") || !m.InScope("y.a.com", 80, "/") {
		t.Errorf("matcher wrong: %v", err)
	}

	d1, err := s.Duplicate(ctx, p.ID)
	if err != nil || d1.Name != "Main (copy)" {
		t.Fatalf("Duplicate = %+v, %v", d1, err)
	}

	if d2, _ := s.Duplicate(ctx, p.ID); d2.Name != "Main (copy 2)" {
		t.Errorf("second duplicate = %q", d2.Name)
	}

	if l, _ := s.List(ctx); len(l) != 3 {
		t.Errorf("List len = %d", len(l))
	}

	if err := s.Delete(ctx, d1.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, d1.ID); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("delete twice err = %v", err)
	}

	if _, err := s.Update(ctx, scope.Preset{ID: 999, Name: "n"}); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("update missing err = %v", err)
	}
}

func TestReorderAndAppend(t *testing.T) {
	t.Parallel()

	s := newService(t)
	ctx := t.Context()

	var ids []int64

	for _, n := range []string{"a", "b", "c"} {
		p, err := s.Create(ctx, scope.Preset{Name: n})
		if err != nil {
			t.Fatal(err)
		}

		ids = append(ids, p.ID)
	}

	if err := s.Reorder(ctx, []int64{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatal(err)
	}

	d, _ := s.Create(ctx, scope.Preset{Name: "d"})

	got, _ := s.List(ctx)
	if len(got) != 4 || got[0].ID != ids[2] || got[1].ID != ids[0] || got[3].ID != d.ID {
		t.Fatalf("order = %+v", got)
	}
}

// Existing rows keep their id order when 0002 adds sort_order.
func TestMigration0002KeepsOrder(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	for _, f := range []string{"0001_init.sql", "", "0002_scope_sort_order.sql"} {
		if f == "" {
			_, err = db.Exec(`INSERT INTO scope_presets(id, name) VALUES (5, 'e'), (2, 'b'), (9, 'i')`)
		} else {
			var b []byte
			if b, err = os.ReadFile(filepath.Join("..", "store", "migrations", "project", f)); err == nil {
				_, err = db.Exec(string(b))
			}
		}

		if err != nil {
			t.Fatal(f, err)
		}
	}

	rows, err := db.Query(`SELECT id FROM scope_presets ORDER BY sort_order, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}

		got = append(got, id)
	}

	if !slices.Equal(got, []int{2, 5, 9}) {
		t.Fatalf("order = %v", got)
	}
}
