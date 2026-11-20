package resolvers_test

import (
	"context"
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/client"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/replay"
	"github.com/pocikode/ratatoskr/internal/store"
	"github.com/pocikode/ratatoskr/internal/testutil"
)

func TestReplaySendGraphQL(t *testing.T) {
	t.Parallel()
	_, svc, db := replayClient(t)
	current := func() *store.DB { return db }
	capture := history.New(history.Deps{DB: current})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { capture.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	c := client.New(graphql.NewHandler(graphql.Options{Resolvers: resolvers.New(resolvers.Deps{
		Replay: svc, History: history.NewService(history.ServiceDeps{DB: current}),
		ReplaySend: replay.NewSender(svc, replay.SendDeps{Capture: capture}),
	})}))

	srv := testutil.NewRawServer(t)
	s, err := svc.CreateSession(t.Context(), replay.CreateInput{Connection: &replay.Connection{Host: srv.Host, Port: srv.Port}, Raw: []byte{}})
	if err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(s.ID, 10)

	var sent struct {
		SendReplaySession struct {
			ID      string
			Error   *string
			Request struct {
				Source   string
				Response *struct{ Code int }
			}
		}
	}
	c.MustPost(`mutation($id: ID!, $raw: Bytes!, $port: Int!, $host: String!) {
		sendReplaySession(id: $id, input: {raw: $raw, connection: {host: $host, port: $port, tls: false}}) {
			id error request { source response { code } }
		}
	}`, &sent, client.Var("id", id), client.Var("raw", base64.StdEncoding.EncodeToString([]byte("GET / HTTP/1.1\r\n\r\n"))),
		client.Var("host", srv.Host), client.Var("port", srv.Port))
	got := sent.SendReplaySession
	if got.ID == "" || got.Error != nil || got.Request.Source != "REPLAY" || got.Request.Response == nil || got.Request.Response.Code != 200 {
		t.Fatalf("entry = %+v", got)
	}

	var cancelled struct{ CancelReplaySend bool }
	c.MustPost(`mutation($id: ID!) { cancelReplaySend(id: $id) }`, &cancelled, client.Var("id", id))
	if cancelled.CancelReplaySend {
		t.Fatal("cancel without a send returned true")
	}
	if err := c.Post(`mutation { cancelReplaySend(id: "999") }`, &cancelled); err == nil {
		t.Fatal("cancel of unknown session succeeded")
	}
}

func TestReplaySendUnwired(t *testing.T) {
	t.Parallel()
	c, _, _ := replayClient(t)
	var out struct{ CancelReplaySend bool }
	if err := c.Post(`mutation { cancelReplaySend(id: "1") }`, &out); err == nil {
		t.Fatal("unwired cancel succeeded")
	}
}
