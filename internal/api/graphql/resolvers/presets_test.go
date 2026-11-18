package resolvers_test

import (
	"os"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

func presetClient(t *testing.T, wired bool) *client.Client {
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

	proj, err := project.Open(t.Context(), project.Deps{Instance: inst, Root: root})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { proj.Close() })

	deps := resolvers.Deps{Projects: proj}
	if wired {
		deps.Scopes, deps.Filters = scope.New(proj.DB), filterpreset.New(proj.DB)
	}

	return client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(deps)}))
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("err = %v, want %s", err, code)
	}
}

func TestScopeResolvers(t *testing.T) {
	t.Parallel()

	c := presetClient(t, true)

	type sp struct {
		ID, Name string
		Allow    []string
		Deny     []string
	}

	var a, b struct{ CreateScopePreset sp }

	const create = `mutation($n: String!, $a: [String!]!) { createScopePreset(name: $n, allow: $a) { id name allow deny } }`
	if err := c.Post(create, &a, client.Var("n", " A "), client.Var("a", []string{"*.a.test"})); err != nil || a.CreateScopePreset.Name != "A" || len(a.CreateScopePreset.Deny) != 0 {
		t.Fatalf("create = %+v, %v", a, err)
	}

	if err := c.Post(create, &b, client.Var("n", "B"), client.Var("a", []string{})); err != nil {
		t.Fatal(err)
	}

	idA, idB := a.CreateScopePreset.ID, b.CreateScopePreset.ID

	var dup struct{ DuplicateScopePreset sp }
	if err := c.Post(`mutation($id: ID!) { duplicateScopePreset(id: $id) { id name } }`, &dup, client.Var("id", idA)); err != nil || dup.DuplicateScopePreset.Name != "A (copy)" {
		t.Fatalf("dup = %+v, %v", dup, err)
	}

	var upd struct{ UpdateScopePreset sp }
	if err := c.Post(`mutation($id: ID!) { updateScopePreset(id: $id, name: "A2", allow: [], deny: ["x.test"]) { name deny } }`, &upd, client.Var("id", idA)); err != nil || upd.UpdateScopePreset.Deny[0] != "x.test" {
		t.Fatalf("update = %+v, %v", upd, err)
	}

	// Reorder: new order is applied and returned; later creates still go last.
	const reorder = `mutation($ids: [ID!]!) { reorderScopePresets(ids: $ids) { id } }`

	var ro struct{ ReorderScopePresets []sp }
	if err := c.Post(reorder, &ro, client.Var("ids", []string{dup.DuplicateScopePreset.ID, idB, idA})); err != nil || len(ro.ReorderScopePresets) != 3 || ro.ReorderScopePresets[0].ID != dup.DuplicateScopePreset.ID {
		t.Fatalf("reorder = %+v, %v", ro, err)
	}

	var list struct{ ScopePresets []sp }
	if err := c.Post(`{ scopePresets { id } }`, &list); err != nil || list.ScopePresets[2].ID != idA {
		t.Fatalf("list = %+v, %v", list, err)
	}

	for name, ids := range map[string][]string{
		"missing":   {idA, idB},
		"extra":     {idA, idB, dup.DuplicateScopePreset.ID, "99"},
		"duplicate": {idA, idA, idB},
		"unknown":   {idA, idB, "99"},
		"non-num":   {idA, idB, "x"},
	} {
		t.Run(name, func(t *testing.T) { wantCode(t, c.Post(reorder, &ro, client.Var("ids", ids)), "BAD_INPUT") })
	}

	// Failed reorder changed nothing.
	if err := c.Post(`{ scopePresets { id } }`, &list); err != nil || list.ScopePresets[0].ID != dup.DuplicateScopePreset.ID {
		t.Fatalf("order changed: %+v, %v", list, err)
	}

	var one struct{ ScopePreset *sp }
	if err := c.Post(`{ scopePreset(id: "999") { id } }`, &one); err != nil || one.ScopePreset != nil {
		t.Fatalf("missing = %+v, %v", one, err)
	}

	wantCode(t, c.Post(`mutation { createScopePreset(name: "", allow: []) { id } }`, &a), "BAD_INPUT")
	wantCode(t, c.Post(`mutation { createScopePreset(name: "B", allow: []) { id } }`, &a), "CONFLICT")
	wantCode(t, c.Post(`mutation { updateScopePreset(id: "999", name: "Z", allow: [], deny: []) { id } }`, &a), "NOT_FOUND")
	wantCode(t, c.Post(`mutation { deleteScopePreset(id: "abc") }`, &a), "NOT_FOUND")

	var del struct{ DeleteScopePreset string }
	if err := c.Post(`mutation($id: ID!) { deleteScopePreset(id: $id) }`, &del, client.Var("id", idB)); err != nil || del.DeleteScopePreset != idB {
		t.Fatalf("delete = %+v, %v", del, err)
	}
}

