package history_test

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

// newScopeEnv is [newEnv] with scopes wired, like internal/app does.
func newScopeEnv(t *testing.T) (*env, *scope.Service) {
	t.Helper()

	e := newEnv(t)
	db := func() *store.DB { return e.db }
	scopes := scope.New(db)

	e.svc = history.NewService(history.ServiceDeps{DB: db, Bus: e.bus, Scope: scopes.Node})

	return e, scopes
}

// newPresetEnv is [newEnv] with preset: expansion over the given presets.
func newPresetEnv(t *testing.T, presets ...filterpreset.Preset) *env {
	t.Helper()

	e := newEnv(t)
	e.svc = history.NewService(history.ServiceDeps{
		DB:  func() *store.DB { return e.db },
		Bus: e.bus,
		ExpandText: func(_ context.Context, src string) (core.Node, error) {
			return filterpreset.Expand(src, presets)
		},
	})

	return e
}

// TestScopeNodeAgreesWithMatcher is the F-SCOPE-1 agreement rule: for every
// scope preset the SQL filter over seeded rows matches Matcher.InScope.
func TestScopeNodeAgreesWithMatcher(t *testing.T) {
	t.Parallel()

	e, scopes := newScopeEnv(t)

	targets := []struct {
		host string
		port int
		path string
	}{
		{host: "api.example.com", port: 443, path: "/v1/users"},
		{host: "api.example.com", port: 80, path: "/v1/users"},
		{host: "API.Example.COM", port: 443, path: "/v1/users"},
		{host: "api.example.com", port: 443, path: "/v10/x"},
		{host: "www.example.com", port: 443, path: "/index.html"},
		{host: "example.com", port: 443, path: "/"},
		{host: "evil-api.example.com", port: 80, path: "/v1"},
		{host: "sub.api.example.com", port: 443, path: "/v1"},
		{host: "a_b.test", port: 443, path: "/x"},
		{host: "a%b.test", port: 443, path: "/x"},
		{host: "ab.test", port: 443, path: "/x"},
		{host: "::1", port: 80, path: "/"},
		{host: "[::1]", port: 80, path: "/"},
	}

	rowIDs := make([]int64, len(targets))
	for i, tg := range targets {
		rowIDs[i] = e.seed(t, seedRow{host: tg.host, port: tg.port, path: tg.path})
	}

	tests := []struct {
		name        string
		allow, deny []string
	}{
		{name: "empty allow allows everything"},
		{name: "allow globs", allow: []string{"*.example.com", "api.*.com"}},
		{name: "rule host is case-insensitive", allow: []string{"API.Example.COM"}},
		{name: "deny only", deny: []string{"api.example.com", "*.test"}},
		{name: "deny wins over allow", allow: []string{"*.example.com"}, deny: []string{"evil-api.example.com", "api.example.com"}},
		{name: "port equality", allow: []string{"api.example.com:443"}},
		{name: "path prefix", allow: []string{"api.example.com/v1"}},
		{name: "path prefix is case-sensitive", allow: []string{"api.example.com/V1"}},
		{name: "underscore and percent are literal", allow: []string{"a_b.test", "a%b.test"}},
		{name: "question mark matches one byte", allow: []string{"su?.*"}},
		{name: "ipv6 literal", allow: []string{"[::1]"}},
		{name: "port and path", allow: []string{"api.example.com:443/v1"}},
	}

	for _, tt := range tests {
		p, err := scopes.Create(t.Context(), scope.Preset{Name: tt.name, Allow: tt.allow, Deny: tt.deny})
		if err != nil {
			t.Fatalf("%s: create: %v", tt.name, err)
		}

		m, err := p.Matcher()
		if err != nil {
			t.Fatalf("%s: matcher: %v", tt.name, err)
		}

		id := strconv.FormatInt(p.ID, 10)

		page, err := e.svc.List(t.Context(), history.PageArgs{First: new(1000), ScopeID: &id})
		if err != nil {
			t.Fatalf("%s: list: %v", tt.name, err)
		}

		var want []int64

		for i, tg := range targets {
			if m.InScope(tg.host, tg.port, tg.path) {
				want = append(want, rowIDs[i])
			}
		}

		if got := ids(page.Items); !slices.Equal(got, want) {
			t.Errorf("%s: SQL rows = %v, want %v", tt.name, got, want)
		}

		if page.Total != len(want) {
			t.Errorf("%s: total = %d, want %d", tt.name, page.Total, len(want))
		}
	}
}

