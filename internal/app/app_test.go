package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/config"
	"github.com/pocikode/ratatoskr/internal/logging"
)

// newTestApp builds an instance over a temp data dir. handler, when not nil,
// replaces the UI handler so a test can control request timing.
func newTestApp(t *testing.T, handler http.Handler) (*App, *bytes.Buffer) {
	t.Helper()

	var logs bytes.Buffer

	a, err := New(t.Context(), Options{
		Config: config.Config{Listen: []string{"127.0.0.1:0"}, DataDir: dataDir(t)},
		Logger: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if handler != nil {
		a.handler = handler
	}

	return a, &logs
}

// dataDir returns a fresh data dir path. t.TempDir itself is 0755, so the path is
// nested: openDataRoot creates the leaf with 0700, like a real install does.
func dataDir(t *testing.T) string {
	t.Helper()

	return t.TempDir() + "/data"
}

// listen returns a loopback listener on a random port.
func listen(t *testing.T) net.Listener {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	return ln
}

func TestNewRejectsUnimplementedOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Config
	}{
		{name: "separate ui listener", cfg: config.Config{Listen: []string{"127.0.0.1:0"}, UIListen: "127.0.0.1:9001"}},
		{
			name: "several listeners",
			cfg:  config.Config{Listen: []string{"127.0.0.1:0", "127.0.0.1:9001"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tt.cfg.DataDir = dataDir(t)

			a, err := New(t.Context(), Options{Config: tt.cfg})
			if err == nil {
				_ = a.Close()
				t.Fatalf("New(%+v) = nil error, want ErrNotImplemented", tt.cfg)
			}

			if !errors.Is(err, ErrNotImplemented) {
				t.Fatalf("New() error = %v, want ErrNotImplemented", err)
			}

			if !strings.Contains(err.Error(), "not implemented yet") {
				t.Errorf("New() error = %q, want it to say %q", err, "not implemented yet")
			}
		})
	}
}

func TestNewCreatesDataDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() + "/nested/data"

	a, err := New(t.Context(), Options{Config: config.Config{Listen: []string{"127.0.0.1:0"}, DataDir: dir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("data dir mode = %o, want 700", got)
	}

	root := a.DataRoot()
	if root == nil {
		t.Fatal("DataRoot() = nil, want the opened data dir")
	}

	// Every access below the data dir goes through the root.
	if err := root.WriteFile("probe", []byte("x"), 0o600); err != nil {
		t.Fatalf("write through root: %v", err)
	}

	if _, err := os.Stat(dir + "/probe"); err != nil {
		t.Errorf("file written through the root is missing: %v", err)
	}

	if _, err := root.Open("../escape"); err == nil {
		t.Error("opening ../escape through the root succeeded, want an error")
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("second Close = %v, want nil", err)
	}
}

func TestNewWarnsOnWideDataDirPermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	var logs bytes.Buffer

	a, err := New(t.Context(), Options{
		Config: config.Config{Listen: []string{"127.0.0.1:0"}, DataDir: dir},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	if !strings.Contains(logs.String(), "accessible to other users") {
		t.Errorf("logs = %q, want a warning about the data dir permissions", logs.String())
	}
}

func TestHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		debug      bool
		token      string
		bearer     string
		method     string
		path       string
		wantStatus []int
		wantBody   string
		notBody    []string
	}{
		// The SPA is only built by `make build`, so an unbuilt tree answers 503
		// instead of 200; both prove the request reached the UI handler.
		{name: "spa is mounted", path: "/", wantStatus: []int{http.StatusOK, http.StatusServiceUnavailable}},
		{name: "spa fallback route", path: "/history", wantStatus: []int{http.StatusOK, http.StatusServiceUnavailable}},
		// Without --debug the pprof path falls through to the SPA, so the body
		// must be neither a profile list nor any other pprof output.
		{
			name:       "pprof is off by default",
			path:       "/debug/pprof/",
			wantStatus: []int{http.StatusOK, http.StatusServiceUnavailable},
			notBody:    []string{"goroutineleak", "Types of profiles available"},
		},
		{name: "pprof index", debug: true, path: "/debug/pprof/", wantStatus: []int{http.StatusOK}, wantBody: "goroutineleak"},
		{name: "pprof goroutineleak", debug: true, path: "/debug/pprof/goroutineleak?debug=1", wantStatus: []int{http.StatusOK}, wantBody: "goroutineleak"},
		{name: "pprof symbol", debug: true, path: "/debug/pprof/symbol", wantStatus: []int{http.StatusOK}},
		// go tool pprof resolves addresses with POST /debug/pprof/symbol.
		{name: "pprof symbol accepts POST", debug: true, method: http.MethodPost, path: "/debug/pprof/symbol", wantStatus: []int{http.StatusOK}},
		// With --auth-token, pprof needs the token like the API does.
		{name: "pprof needs the token", debug: true, token: "s3cret", path: "/debug/pprof/cmdline", wantStatus: []int{http.StatusUnauthorized}},
		{name: "pprof with the token", debug: true, token: "s3cret", bearer: "s3cret", path: "/debug/pprof/", wantStatus: []int{http.StatusOK}, wantBody: "goroutineleak"},
		// The API is mounted under /api/ and guarded by the CSRF middleware.
		{name: "graphql needs the api header", method: http.MethodPost, path: "/api/graphql", wantStatus: []int{http.StatusUnsupportedMediaType}},
		{name: "unknown api path is 404, not the spa", path: "/api/nope", wantStatus: []int{http.StatusNotFound}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := New(t.Context(), Options{Config: config.Config{
				Listen:    []string{"127.0.0.1:0"},
				DataDir:   dataDir(t),
				Debug:     tt.debug,
				AuthToken: tt.token,
			}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			t.Cleanup(func() { _ = a.Close() })

			rec := httptest.NewRecorder()

			method := tt.method
			if method == "" {
				method = http.MethodGet
			}

			req := httptest.NewRequest(method, tt.path, nil)
			req.Host = "localhost:8080" // the Host check is on for a loopback listener (T-027)
			if tt.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tt.bearer)
			}

			a.handler.ServeHTTP(rec, req)

			if !slices.Contains(tt.wantStatus, rec.Code) {
				t.Fatalf("status = %d, want one of %v", rec.Code, tt.wantStatus)
			}

			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}

			for _, notBody := range tt.notBody {
				if strings.Contains(rec.Body.String(), notBody) {
					t.Errorf("body %q contains %q, want it absent", rec.Body.String(), notBody)
				}
			}
		})
	}
}

