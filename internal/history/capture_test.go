package history_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/store"
)

func openDB(t testing.TB) *store.DB {
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

func BenchmarkCapture(b *testing.B) {
	db := openDB(b)
	c := history.New(history.Deps{DB: func() *store.DB { return db }})
	ctx, cancel := context.WithCancel(b.Context())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	entry := sample()
	for b.Loop() {
		id := c.NewPending(ctx, entry)
		c.Complete(ctx, id, entry.Resp)
	}

	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		var n int
		if err := db.Reader().QueryRowContext(ctx, "SELECT count(*) FROM responses").Scan(&n); err != nil {
			b.Fatal(err)
		}
		if n == b.N {
			return
		}
		if time.Now().After(deadline) || c.Dropped() != 0 {
			b.Fatalf("stored %d of %d responses, dropped %d", n, b.N, c.Dropped())
		}
	}
}

func TestPendingQueuesBeforeFlush(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	synctest.Test(t, func(t *testing.T) {
		bus := &events.Bus[history.Event]{}
		c := history.New(history.Deps{DB: func() *store.DB { return db }, Bus: bus})
		sub := bus.Subscribe(t.Context())
		ctx, cancel := context.WithCancel(t.Context())
		start := time.Now()
		e := sample()
		id := c.NewPending(ctx, e)
		if id != 1 || time.Since(start) >= history.FlushInterval {
			t.Fatalf("ID %d returned after %s", id, time.Since(start))
		}
		c.Complete(ctx, id, e.Resp)
		cancel()
		c.Run(ctx)

		for _, kind := range []history.Kind{history.Added, history.Updated} {
			select {
			case got := <-sub:
				if got.Value.Kind != kind || got.Value.RequestID != id {
					t.Fatalf("event = %+v, want kind %v for %d", got, kind, id)
				}
			default:
				t.Fatalf("missing event %v for %d", kind, id)
			}
		}
	})
}

func TestWrittenWaitsForPendingCommit(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	synctest.Test(t, func(t *testing.T) {
		var active atomic.Pointer[store.DB]
		active.Store(db)
		c := history.New(history.Deps{DB: active.Load})
		svc := history.NewService(history.ServiceDeps{DB: active.Load})
		ctx, stop := context.WithCancel(t.Context())
		defer stop()
		go c.Run(ctx)

		waitFor := func(ctx context.Context, id int64) <-chan error {
			done := make(chan error, 1)
			go func() { done <- c.Written(ctx, id) }()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("Written(%d) returned early: %v", id, err)
			default:
				return done
			}
			return nil
		}

		id := c.NewPending(ctx, sample())
		waitCtx, cancel := context.WithCancel(ctx)
		canceled := waitFor(waitCtx, id)
		cancel()
		if err := <-canceled; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Written = %v", err)
		}

		start := time.Now()
		if err := <-waitFor(ctx, id); err != nil || time.Since(start) != history.FlushInterval {
			t.Fatalf("Written returned after %s: %v", time.Since(start), err)
		}
		if r, err := svc.Get(ctx, id); err != nil || r.ID != id || r.Resp != nil {
			t.Fatalf("committed request = %+v, err %v", r, err)
		}
		start = time.Now()
		if err := c.Written(ctx, id); err != nil || time.Since(start) != 0 {
			t.Fatalf("Written for committed request returned after %s: %v", time.Since(start), err)
		}
		c.CompleteDropped(ctx, id, nil)
		time.Sleep(history.FlushInterval)
		synctest.Wait()
		if r, err := svc.Get(ctx, id); err != nil || !r.Dropped || r.Resp != nil {
			t.Fatalf("dropped request = %+v, err %v", r, err)
		}

		next := c.NewPending(ctx, sample())
		closed := waitFor(ctx, next)
		active.Store(nil)
		if err := <-closed; err == nil {
			t.Fatal("Written succeeded after the project closed")
		}
	})
}

