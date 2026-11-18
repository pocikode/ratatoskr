package history_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/store"
)

func sitemapSeed(ctx context.Context, db *store.DB, rows []store.Request) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		for _, row := range rows {
			row.Source, row.CreatedAt = "proxy", 1000
			row.Raw = []byte("GET / HTTP/1.1\r\n\r\n")
			if row.Method == "" {
				row.Method = "GET"
			}
			if _, err := store.InsertRequestTx(ctx, tx, row); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestSitemapCountsAndQueries(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var rows []store.Request
	for _, host := range []string{"a.test", "::1", `h"\\.test`} {
		for _, path := range []string{"/", "*", "http://absolute.test/a", "/a", "/a//b/", "/a/b/c", "/a.b/(x)+/[z]", "/%2F", "/A", "/a/quote\"\\"} {
			for _, q := range []string{"", "&", "a=1&b=2", "b=3&&a=4&a=5&", "a=1", "=x", "a.b=1&[x]=2", "a=1&=x"} {
				rows = append(rows, store.Request{Host: host, Port: 443, TLS: true, Path: path, Query: q})
			}
		}
	}
	rows = append(rows, store.Request{Host: "a.test", Port: 443, Path: "/z", Method: "POST"})
	rows = append(rows, store.Request{Host: "a.test", Port: 80, Path: "/z"})
	if err := sitemapSeed(t.Context(), e.db, rows); err != nil {
		t.Fatal(err)
	}
	svc := history.NewService(history.ServiceDeps{DB: func() *store.DB { return e.db }, Scope: func(_ context.Context, id string) (core.Node, error) {
		if id != "a" {
			return nil, apperr.New(apperr.NotFound, "unknown scope")
		}
		return httpql.Parse(`req.host.eq:"a.test"`)
	}})
	var firstIDs []string
	for _, bar := range []string{"", " /* note */ // comment", `req.method.eq:"GET"`, `req.method.eq:"GET" // trailing`} {
		for _, scope := range []*string{nil, new("a")} {
			nodes, err := svc.SitemapNodes(t.Context(), nil, &bar, scope, true)
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[string]bool{}
			for _, n := range nodes {
				kinds[n.Kind] = true
				page, err := svc.List(t.Context(), history.PageArgs{Query: &n.Query, ScopeID: scope, First: new(1)})
				if err != nil {
					t.Fatalf("%s %q query %q: %v", n.Kind, n.URL, n.Query, err)
				}
				if page.Total != n.RequestCount {
					t.Fatalf("bar=%q scope=%v %s %q: table=%d badge=%d query=%s", bar, scope, n.Kind, n.URL, page.Total, n.RequestCount, n.Query)
				}
				children, err := svc.SitemapNodes(t.Context(), &n.ID, &bar, scope, false)
				if err != nil {
					t.Fatal(err)
				}
				if (len(children) > 0) != n.HasChildren {
					t.Fatalf("children of %s: %v", n.URL, children)
				}
				for _, child := range children {
					if child.ParentID == nil || *child.ParentID != n.ID {
						t.Fatalf("wrong parent: %+v", child)
					}
				}
			}
			if len(kinds) != 4 {
				t.Fatalf("missing kinds: %v", kinds)
			}
			if scope == nil && bar == "" {
				for _, n := range nodes {
					firstIDs = append(firstIDs, n.ID)
				}
			}
			if scope == nil && strings.Contains(bar, "note") {
				var ids []string
				for _, n := range nodes {
					ids = append(ids, n.ID)
				}
				if !slices.Equal(ids, firstIDs) {
					t.Fatal("comment-only query changed IDs")
				}
			}
		}
	}
}

func TestSitemapOrderingDeletionAndErrors(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rows := []store.Request{
		{Host: "b.test", Port: 80, Path: "/"},
		{Host: "a.test", Port: 443, TLS: true, Path: "/a", Query: "z=1"},
		{Host: "a.test", Port: 443, TLS: true, Path: "/a", Query: "a=1"},
		{Host: "a.test", Port: 443, TLS: true, Path: "/a/b"},
		{Host: "a.test", Port: 443, TLS: true, Path: "/a//b/"},
		{Host: "a.test", Port: 443, Path: "/z"},
		{Host: "a.test", Port: 80, Path: "/z"},
	}
	if err := sitemapSeed(t.Context(), e.db, rows); err != nil {
		t.Fatal(err)
	}
	roots, err := e.svc.SitemapNodes(t.Context(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, n := range roots {
		labels = append(labels, n.Label)
	}
	if !slices.Equal(labels, []string{"http://a.test:80", "http://a.test:443", "https://a.test:443", "http://b.test:80"}) {
		t.Fatal(labels)
	}
	nodes, err := e.svc.SitemapNodes(t.Context(), &roots[2].ID, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	labels = nil
	for _, n := range nodes {
		labels = append(labels, n.Label)
	}
	if !slices.Equal(labels, []string{"a", "b", "?a", "?z"}) {
		t.Fatal(labels)
	}
	n := nodes[1]
	deleted, err := e.svc.Delete(t.Context(), history.DeleteArgs{Query: &n.Query})
	if err != nil || deleted != int64(n.RequestCount) {
		t.Fatalf("delete: %d, %v", deleted, err)
	}
	left, err := e.svc.SitemapNodes(t.Context(), &n.ID, nil, nil, false)
	if err != nil || len(left) != 0 {
		t.Fatalf("deleted node: %v %v", left, err)
	}
	for _, id := range []string{"", "!", "e30", "bnVsbA"} {
		if _, err := e.svc.SitemapNodes(t.Context(), &id, nil, nil, false); apperr.CodeOf(err) != apperr.BadInput {
			t.Fatalf("bad id %q: %v", id, err)
		}
	}
	if _, err := e.svc.SitemapNodes(t.Context(), nil, new("("), nil, false); apperr.CodeOf(err) != apperr.BadQuery {
		t.Fatal(err)
	}
	bar := `req.host.eq:"` + strings.Repeat("a", core.MaxQueryLen-30) + `"`
	if _, err := e.svc.SitemapNodes(t.Context(), nil, &bar, nil, false); err != nil {
		t.Fatal(err)
	} // no matching nodes
	bar = `req.host.ne:"` + strings.Repeat("a", core.MaxQueryLen-30) + `"`
	if _, err := e.svc.SitemapNodes(t.Context(), nil, &bar, nil, false); apperr.CodeOf(err) != apperr.BadQuery {
		t.Fatalf("long composition: %v", err)
	}
}

func TestSitemapRecursiveLimit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rows := make([]store.Request, 5000)
	for i := range rows {
		rows[i] = store.Request{Host: "a.test", Port: 80, Path: fmt.Sprintf("/p%d", i)}
	}
	if err := sitemapSeed(t.Context(), e.db, rows); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SitemapNodes(t.Context(), nil, nil, nil, true); apperr.CodeOf(err) != apperr.BadInput {
		t.Fatalf("over cap: %v", err)
	}
	roots, err := e.svc.SitemapNodes(t.Context(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := e.svc.SitemapNodes(t.Context(), &roots[0].ID, nil, nil, true)
	if err != nil || len(nodes) != 5000 {
		t.Fatalf("at cap: %d %v", len(nodes), err)
	}
}

func BenchmarkSitemapRoots(b *testing.B) {
	root, err := os.OpenRoot(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = root.Close() })
	db, err := store.OpenProject(b.Context(), root)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := db.Close(); err != nil {
			b.Error(err)
		}
	})
	rows := make([]store.Request, 10000)
	for i := range rows {
		rows[i] = store.Request{Host: fmt.Sprintf("h%03d.test", i%200), Port: 443, TLS: true, Path: fmt.Sprintf("/a/p%d", i), Query: "a=1&b=2"}
	}
	if err := sitemapSeed(b.Context(), db, rows); err != nil {
		b.Fatal(err)
	}
	svc := history.NewService(history.ServiceDeps{DB: func() *store.DB { return db }})
	b.ResetTimer()
	for b.Loop() {
		start := time.Now()
		nodes, err := svc.SitemapNodes(b.Context(), nil, nil, nil, false)
		if err != nil || len(nodes) != 200 {
			b.Fatalf("roots: %d %v", len(nodes), err)
		}
		if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
			b.Fatalf("root load: %v, budget 500ms", elapsed)
		}
	}
}