// TestHandlerHostAllowlist covers the DNS-rebinding Host check of a loopback
// listener (docs/03-architecture.md §8, T-027): a page on a rebinding domain has
// its own origin as Host, so it must be answered 421 before CSRF and auth.
func TestHandlerHostAllowlist(t *testing.T) {
	t.Parallel()

	a, err := New(t.Context(), Options{Config: config.Config{
		Listen:    []string{"127.0.0.1:0"},
		DataDir:   dataDir(t),
		Debug:     true,
		AuthToken: "s3cret",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	tests := []struct {
		name   string
		method string
		path   string
		host   string
		// ws sends the upgrade headers of the GraphQL WebSocket endpoint.
		ws         bool
		wantMisdir bool
	}{
		{"graphql", http.MethodPost, "/api/graphql", "evil.example:8080", false, true},
		{"graphql without the API headers", http.MethodPost, "/api/graphql", "evil.example", false, true},
		{"websocket", http.MethodGet, "/api/graphql/ws", "evil.example:8080", true, true},
		{"pprof", http.MethodGet, "/debug/pprof/", "evil.example:8080", false, true},
		{"spa", http.MethodGet, "/", "evil.example:8080", false, true},
		{"localhost", http.MethodPost, "/api/graphql", "localhost:8080", false, false},
		{"localhost without a port", http.MethodGet, "/api/graphql/ws", "localhost", true, false},
		{"ipv4 loopback", http.MethodPost, "/api/graphql", "127.0.0.1:8080", false, false},
		{"ipv6 loopback", http.MethodPost, "/api/graphql", "[::1]:8080", false, false},
		{"tool hostname", http.MethodGet, "/debug/pprof/", "ratatoskr.localhost:8080", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"query":"{ serverInfo { version } }"}`))
			req.Host = tt.host

			if tt.ws {
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Connection", "Upgrade")
			}

			rec := httptest.NewRecorder()
			a.handler.ServeHTTP(rec, req)

			if got := rec.Code == http.StatusMisdirectedRequest; got != tt.wantMisdir {
				t.Errorf("Host %q %s %s: status = %d, want 421 = %v", tt.host, tt.method, tt.path, rec.Code, tt.wantMisdir)
			}
		})
	}
}

// TestHandlerHostAllowlistOffWhenExposed: a listener bound to a non-loopback
// address is protected by --auth-token, so the Host check is off.
func TestHandlerHostAllowlistOffWhenExposed(t *testing.T) {
	t.Parallel()

	a, err := New(t.Context(), Options{Config: config.Config{
		Listen:  []string{"0.0.0.0:0"},
		DataDir: dataDir(t),
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example:8080"

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusMisdirectedRequest {
		t.Errorf("status = %d, want the Host check off for a non-loopback listener", rec.Code)
	}
}

func TestServeGracefulShutdownDrains(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})

	a, _ := newTestApp(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release

		w.WriteHeader(http.StatusOK)
	}))

	ln := listen(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.Serve(ctx, ln) }()

	reqErr := make(chan error, 1)

	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			reqErr <- err
			return
		}

		defer func() { _ = resp.Body.Close() }()

		_, err = io.Copy(io.Discard, resp.Body)
		reqErr <- err
	}()

	<-started
	cancel()
	close(release) // the in-flight request finishes during the drain

	if err := <-reqErr; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}

	if err := <-serveErr; err != nil {
		t.Fatalf("Serve() = %v, want nil after a drained shutdown", err)
	}
}

func TestServeShutdownDeadline(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	a, _ := newTestApp(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done() // ignore the drain: only Close stops this handler
	}))
	a.grace = 20 * time.Millisecond

	ln := listen(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.Serve(ctx, ln) }()

	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err == nil {
			_ = resp.Body.Close()
		}
	}()

	<-started

	start := time.Now()
	cancel()

	err := <-serveErr
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Serve() = %v, want a shutdown deadline error", err)
	}

	if elapsed > shutdownGrace {
		t.Errorf("shutdown took %v, want at most %v", elapsed, shutdownGrace)
	}
}

func TestWarnIfExposed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		token   string
		wantLog bool
	}{
		{name: "loopback", addr: "127.0.0.1:8080"},
		{name: "ipv6 loopback", addr: "[::1]:8080"},
		{name: "all interfaces", addr: "0.0.0.0:8080", wantLog: true},
		{name: "lan address", addr: "192.168.1.10:8080", wantLog: true},
		{name: "lan address with token", addr: "192.168.1.10:8080", token: "s3cret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, logs := newTestApp(t, nil)
			a.cfg.AuthToken = tt.token
			a.warnIfExposed(tt.addr)

			if got := strings.Contains(logs.String(), "reachable from the network"); got != tt.wantLog {
				t.Errorf("warned = %v, want %v (logs: %s)", got, tt.wantLog, logs.String())
			}
		})
	}
}

func TestLogs(t *testing.T) {
	t.Parallel()

	ring := logging.NewRing(logging.Options{Capacity: 4, Level: slog.LevelDebug})

	a, err := New(t.Context(), Options{
		Config: config.Config{Listen: []string{"127.0.0.1:0"}, DataDir: dataDir(t)},
		Logs:   ring,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	if a.Logs() != ring {
		t.Error("Logs() does not return the ring the instance was built with")
	}
}

func TestIsLoopback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:8080", want: true},
		{addr: "[::1]:8080", want: true},
		{addr: "localhost:8080", want: true},
		{addr: "0.0.0.0:8080", want: false},
		{addr: "[::]:8080", want: false},
		{addr: "192.168.1.10:8080", want: false},
		{addr: "not-an-address", want: false},
	}

	for _, tt := range tests {
		if got := isLoopback(tt.addr); got != tt.want {
			t.Errorf("isLoopback(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestProjectsAreWired(t *testing.T) {
	t.Parallel()

	a, _ := newTestApp(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/graphql",
		strings.NewReader(`{"query":"{ projects { id name isTemporary isSelected } }"}`))
	req.Host = "localhost:8080" // the Host check is on for a loopback listener (T-027)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ratatoskr", "1")

	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "NOT_IMPLEMENTED") || !strings.Contains(body, `"isTemporary":true`) {
		t.Fatalf("status %d, body %s; want the temporary project listed", rec.Code, body)
	}
}

func TestCloseRemovesTemporaryProject(t *testing.T) {
	t.Parallel()

	dir := dataDir(t)

	a, err := New(t.Context(), Options{Config: config.Config{Listen: []string{"127.0.0.1:0"}, DataDir: dir}})
	if err != nil {
		t.Fatal(err)
	}

	if entries, _ := os.ReadDir(dir + "/projects"); len(entries) != 1 {
		t.Fatalf("project dirs after New = %d, want 1", len(entries))
	}

	if err := a.Close(); err != nil {
		t.Fatal(err)
	}

	if entries, _ := os.ReadDir(dir + "/projects"); len(entries) != 0 {
		t.Errorf("project dirs after Close = %d, want 0", len(entries))
	}
}
