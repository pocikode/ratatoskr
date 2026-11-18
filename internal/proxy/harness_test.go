package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/ca"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/netx"
	"github.com/pocikode/ratatoskr/internal/proxy"
	"github.com/pocikode/ratatoskr/internal/store"
)

// recorder is a Capture that keeps entries in memory.
type recorder struct {
	mu      sync.Mutex
	entries []history.Entry
}

func (r *recorder) Add(_ context.Context, e history.Entry) {
	r.mu.Lock()
	r.entries = append(r.entries, e)
	r.mu.Unlock()
}

func (r *recorder) all() []history.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]history.Entry(nil), r.entries...)
}

// rig is a running proxy with a real CA.
type rig struct {
	addr string
	ca   *ca.Service
	rec  *recorder
	stop func() // shuts the proxy down; also runs at cleanup
}

// startProxy runs a proxy whose UI answers "ui:<path>" and which trusts any
// upstream certificate (the upstreams are httptest servers).
func startProxy(t testing.TB, opts ...func(*proxy.Deps)) *rig {
	t.Helper()

	return startProxyOn(t, "127.0.0.1:0", opts...)
}

func startProxyOn(t testing.TB, listen string, opts ...func(*proxy.Deps)) *rig {
	t.Helper()

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = inst.Close() })

	svc, err := ca.Open(t.Context(), ca.Deps{Instance: inst, Root: root})
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	pool := &netx.Pool{TLS: &tls.Config{InsecureSkipVerify: true}}
	t.Cleanup(pool.Close)

	deps := proxy.Deps{
		CA: svc, Pool: pool, Capture: rec,
		UI: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ui:" + r.URL.Path))
		}),
	}
	for _, o := range opts {
		o(&deps)
	}

	srv := proxy.New(deps)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- srv.Serve(ctx, ln) }()

	var once sync.Once

	stop := func() {
		once.Do(func() {
			cancel()

			if err := <-done; err != nil {
				t.Errorf("Serve: %v", err)
			}
		})
	}
	t.Cleanup(stop)

	return &rig{addr: ln.Addr().String(), ca: svc, rec: rec, stop: stop}
}

// client is an http.Client that goes through the proxy and trusts its CA.
func (r *rig) client(t testing.TB) *http.Client {
	t.Helper()

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(r.ca.CertPEM()) {
		t.Fatal("CA PEM not accepted")
	}

	proxyURL := &url.URL{Scheme: "http", Host: r.addr}
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: pool}}
	t.Cleanup(tr.CloseIdleConnections)

	return &http.Client{Transport: tr}
}