func TestPendingIDsAcrossProjectSwitch(t *testing.T) {
	t.Parallel()

	old, other := openDB(t), openDB(t)
	active := old
	c := history.New(history.Deps{DB: func() *store.DB { return active }})
	e := sample()
	ctx, cancel := context.WithCancel(t.Context())
	id := c.NewPending(ctx, e)
	cancel()
	c.Run(ctx)

	if id != 1 {
		t.Fatalf("first ID = %d", id)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}

	active = other
	ctx, cancel = context.WithCancel(t.Context())
	otherID := c.NewPending(ctx, e)
	if otherID == id || otherID == 0 {
		t.Fatalf("new project reused outstanding ID %d", otherID)
	}
	if c.PendingDB(id) != old || c.PendingDB(otherID) != other {
		t.Fatal("pending requests lost their original project")
	}
	edit := e
	edit.ProjectDB = old
	edit.EditedFrom = &id
	if got := c.NewPending(ctx, edit); got != 0 {
		t.Fatalf("queued an edited record in the wrong project: ID %d", got)
	}
	c.Complete(ctx, id, e.Resp)
	c.Complete(ctx, otherID, e.Resp)
	cancel()
	c.Run(ctx)
	if c.Dropped() != 2 {
		t.Fatalf("dropped = %d, want 2", c.Dropped())
	}
	if c.PendingDB(id) != nil || c.PendingDB(otherID) != nil {
		t.Fatal("completed requests still reserve project ownership")
	}

	root, err := os.OpenRoot(filepath.Dir(old.Path()))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	active, err = store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()

	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	if first, second := c.NewPending(ctx, e), c.NewPending(ctx, e); first != id+1 || second != first+1 {
		t.Fatalf("IDs after reopen = %d, %d", first, second)
	}
}

func TestCaptureStoresEditedPairAndDroppedRequest(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	dbForCapture := func() *store.DB { return db }
	c := history.New(history.Deps{DB: dbForCapture})
	ctx, cancel := context.WithCancel(t.Context())
	e := sample()
	original := c.NewPending(ctx, e)

	edit := e
	edit.ProjectDB = c.PendingDB(original)
	c.Complete(ctx, original, e.Resp)
	edit.Source = "intercept"
	edit.EditedFrom = &original
	edited := c.NewPending(ctx, edit)
	resp := *edit.Resp
	resp.Truncated = true
	c.Complete(ctx, edited, &resp)

	dropped := c.NewPending(ctx, e)
	c.CompleteDropped(ctx, dropped, nil)
	droppedResponse := c.NewPending(ctx, e)
	c.CompleteDropped(ctx, droppedResponse, e.Resp)
	e.Dropped = true
	initiallyDropped := c.NewPending(ctx, e)
	c.Complete(ctx, initiallyDropped, nil)
	cancel()
	c.Run(ctx)

	svc := history.NewService(history.ServiceDeps{DB: dbForCapture})
	r, found, err := svc.EditedBy(t.Context(), original)
	if err != nil || !found || r.ID != edited || r.EditedFrom == nil || *r.EditedFrom != original ||
		r.Source != "intercept" || !r.Truncated || r.Resp == nil || r.Resp.Code != 200 {
		t.Fatalf("edited record = %+v, found %t, err %v", r, found, err)
	}
	for _, id := range []int64{dropped, initiallyDropped} {
		r, err = svc.Get(t.Context(), id)
		if err != nil || !r.Dropped || r.Resp != nil {
			t.Fatalf("dropped record = %+v, err %v", r, err)
		}
	}
	if _, found, err := svc.EditedBy(t.Context(), dropped); err != nil || found {
		t.Fatalf("edited record for dropped request: found %t, err %v", found, err)
	}
	r, err = svc.Get(t.Context(), droppedResponse)
	if err != nil || !r.Dropped || r.Resp == nil || r.Resp.Code != 200 {
		t.Fatalf("record for dropped response = %+v, err %v", r, err)
	}
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

func TestCaptureFailedResponseLeavesNoRequest(t *testing.T) {
	t.Parallel()

	db := openDB(t)
	c := history.New(history.Deps{DB: func() *store.DB { return db }})

	e := sample()
	e.Resp.Raw = nil // responses.raw is NOT NULL: the second insert fails

	ctx, cancel := context.WithCancel(t.Context())
	c.Add(ctx, e)
	cancel()
	c.Run(ctx)

	var n int
	if err := db.Reader().QueryRowContext(t.Context(), "SELECT count(*) FROM requests").Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows = %d, err = %v; want 0", n, err)
	}

	if c.Dropped() != 1 {
		t.Fatalf("dropped = %d, want 1", c.Dropped())
	}
}