// TestScopeFiltersListCountDeleteAndAdded checks that one preset narrows the
// list, its total, delete-by-query and requestAdded the same way.
func TestScopeFiltersListCountDeleteAndAdded(t *testing.T) {
	t.Parallel()

	e, scopes := newScopeEnv(t)

	p, err := scopes.Create(t.Context(), scope.Preset{Name: "only a", Allow: []string{"a.test"}})
	if err != nil {
		t.Fatal(err)
	}

	id := strconv.FormatInt(p.ID, 10)

	in1 := e.seed(t, seedRow{host: "a.test"})
	out1 := e.seed(t, seedRow{host: "b.test"})
	in2 := e.seed(t, seedRow{host: "a.test"})

	page, err := e.svc.List(t.Context(), history.PageArgs{ScopeID: &id})
	if err != nil || page.Total != 2 || !slices.Equal(ids(page.Items), []int64{in1, in2}) {
		t.Fatalf("list: rows %v total %d, err %v", ids(page.Items), page.Total, err)
	}

	// Subscriptions filter the same way; the bus keeps the publish order, so a
	// leaked out-of-scope row would arrive before the in-scope one.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch, err := e.svc.SubscribeAdded(ctx, nil, &id)
	if err != nil {
		t.Fatal(err)
	}

	hit := e.seed(t, seedRow{host: "a.test"})
	e.bus.Publish(history.Event{Kind: history.Added, RequestID: out1})
	e.bus.Publish(history.Event{Kind: history.Added, RequestID: hit})

	if r := recv(t, ch); r.ID != hit {
		t.Fatalf("requestAdded delivered row %d, want %d", r.ID, hit)
	}

	// Delete-by-query removes only what the scope allows.
	if n, err := e.svc.Delete(t.Context(), history.DeleteArgs{Query: new(""), ScopeID: &id}); err != nil || n != 3 {
		t.Fatalf("delete in scope = %d, %v; want 3", n, err)
	}

	rest, err := e.svc.List(t.Context(), history.PageArgs{})
	if err != nil || !slices.Equal(ids(rest.Items), []int64{out1}) {
		t.Fatalf("rows after delete = %v, %v; want only %d", ids(rest.Items), err, out1)
	}

	cancel()

	for range ch {
	}

	unknown := "999"

	if _, err := e.svc.List(t.Context(), history.PageArgs{ScopeID: &unknown}); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("list with unknown scope: %v", err)
	}

	if _, err := e.svc.Delete(t.Context(), history.DeleteArgs{Query: new(""), ScopeID: &unknown}); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("delete with unknown scope: %v", err)
	}

	if _, err := e.svc.SubscribeAdded(t.Context(), nil, &unknown); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("subscribe with unknown scope: %v", err)
	}
}

// TestPresetExpansion covers preset: in every query the service parses.
func TestPresetExpansion(t *testing.T) {
	t.Parallel()

	errs := filterpreset.Preset{ID: 1, Name: "Errors", Alias: "errors", Query: `resp.code.gte:400`}
	nested := filterpreset.Preset{ID: 2, Name: "In A", Alias: "in-a", Query: `preset:errors AND req.host.eq:"a.test"`}
	loop := filterpreset.Preset{ID: 3, Name: "Loop", Alias: "loop", Query: `preset:loop`}

	e := newPresetEnv(t, errs, nested, loop)

	a404 := e.seed(t, seedRow{host: "a.test", code: 404})
	e.seed(t, seedRow{host: "a.test", code: 200})
	b500 := e.seed(t, seedRow{host: "b.test", code: 500})

	tests := []struct {
		name  string
		query string
		want  []int64
	}{
		{"alias", `preset:errors`, []int64{a404, b500}},
		{"name", `preset:"Errors"`, []int64{a404, b500}},
		{"nested preset", `preset:in-a`, []int64{a404}},
		{"with a clause", `preset:errors AND req.host.eq:"b.test"`, []int64{b500}},
	}

	for _, tt := range tests {
		page, err := e.svc.List(t.Context(), history.PageArgs{Query: &tt.query})
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}

		if got := ids(page.Items); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch, err := e.svc.SubscribeAdded(ctx, new(`preset:errors`), nil)
	if err != nil {
		t.Fatal(err)
	}

	hit := e.seed(t, seedRow{host: "c.test", code: 500})
	e.bus.Publish(history.Event{Kind: history.Added, RequestID: hit})

	if r := recv(t, ch); r.ID != hit {
		t.Fatalf("requestAdded delivered row %d, want %d", r.ID, hit)
	}

	cancel()

	for range ch {
	}

	for _, tt := range []struct{ name, query string }{
		{"unknown preset", `preset:nope`},
		{"preset cycle", `preset:loop`},
	} {
		_, err := e.svc.List(t.Context(), history.PageArgs{Query: &tt.query})

		ae, ok := errorsAsApp(err)
		if !ok || ae.Code != apperr.BadQuery || ae.Span == nil {
			t.Errorf("%s: %v, want BAD_QUERY with a span", tt.name, err)
		}
	}
}
