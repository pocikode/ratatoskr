package resolvers_test

import (
	"database/sql"
	"encoding/base64"
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

	resp := "HTTP/1.1 200 OK\r\n\r\nok"
	raw := "GET /a HTTP/1.1\r\nHost: a.test\r\n\r\n"

	err = proj.DB().Write(t.Context(), func(tx *sql.Tx) error {
		_, err := store.InsertRequestTx(t.Context(), tx, store.Request{
			Source: "proxy", Host: "a.test", Port: 80, Method: "GET", Path: "/a",
			Raw: []byte(raw), BodyOffset: len(raw), CreatedAt: 1000,
		})

		if err != nil {
			return err
		}

		_, err = tx.Exec(`INSERT INTO responses (request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
			VALUES (1, 200, ?, ?, ?, 5, 2000)`, []byte(resp), len(resp)-2, len(resp))

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

	var rsp struct {
		Request struct{ Response struct{ Head, Body string } }
	}
	if err := c.Post(`query($id: ID!) { request(id: $id) { response { head body(offset: 0, limit: 10, decode: true) } } }`, &rsp, client.Var("id", id)); err != nil ||
		rsp.Request.Response.Head != base64.StdEncoding.EncodeToString([]byte("HTTP/1.1 200 OK\r\n\r\n")) || rsp.Request.Response.Body != base64.StdEncoding.EncodeToString([]byte("ok")) {
		t.Fatalf("response = %+v, %v", rsp, err)
	}

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
