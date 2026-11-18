package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pocikode/ratatoskr/internal/history"
)

// TestProxyCapturesIntoProject checks the whole wiring: a proxied request ends
// up in the open project's database and is announced on the event bus.
func TestProxyCapturesIntoProject(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t, nil)

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hi")) }))
	t.Cleanup(target.Close)

	ln := listen(t)

	ctx, cancel := context.WithCancel(t.Context())
	sub := a.Events().Subscribe(ctx)
	done := make(chan error, 1)

	go func() { done <- a.Serve(ctx, ln) }()

	proxyURL := &url.URL{Scheme: "http", Host: ln.Addr().String()}
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	resp, err := c.Get(target.URL + "/p?q=1")
	if err != nil {
		t.Fatal(err)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if m := <-sub; m.Value.Kind != history.Added {
		t.Fatalf("event = %+v, want Added", m)
	}

	var path, query string
	if err := a.project.DB().Reader().QueryRowContext(ctx, "SELECT path, query FROM requests").Scan(&path, &query); err != nil {
		t.Fatal(err)
	}

	if path != "/p" || query != "q=1" {
		t.Fatalf("stored %q %q", path, query)
	}

	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}
