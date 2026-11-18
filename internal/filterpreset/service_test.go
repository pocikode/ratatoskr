package filterpreset_test

import (
	"os"
	"slices"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/store"
)

func newService(t *testing.T) *filterpreset.Service {
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

	return filterpreset.New(func() *store.DB { return db })
}

func TestValidation(t *testing.T) {
	t.Parallel()

	s, ctx := newService(t), t.Context()

	for _, tt := range []struct {
		name string
		p    filterpreset.Preset
		code apperr.Code
	}{
		{"empty name", filterpreset.Preset{Query: "row.id.gt:1"}, apperr.BadInput},
		{"empty query", filterpreset.Preset{Name: "x"}, apperr.BadInput},
		{"upper alias", filterpreset.Preset{Name: "x", Alias: "Ab", Query: "row.id.gt:1"}, apperr.BadInput},
		{"space alias", filterpreset.Preset{Name: "x", Alias: "a b", Query: "row.id.gt:1"}, apperr.BadInput},
		{"quote alias", filterpreset.Preset{Name: "x", Alias: `a"b`, Query: "row.id.gt:1"}, apperr.BadInput},
		{"comment alias", filterpreset.Preset{Name: "x", Alias: "a//b", Query: "row.id.gt:1"}, apperr.BadInput},
		{"bad query", filterpreset.Preset{Name: "x", Query: "req.nope.eq:1"}, apperr.BadQuery},
		{"unknown ref", filterpreset.Preset{Name: "x", Query: "preset:ghost"}, apperr.BadQuery},
		{"self ref", filterpreset.Preset{Name: "x", Alias: "xx", Query: "preset:xx"}, apperr.BadQuery},
	} {
		if _, err := s.Create(ctx, tt.p); apperr.CodeOf(err) != tt.code {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.code)
		}
	}
}

func TestCRUDAndCycles(t *testing.T) {
	t.Parallel()

	s, ctx := newService(t), t.Context()

	a, err := s.Create(ctx, filterpreset.Preset{Name: "Errors", Alias: "errs", Query: "resp.code.gte:500"})
	if err != nil {
		t.Fatal(err)
	}

	b, err := s.Create(ctx, filterpreset.Preset{Name: "Api errors", Query: `req.path.cont:"/api" AND preset:errs`})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Create(ctx, filterpreset.Preset{Name: "Errors", Query: "row.id.gt:1"}); apperr.CodeOf(err) != apperr.Conflict {
		t.Errorf("dup name err = %v", err)
	}

	if _, err := s.Create(ctx, filterpreset.Preset{Name: "Other", Alias: "errs", Query: "row.id.gt:1"}); apperr.CodeOf(err) != apperr.Conflict {
		t.Errorf("dup alias err = %v", err)
	}

	// Editing a so that it refers to b closes the loop a -> b -> a.
	a.Query = `preset:"Api errors"`
	if _, err := s.Update(ctx, a); apperr.CodeOf(err) != apperr.BadQuery {
		t.Errorf("cycle err = %v", err)
	}

	n, err := s.ExpandText(ctx, `preset:"Api errors" OR req.method.eq:"GET"`)
	if err != nil {
		t.Fatal(err)
	}

	if want := `req.path.cont:"/api" AND resp.code.gte:500 OR req.method.eq:"GET"`; n.String() != want {
		t.Errorf("expanded = %s, want %s", n, want)
	}

	if _, err := s.ExpandText(ctx, "preset:nope"); apperr.CodeOf(err) != apperr.BadQuery {
		t.Errorf("unknown err = %v", err)
	}

	if err := s.Reorder(ctx, []int64{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}

	l, _ := s.List(ctx)
	if len(l) != 2 || l[0].ID != b.ID {
		t.Errorf("order = %+v", l)
	}

	if err := s.Reorder(ctx, []int64{b.ID, b.ID}); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("bad reorder err = %v", err)
	}

	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, a.ID); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("delete twice err = %v", err)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	t.Parallel()

	src, ctx := newService(t), t.Context()

	for _, p := range []filterpreset.Preset{
		{Name: "Base", Alias: "base", Query: "resp.code.eq:200"},
		{Name: "Top", Query: `preset:base AND req.tls.eq:true`},
	} {
		if _, err := src.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	data, err := src.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dst := newService(t)

	n, err := dst.Import(ctx, data)
	if err != nil || n != 2 {
		t.Fatalf("Import = %d, %v", n, err)
	}

	got, _ := dst.Export(ctx)
	if string(got) != string(data) {
		t.Errorf("round trip differs:\n%s\n%s", data, got)
	}

	if n, err := dst.Import(ctx, data); err != nil || n != 0 { // all names exist: skipped
		t.Errorf("re-import = %d, %v", n, err)
	}

	for _, bad := range []string{
		`not json`,
		`{"version":2,"presets":[]}`,
		`{"version":1,"presets":[{"name":"A","alias":"a","query":"preset:a"}]}`,
		`{"version":1,"presets":[{"name":"A","alias":"a","query":"preset:b"},{"name":"B","alias":"b","query":"preset:a"}]}`,
	} {
		before, _ := dst.List(ctx)
		if _, err := dst.Import(ctx, []byte(bad)); err == nil {
			t.Errorf("Import(%s) succeeded", bad)
		}

		after, _ := dst.List(ctx)
		if !slices.Equal(before, after) {
			t.Errorf("failed import changed presets for %s", bad)
		}
	}

	fwd := `{"version":1,"presets":[{"name":"Dep","query":"preset:first"},{"name":"First","alias":"first","query":"row.id.gt:1"}]}`
	if n, err := dst.Import(ctx, []byte(fwd)); err != nil || n != 2 {
		t.Errorf("forward ref import = %d, %v", n, err)
	}
}

func TestImportIgnoresDanglingExisting(t *testing.T) {
	t.Parallel()

	s, ctx := newService(t), t.Context()

	a, err := s.Create(ctx, filterpreset.Preset{Name: "A", Alias: "a", Query: "row.id.gt:1"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Create(ctx, filterpreset.Preset{Name: "B", Query: "preset:a"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	n, err := s.Import(ctx, []byte(`{"version":1,"presets":[{"name":"C","query":"row.id.gt:2"}]}`))
	if err != nil || n != 1 {
		t.Fatalf("Import = %d, %v; want 1, nil", n, err)
	}
}
