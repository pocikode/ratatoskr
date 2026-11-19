package resolvers_test

import (
	"database/sql"
	"encoding/base64"
	"os"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/replay"
	"github.com/pocikode/ratatoskr/internal/store"
)

func replayClient(t *testing.T) (*client.Client, *replay.Service, *store.DB) {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	current := func() *store.DB { return db }
	svc := replay.New(current)
	deps := resolvers.Deps{Replay: svc, History: history.NewService(history.ServiceDeps{DB: current})}
	return client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(deps)})), svc, db
}

type replayCollectionResult struct {
	ID, Name  string
	IsDefault bool
	Sessions  []replaySessionResult
}

type replaySessionResult struct {
	ID, Name, Kind, Draft string
	Color                 *string
	Sending               bool
	Collection            struct {
		ID, Name  string
		IsDefault bool
	}
	Connection struct {
		Host string
		Port int
		TLS  bool
		SNI  *string
	}
	Options struct {
		FollowRedirects, UpdateContentLength, ApplyMatchReplace bool
		TimeoutMs                                               int
	}
	Entries struct {
		TotalCount int
		PageInfo   struct {
			HasNextPage, HasPreviousPage bool
			EndCursor                    *string
		}
		Edges []struct {
			Cursor string
			Node   struct {
				ID      string
				Error   *string
				Request struct {
					ID, Host, Head, Body string
					Response             *struct{ Code int }
				}
			}
		}
	}
}

const replaySessionFields = `id name kind draft color sending collection { id name isDefault } connection { host port tls sni } options { followRedirects updateContentLength timeoutMs applyMatchReplace }`