func TestFilterResolvers(t *testing.T) {
	t.Parallel()

	c := presetClient(t, true)

	type fp struct {
		ID, Name, Query string
		Alias           *string
	}

	var a struct{ CreateFilterPreset fp }
	if err := c.Post(`mutation { createFilterPreset(name: "Posts", alias: "posts", query: "req.method.eq:\"POST\"") { id name alias query } }`, &a); err != nil || a.CreateFilterPreset.Alias == nil || *a.CreateFilterPreset.Alias != "posts" {
		t.Fatalf("create = %+v, %v", a, err)
	}

	var b struct{ CreateFilterPreset fp }
	if err := c.Post(`mutation { createFilterPreset(name: "Both", query: "preset:posts") { id alias } }`, &b); err != nil || b.CreateFilterPreset.Alias != nil {
		t.Fatalf("create b = %+v, %v", b, err)
	}

	var upd struct{ UpdateFilterPreset fp }
	if err := c.Post(`mutation($id: ID!) { updateFilterPreset(id: $id, name: "Both", query: "req.method.eq:\"GET\"") { query } }`, &upd, client.Var("id", b.CreateFilterPreset.ID)); err != nil {
		t.Fatal(err)
	}

	var one struct{ FilterPreset *fp }
	if err := c.Post(`{ filterPreset(id: "zz") { id } }`, &one); err != nil || one.FilterPreset != nil {
		t.Fatalf("missing = %+v, %v", one, err)
	}

	idA, idB := a.CreateFilterPreset.ID, b.CreateFilterPreset.ID

	wantCode(t, c.Post(`mutation { createFilterPreset(name: "", query: "x") { id } }`, &a), "BAD_INPUT")
	wantCode(t, c.Post(`mutation { createFilterPreset(name: "Posts", query: "req.method.eq:\"GET\"") { id } }`, &a), "CONFLICT")
	wantCode(t, c.Post(`mutation { createFilterPreset(name: "Bad", query: "preset:nope") { id } }`, &a), "BAD_QUERY")
	wantCode(t, c.Post(`mutation { updateFilterPreset(id: "999", name: "Z", query: "req.method.eq:\"GET\"") { id } }`, &a), "NOT_FOUND")
	wantCode(t, c.Post(`mutation { deleteFilterPreset(id: "999") }`, &a), "NOT_FOUND")
	wantCode(t, c.Post(`mutation { reorderFilterPresets(ids: ["1"]) { id } }`, &a), "BAD_INPUT")
	wantCode(t, c.Post(`mutation { importFilterPresets(json: "nope") }`, &a), "BAD_INPUT")

	var ro struct{ ReorderFilterPresets []fp }
	if err := c.Post(`mutation($a: ID!, $b: ID!) { reorderFilterPresets(ids: [$b, $a]) { id } }`, &ro,
		client.Var("a", idA), client.Var("b", idB)); err != nil || ro.ReorderFilterPresets[0].ID != idB {
		t.Fatalf("reorder = %+v, %v", ro, err)
	}

	// Export → delete all → import returns the same presets in the same order.
	var exp struct{ ExportFilterPresets string }
	if err := c.Post(`{ exportFilterPresets }`, &exp); err != nil {
		t.Fatal(err)
	}

	var before struct{ FilterPresets []fp }
	if err := c.Post(`{ filterPresets { name alias query } }`, &before); err != nil {
		t.Fatal(err)
	}

	for _, p := range ro.ReorderFilterPresets {
		var d struct{ DeleteFilterPreset string }
		if err := c.Post(`mutation($id: ID!) { deleteFilterPreset(id: $id) }`, &d, client.Var("id", p.ID)); err != nil {
			t.Fatal(err)
		}
	}

	var imp struct{ ImportFilterPresets int }
	if err := c.Post(`mutation($j: String!) { importFilterPresets(json: $j) }`, &imp, client.Var("j", exp.ExportFilterPresets)); err != nil || imp.ImportFilterPresets != 2 {
		t.Fatalf("import = %+v, %v", imp, err)
	}

	var after struct{ FilterPresets []fp }
	if err := c.Post(`{ filterPresets { name alias query } }`, &after); err != nil {
		t.Fatal(err)
	}

	if len(after.FilterPresets) != 2 || after.FilterPresets[0].Name != before.FilterPresets[0].Name || after.FilterPresets[1].Name != before.FilterPresets[1].Name {
		t.Fatalf("round trip: before %+v after %+v", before, after)
	}
}

func TestPresetsNotImplemented(t *testing.T) {
	t.Parallel()

	c := presetClient(t, false)

	var out struct{ ScopePresets []struct{ ID string } }
	wantCode(t, c.Post(`{ scopePresets { id } }`, &out), "NOT_IMPLEMENTED")
	wantCode(t, c.Post(`{ exportFilterPresets }`, &out), "NOT_IMPLEMENTED")
}
