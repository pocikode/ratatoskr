package scope_test

import (
	"os"
	"slices"
	"testing"

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
