package history_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/store"
)

func openDB(t *testing.T) *store.DB {
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

	return db
}

func sample() history.Entry {
	raw := []byte("GET http://h.test/a/b.js?x=1#f HTTP/1.1\r\nHost: h.test\r\nX-A: 1\r\n\r\n")

	return history.Entry{
		Host: "h.test", Port: 80, Method: "GET", Target: "http://h.test/a/b.js?x=1#f",
		CreatedAt: time.UnixMilli(1000),
		Req:       history.Side{Raw: raw, BodyOffset: len(raw), Headers: []history.Field{{"Host", "h.test"}, {"X-A", "1"}}},
		Resp: &history.Response{
			Raw: []byte("HTTP/1.1 200 OK\r\n\r\n"), BodyOffset: 19, Headers: []history.Field{{"Content-Type", "x"}},
			Code: 200, RoundTrip: 7 * time.Millisecond, CreatedAt: time.UnixMilli(1010),
		},
	}
}

func TestCaptureStoresAndAnnounces(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	bus := &events.Bus[history.Event]{}
	c := history.New(history.Deps{DB: func() *store.DB { return db }, Bus: bus})

	ctx, cancel := context.WithCancel(t.Context())
	sub := bus.Subscribe(ctx)
	done := make(chan struct{})

	go func() { c.Run(ctx); close(done) }()

	c.Add(ctx, sample())

	if m := <-sub; m.Value.Kind != history.Added {
		t.Fatalf("first event = %+v, want Added", m)
	}

	if m := <-sub; m.Value.Kind != history.Updated {
		t.Fatalf("second event = %+v, want Updated", m)
	}

	var path, query, ext string

	var code, hdrs int

	err := db.Reader().QueryRowContext(ctx, `SELECT r.path, r.query, r.ext, s.code,
		(SELECT count(*) FROM request_headers) FROM requests r JOIN responses s ON s.request_id = r.id`).
		Scan(&path, &query, &ext, &code, &hdrs)
	if err != nil {
		t.Fatal(err)
	}

	if path != "/a/b.js" || query != "x=1" || ext != ".js" || code != 200 || hdrs != 2 {
		t.Fatalf("row = %q %q %q %d %d", path, query, ext, code, hdrs)
	}

	cancel()
	<-done
}

func TestCapturePausedWithoutProject(t *testing.T) {
	t.Parallel()

	c := history.New(history.Deps{DB: func() *store.DB { return nil }})
	c.Add(t.Context(), sample()) // must not block: nothing runs the writer

	if !c.Paused() || c.Dropped() != 1 {
		t.Fatalf("paused=%v dropped=%d", c.Paused(), c.Dropped())
	}
}

func TestCaptureFlushesOnShutdown(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	c := history.New(history.Deps{DB: func() *store.DB { return db }})

	ctx, cancel := context.WithCancel(t.Context())
	c.Add(ctx, sample())
	cancel()
	c.Run(ctx) // returns after the final flush

	var n int
	if err := db.Reader().QueryRowContext(t.Context(), "SELECT count(*) FROM requests").Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows = %d, err = %v", n, err)
	}
}
