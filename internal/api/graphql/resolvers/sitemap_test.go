package resolvers_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/store"
)

func TestSitemapResolvers(t *testing.T) {
	t.Parallel()

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
	err = proj.DB().Write(t.Context(), func(tx *sql.Tx) error {
		for _, path := range []string{"/a", "/a//b/", "/a/b"} {
			for _, host := range []string{"::1", "other.test"} {
				_, err := store.InsertRequestTx(t.Context(), tx, store.Request{
					Source: "proxy", Host: host, Port: 443, TLS: true, Method: "GET", Path: path,
					Query: "a=1&b=2", Raw: []byte("GET / HTTP/1.1\r\n\r\n"), CreatedAt: 1000,
				})
				if err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := history.NewService(history.ServiceDeps{DB: proj.DB, Scope: func(_ context.Context, _ string) (core.Node, error) {
		return httpql.Parse(`req.host.eq:"::1"`)
	}})
	c := client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(resolvers.Deps{Projects: proj, History: svc})}))
	type node struct {
		ID, Kind, Label, URL, Host, Query string
		ParentID                          *string
		Port, RequestCount                int
		TLS, HasChildren                  bool
	}
	var tree struct{ SitemapNodes []node }
	const nodes = `query($q: String, $scope: ID, $parent: ID, $recursive: Boolean!) {
		sitemapNodes(query: $q, scopeId: $scope, parentId: $parent, recursive: $recursive) {
			id parentId kind label url host port tls requestCount hasChildren query
		}
	}`
	for _, bar := range []string{"", "/* comment */", `req.method.eq:"GET" // trailing`} {
		if err := c.Post(nodes, &tree, client.Var("q", bar), client.Var("scope", "ipv6"), client.Var("recursive", true)); err != nil {
			t.Fatal(err)
		}
		kinds := map[string]bool{}
		for _, n := range tree.SitemapNodes {
			kinds[n.Kind] = true
			if n.Host != "::1" || !n.TLS || n.Port != 443 {
				t.Fatalf("node = %+v", n)
			}
			var table struct{ Requests struct{ TotalCount int } }
			if err := c.Post(`query($q: String!, $scope: ID) { requests(query: $q, scopeId: $scope) { totalCount } }`, &table,
				client.Var("q", n.Query), client.Var("scope", "ipv6")); err != nil || table.Requests.TotalCount != n.RequestCount {
				t.Fatalf("badge/table mismatch: node=%+v total=%d err=%v", n, table.Requests.TotalCount, err)
			}
		}
		if len(kinds) != 4 || tree.SitemapNodes[0].Label != "https://[::1]:443" {
			t.Fatalf("tree = %+v", tree)
		}
	}
	host := tree.SitemapNodes[0]
	if err := c.Post(nodes, &tree, client.Var("parent", host.ID), client.Var("recursive", false)); err != nil ||
		len(tree.SitemapNodes) != 1 || tree.SitemapNodes[0].ParentID == nil || *tree.SitemapNodes[0].ParentID != host.ID {
		t.Fatalf("lazy children = %+v, %v", tree, err)
	}
	var del struct{ DeleteRequests string }
	if err := c.Post(`mutation($q: String!, $scope: ID) { deleteRequests(query: $q, scopeId: $scope) }`, &del,
		client.Var("q", host.Query), client.Var("scope", "ipv6")); err != nil || del.DeleteRequests != "3" {
		t.Fatalf("delete = %+v, %v", del, err)
	}
	if err := c.Post(nodes, &tree, client.Var("recursive", false)); err != nil || len(tree.SitemapNodes) != 1 || tree.SitemapNodes[0].Host != "other.test" {
		t.Fatalf("remaining = %+v, %v", tree, err)
	}
	wantCode(t, c.Post(nodes, &tree, client.Var("parent", "!"), client.Var("recursive", false)), "BAD_INPUT")
	wantCode(t, c.Post(nodes, &tree, client.Var("q", "("), client.Var("recursive", false)), "BAD_QUERY")
}
