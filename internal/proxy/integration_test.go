//go:build integration

package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/proxy"
	"github.com/pocikode/ratatoskr/internal/store"
)

const (
	loadClients  = 200
	loadRequests = 50
)

func TestHistoryListsPendingBeforeUpstreamResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	db, err := store.OpenProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bus := &events.Bus[history.Event]{}
	dbForCapture := func() *store.DB { return db }
	capture := history.New(history.Deps{DB: dbForCapture, Bus: bus})
	svc := history.NewService(history.ServiceDeps{DB: dbForCapture})
	sub := bus.Subscribe(ctx)
	runCtx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { capture.Run(runCtx); close(done) }()
	t.Cleanup(func() { stop(); <-done })

	release := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, err := io.ReadAll(r.Body); err != nil || string(body) != "upload" {
			t.Errorf("upstream body = %q, %v", body, err)
		}
		select {
		case <-release:
			_, _ = io.WriteString(w, "answer")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(up.Close)
	r := startProxy(t, func(d *proxy.Deps) { d.Capture = capture })
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	reqWire := fmt.Sprintf("POST %s/slow HTTP/1.1\r\nHost: %s\r\nContent-Length: 6\r\nConnection: close\r\n\r\nupload",
		up.URL, up.Listener.Addr())
	if _, err := io.WriteString(conn, reqWire); err != nil {
		t.Fatal(err)
	}

	next := func(kind history.Kind) int64 {
		t.Helper()
		select {
		case event := <-sub:
			if event.Value.Kind != kind {
				t.Fatalf("event = %+v, want kind %v", event, kind)
			}
			return event.Value.RequestID
		case <-ctx.Done():
			t.Fatalf("waiting for event %v: %v", kind, ctx.Err())
			return 0
		}
	}
	id := next(history.Added)
	page, err := svc.List(ctx, history.PageArgs{})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != id || page.Items[0].Resp != nil {
		t.Fatalf("pending page = %+v, err %v", page, err)
	}
	releasedAt := time.Now()
	close(release)
	answerWire, err := io.ReadAll(conn)
	if err != nil {
		t.Fatal(err)
	}
	if updated := next(history.Updated); updated != id {
		t.Fatalf("updated ID = %d, want %d", updated, id)
	}

	got, err := svc.Get(ctx, id)
	if err != nil || got.Resp == nil || got.Resp.Code != 200 || got.CreatedAt != page.Items[0].CreatedAt ||
		got.Resp.CreatedAt.Before(releasedAt.Truncate(time.Millisecond)) ||
		got.Resp.RoundTripMs < releasedAt.Sub(got.CreatedAt.Add(time.Millisecond)).Milliseconds() {
		t.Fatalf("completed exchange = %+v, response = %+v, err %v", got, got.Resp, err)
	}

	for side, want := range []string{reqWire, string(answerWire)} {
		head, headErr := svc.Head(ctx, id, history.Part(side))
		body, bodyErr := svc.Body(ctx, id, history.Part(side), 0, len(want), false)
		if headErr != nil || bodyErr != nil || string(head)+string(body) != want {
			t.Fatalf("side %d = %q%q, want %q, errors %v, %v", side, head, body, want, headErr, bodyErr)
		}
	}
}

// TestLoadNoLeakAndLatency is T-011's acceptance run: 200 clients x 50
// requests, no goroutine left behind after shutdown, and the proxy adding less
// than 5 ms at p99 over a direct request.
func TestLoadNoLeakAndLatency(t *testing.T) {
	before := runtime.NumGoroutine()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))

	r := startProxy(t)

	var wg sync.WaitGroup

	// A connect storm overflows the OS listen backlog (somaxconn 128 on macOS)
	// before the proxy is even involved, so connects are admitted 32 at a time.
	// All 200 clients stay active afterwards.
	admit := make(chan struct{}, 32)

	for range loadClients {
		wg.Go(func() {
			c := r.client(t)
			for i := range loadRequests {
				if i == 0 {
					admit <- struct{}{}
				}

				resp, err := c.Get(s.URL)
				if i == 0 {
					<-admit
				}

				if err != nil {
					t.Error(err)

					return
				}

				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		})
	}

	wg.Wait()

	if n := len(r.rec.all()); n != loadClients*loadRequests {
		t.Errorf("captured %d exchanges, want %d", n, loadClients*loadRequests)
	}

	// Latency: sequential requests direct vs through the proxy.
	direct, via := sample(t, s.Client(), s.URL), sample(t, r.client(t), s.URL)
	if added := via - direct; added > 5*time.Millisecond {
		t.Errorf("p99 added latency %v (direct %v, proxy %v), want < 5ms", added, direct, via)
	}

	s.Close()
	r.stop()

	// Everything the test started must wind down.
	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}

	if n := runtime.NumGoroutine(); n > before+2 {
		buf := make([]byte, 1<<20)
		t.Fatalf("goroutines: %d before, %d after\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}

func sample(t *testing.T, c *http.Client, url string) time.Duration {
	t.Helper()

	const n = 500

	durs := make([]time.Duration, 0, n)

	for range n {
		start := time.Now()

		resp, err := c.Get(url)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		durs = append(durs, time.Since(start))
	}

	slices.Sort(durs)

	return durs[n*99/100]
}