func TestReplayGraphQLCRUD(t *testing.T) {
	t.Parallel()
	c, _, _ := replayClient(t)
	var cs struct{ ReplayCollections []replayCollectionResult }
	c.MustPost(`{ replayCollections { id name isDefault sessions { id } } }`, &cs)
	if len(cs.ReplayCollections) != 1 || !cs.ReplayCollections[0].IsDefault || len(cs.ReplayCollections[0].Sessions) != 0 {
		t.Fatal(cs)
	}
	def := cs.ReplayCollections[0].ID
	var collection struct{ CreateReplayCollection replayCollectionResult }
	c.MustPost(`mutation { createReplayCollection(name:"  Target  ") { id name isDefault } }`, &collection)
	target := collection.CreateReplayCollection.ID
	if collection.CreateReplayCollection.Name != "Target" || collection.CreateReplayCollection.IsDefault {
		t.Fatal(collection)
	}
	var renamed struct{ RenameReplayCollection replayCollectionResult }
	c.MustPost(`mutation($id:ID!){ renameReplayCollection(id:$id,name:"Main"){id name isDefault} }`, &renamed, client.Var("id", def))
	if !renamed.RenameReplayCollection.IsDefault || renamed.RenameReplayCollection.Name != "Main" {
		t.Fatal(renamed)
	}
	raw := base64.StdEncoding.EncodeToString([]byte("POST /exact HTTP/1.1\r\nHoST: unchanged.test\r\n\r\n\x00\xff"))
	input := map[string]any{"connection": map[string]any{"host": "dial.test", "port": 443, "tls": true}, "raw": raw}
	var created struct{ CreateReplaySession replaySessionResult }
	c.MustPost(`mutation($in:CreateReplaySessionInput!){ createReplaySession(input:$in){`+replaySessionFields+`} }`, &created, client.Var("in", input))
	a := created.CreateReplaySession
	if a.Name != "POST /exact" || a.Kind != "HTTP" || a.Draft != raw || a.Color != nil || a.Sending || a.Collection.ID != def || !a.Options.UpdateContentLength || a.Options.TimeoutMs != 60000 || a.Options.FollowRedirects || a.Options.ApplyMatchReplace {
		t.Fatal(a)
	}
	var updated struct{ UpdateReplaySession replaySessionResult }
	c.MustPost(`mutation($id:ID!){updateReplaySession(id:$id,input:{name:" Saved ",connection:{host:"new.test",port:80,tls:false,sni:"override.test"},options:{followRedirects:true,updateContentLength:false,timeoutMs:1000,applyMatchReplace:true},draft:"AP8="}){`+replaySessionFields+`}}`, &updated, client.Var("id", a.ID))
	if updated.UpdateReplaySession.Name != "Saved" || updated.UpdateReplaySession.Draft != "AP8=" || updated.UpdateReplaySession.Connection.SNI == nil || *updated.UpdateReplaySession.Connection.SNI != "override.test" || !updated.UpdateReplaySession.Options.FollowRedirects || updated.UpdateReplaySession.Options.UpdateContentLength || updated.UpdateReplaySession.Options.TimeoutMs != 1000 {
		t.Fatal(updated)
	}
	var color struct{ SetReplaySessionColor replaySessionResult }
	c.MustPost(`mutation($id:ID!){setReplaySessionColor(id:$id,color:BLUE){color}}`, &color, client.Var("id", a.ID))
	if color.SetReplaySessionColor.Color == nil || *color.SetReplaySessionColor.Color != "BLUE" {
		t.Fatal(color)
	}
	var dup struct{ DuplicateReplaySession replaySessionResult }
	c.MustPost(`mutation($id:ID!){duplicateReplaySession(id:$id){`+replaySessionFields+` entries {totalCount}}}`, &dup, client.Var("id", a.ID))
	b := dup.DuplicateReplaySession
	if b.Name != "Saved (copy)" || b.Draft != "AP8=" || b.Color == nil || *b.Color != "BLUE" || b.Entries.TotalCount != 0 || b.ID == a.ID || b.Options.TimeoutMs != 1000 {
		t.Fatal(b)
	}
	var moved struct{ MoveReplaySession replaySessionResult }
	c.MustPost(`mutation($id:ID!,$target:ID!){moveReplaySession(id:$id,collectionId:$target,index:99){id collection{id}}}`, &moved, client.Var("id", b.ID), client.Var("target", target))
	if moved.MoveReplaySession.Collection.ID != target {
		t.Fatal(moved)
	}
	var order struct{ ReorderReplayCollections []replayCollectionResult }
	c.MustPost(`mutation($ids:[ID!]!){reorderReplayCollections(ids:$ids){id sessions{id}}}`, &order, client.Var("ids", []string{target, def}))
	if order.ReorderReplayCollections[0].ID != target || len(order.ReorderReplayCollections[0].Sessions) != 1 || order.ReorderReplayCollections[0].Sessions[0].ID != b.ID {
		t.Fatal(order)
	}
	var cleared struct{ SetReplaySessionColor replaySessionResult }
	c.MustPost(`mutation($id:ID!){setReplaySessionColor(id:$id,color:null){color}}`, &cleared, client.Var("id", a.ID))
	if cleared.SetReplaySessionColor.Color != nil {
		t.Fatal(cleared)
	}
	// Empty bytes clear the draft; null and omitted fields retain it and options.
	c.MustPost(`mutation($id:ID!){updateReplaySession(id:$id,input:{draft:""}){draft}}`, &updated, client.Var("id", a.ID))
	if updated.UpdateReplaySession.Draft != "" {
		t.Fatal(updated)
	}
	c.MustPost(`mutation($id:ID!){updateReplaySession(id:$id,input:{draft:null,name:null,options:{timeoutMs:null}}){draft name options{timeoutMs}}}`, &updated, client.Var("id", a.ID))
	if updated.UpdateReplaySession.Draft != "" || updated.UpdateReplaySession.Name != "Saved" || updated.UpdateReplaySession.Options.TimeoutMs != 1000 {
		t.Fatal(updated)
	}
	var deleted struct {
		DeleteReplayCollection string
		DeleteReplaySession    string
	}
	c.MustPost(`mutation($id:ID!){deleteReplayCollection(id:$id)}`, &deleted, client.Var("id", target))
	if deleted.DeleteReplayCollection != target {
		t.Fatal(deleted)
	}
	var one struct{ ReplaySession *replaySessionResult }
	c.MustPost(`query($id:ID!){replaySession(id:$id){id}}`, &one, client.Var("id", b.ID))
	if one.ReplaySession != nil {
		t.Fatal(one)
	}
	c.MustPost(`mutation($id:ID!){deleteReplaySession(id:$id)}`, &deleted, client.Var("id", a.ID))
	if deleted.DeleteReplaySession != a.ID {
		t.Fatal(deleted)
	}
}

