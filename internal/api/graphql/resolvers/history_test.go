package resolvers_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/store"
)

func TestHistoryResolvers(t *testing.T) {
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

	raw := "GET /a HTTP/1.1\r\nHost: a.test\r\n\r\n"

	err = proj.DB().Write(t.Context(), func(tx *sql.Tx) error {
		_, err := store.InsertRequestTx(t.Context(), tx, store.Request{
			Source: "proxy", Host: "a.test", Port: 80, Method: "GET", Path: "/a",
			Raw: []byte(raw), BodyOffset: len(raw), CreatedAt: 1000,
		})

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	svc := history.NewService(history.ServiceDeps{DB: proj.DB})
	c := client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(resolvers.Deps{Projects: proj, History: svc})}))

	var list struct {
		Requests struct {
			TotalCount int
			Edges      []struct {
				Node struct{ ID, Source, Method, Head string }
			}
		}
	}

	if err := c.Post(`{ requests(query: "req.method.eq:\"GET\"") { totalCount edges { node { id source method head } } } }`, &list); err != nil {
		t.Fatal(err)
	}

	if len(list.Requests.Edges) != 1 || list.Requests.TotalCount != 1 || list.Requests.Edges[0].Node.Source != "PROXY" {
		t.Fatalf("list = %+v", list)
	}

	id := list.Requests.Edges[0].Node.ID

	var note struct{ SetRequestNote struct{ Note string } }
	if err := c.Post(`mutation($id: ID!) { setRequestNote(id: $id, note: " hi ") { note } }`, &note, client.Var("id", id)); err != nil || note.SetRequestNote.Note != "hi" {
		t.Fatalf("note = %+v, %v", note, err)
	}

	var missing struct{ Request *struct{ ID string } }
	if err := c.Post(`{ request(id: "999") { id } }`, &missing); err != nil || missing.Request != nil {
		t.Fatalf("missing = %+v, %v", missing, err)
	}

	err = c.Post(`{ requests(query: "bogus") { totalCount } }`, &list)
	if err == nil || !strings.Contains(err.Error(), "BAD_QUERY") {
		t.Fatalf("bad query err = %v", err)
	}

	var del struct{ DeleteRequests string }
	if err := c.Post(`mutation { deleteRequests(query: "") }`, &del); err != nil || del.DeleteRequests != "1" {
		t.Fatalf("delete = %+v, %v", del, err)
	}
}
