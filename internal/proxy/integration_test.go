//go:build integration

package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"
)

const (
	loadClients  = 200
	loadRequests = 50
)

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