func TestReplayGraphQLHistoryEntriesAndErrors(t *testing.T) {
	t.Parallel()
	c, svc, db := replayClient(t)
	raw := []byte("GET /source HTTP/1.1\r\nHost: source.test\r\n\r\n\xff")
	var reqID int64
	err := db.Write(t.Context(), func(tx *sql.Tx) error {
		res, err := tx.ExecContext(t.Context(), `INSERT INTO requests(source,host,port,tls,method,path,raw,body_offset,len,created_at) VALUES ('replay','source.test',8443,1,'GET','/source',?,?,?,1234)`, raw, len(raw)-1, len(raw))
		if err != nil {
			return err
		}
		reqID, err = res.LastInsertId()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var created struct{ CreateReplaySession replaySessionResult }
	c.MustPost(`mutation($rid:ID!){createReplaySession(input:{fromRequestId:$rid}){`+replaySessionFields+`}}`, &created, client.Var("rid", strconv.FormatInt(reqID, 10)))
	a := created.CreateReplaySession
	if a.Draft != base64.StdEncoding.EncodeToString(raw) || a.Connection.Host != "source.test" || a.Connection.Port != 8443 || a.Connection.SNI == nil || *a.Connection.SNI != "source.test" {
		t.Fatal(a)
	}
	sessionID, err := strconv.ParseInt(a.ID, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddEntry(t.Context(), sessionID, reqID, "dial failed"); err != nil {
		t.Fatal(err)
	}
	var one struct{ ReplaySession replaySessionResult }
	c.MustPost(`query($id:ID!){replaySession(id:$id){entries(first:1){totalCount pageInfo{hasNextPage hasPreviousPage endCursor} edges{cursor node{id error request{id host head body response{code}}}}}}}`, &one, client.Var("id", a.ID))
	entries := one.ReplaySession.Entries
	if entries.TotalCount != 1 || len(entries.Edges) != 1 || entries.Edges[0].Node.Error == nil || *entries.Edges[0].Node.Error != "dial failed" || entries.Edges[0].Node.Request.Host != "source.test" || entries.Edges[0].Node.Request.Body != "/w==" || entries.Edges[0].Node.Request.Response != nil || entries.PageInfo.EndCursor == nil {
		t.Fatal(entries)
	}
	var ignored any
	cases := []struct{ query, code string }{
		{`mutation {createReplayCollection(name:""){id}}`, "BAD_INPUT"},
		{`mutation {deleteReplayCollection(id:"` + a.Collection.ID + `")}`, "CONFLICT"},
		{`mutation {renameReplayCollection(id:"999",name:"X"){id}}`, "NOT_FOUND"},
		{`mutation {reorderReplayCollections(ids:["bad"]){id}}`, "BAD_INPUT"},
		{`mutation {reorderReplayCollections(ids:[]){id}}`, "BAD_INPUT"},
		{`mutation {createReplaySession(input:{}){id}}`, "BAD_INPUT"},
		{`mutation {createReplaySession(input:{fromRequestId:"unknown"}){id}}`, "NOT_FOUND"},
		{`mutation {createReplaySession(input:{fromRequestId:"999"}){id}}`, "NOT_FOUND"},
		{`mutation {createReplaySession(input:{connection:{host:"x",port:0,tls:false},raw:""}){id}}`, "BAD_INPUT"},
		{`mutation {updateReplaySession(id:"` + a.ID + `",input:{name:"changed",options:{timeoutMs:999}}){id}}`, "BAD_INPUT"},
		{`mutation {duplicateReplaySession(id:"unknown"){id}}`, "NOT_FOUND"},
		{`mutation {moveReplaySession(id:"` + a.ID + `",collectionId:"` + a.Collection.ID + `",index:-1){id}}`, "BAD_INPUT"},
		{`mutation {moveReplaySession(id:"` + a.ID + `",collectionId:"999",index:0){id}}`, "NOT_FOUND"},
		{`mutation {deleteReplaySession(id:"999")}`, "NOT_FOUND"},
		{`query {replaySession(id:"` + a.ID + `"){entries(first:0){totalCount}}}`, "BAD_INPUT"},
		{`query {replaySession(id:"` + a.ID + `"){entries(after:"bad"){totalCount}}}`, "BAD_INPUT"},
	}
	for _, tt := range cases {
		wantCode(t, c.Post(tt.query, &ignored), tt.code)
	}
	c.MustPost(`query($id:ID!){replaySession(id:$id){name}}`, &one, client.Var("id", a.ID))
	if one.ReplaySession.Name != "GET /source" {
		t.Fatal("invalid update changed name")
	}
	var missing struct{ ReplaySession *replaySessionResult }
	for _, id := range []string{"unknown", "999"} {
		c.MustPost(`query($id:ID!){replaySession(id:$id){id}}`, &missing, client.Var("id", id))
		if missing.ReplaySession != nil {
			t.Fatal(missing)
		}
	}
	// Failed sends remain History records after their session is deleted.
	c.MustPost(`mutation($id:ID!){deleteReplaySession(id:$id)}`, &ignored, client.Var("id", a.ID))
	var count int
	if err := db.Reader().QueryRowContext(t.Context(), `SELECT count(*) FROM requests WHERE id = ?`, reqID).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestReplayUnwired(t *testing.T) {
	t.Parallel()
	c := client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(resolvers.Deps{})}))
	var ignored any
	for _, q := range []string{`{replayCollections{id}}`, `{replaySession(id:"1"){id}}`, `mutation{createReplayCollection(name:"A"){id}}`, `mutation{deleteReplaySession(id:"1")}`} {
		wantCode(t, c.Post(q, &ignored), "NOT_IMPLEMENTED")
	}
}
