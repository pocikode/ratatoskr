// Package app wires a Ratatoskr instance together: configuration, logging, the
// data dir and the HTTP servers, and runs them until the context is cancelled.
//
// The proxy joins in a later task; the store, the project registry and the CA
// receive their dependencies from App instead of package-level state
// (docs/03-architecture.md §2).
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"time"

	"github.com/pocikode/ratatoskr/internal/api"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/api/middleware"
	"github.com/pocikode/ratatoskr/internal/api/rest"
	"github.com/pocikode/ratatoskr/internal/api/static"
	"github.com/pocikode/ratatoskr/internal/ca"
	"github.com/pocikode/ratatoskr/internal/config"
	"github.com/pocikode/ratatoskr/internal/logging"
	"github.com/pocikode/ratatoskr/internal/project"
	"github.com/pocikode/ratatoskr/internal/store"
)

const (
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 1 << 20
	maxHeaderValues   = 512
	// shutdownGrace bounds how long in-flight requests may take once the context
	// is cancelled (T-004: SIGINT/SIGTERM shuts down within 5 s).
	shutdownGrace = 5 * time.Second
)

// ErrNotImplemented marks an option whose machinery lands in a later task. It is
// reported instead of silently ignoring the request, which for a security option
// like --auth-token would leave the operator with a false sense of protection.
var ErrNotImplemented = errors.New("not implemented yet")

// Options are the inputs to [New].
type Options struct {
	// Config is the resolved configuration. It is validated again by New.
	Config config.Config
	// Logger receives lifecycle events. Defaults to [slog.Default].
	Logger *slog.Logger
	// Logs is the ring buffer behind Logger, exposed to the Logs page. Optional.
	Logs *logging.Ring
	// Version and Commit describe the binary for the serverInfo query.
	// Empty values are reported as "dev" and "unknown".
	Version string
	Commit  string
}

// App is one configured Ratatoskr instance. Create it with [New], run it with
// [Run], and release it with [Close].
type App struct {
	cfg     config.Config
	log     *slog.Logger
	logs    *logging.Ring
	data    *os.Root
	inst    *store.DB
	project *project.Service
	ca      *ca.Service
	handler http.Handler

	// grace is the shutdown deadline; tests shorten it.
	grace time.Duration
}

// New prepares an instance: it rejects options that are not implemented yet,
// creates the data dir (0700), opens instance.db and the project service, and
// builds the UI/API handler.
//
// ctx only carries values: a cancelled ctx does not abort opening the databases.
func New(ctx context.Context, opts Options) (*App, error) {
	cfg := opts.Config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Options that need machinery from later tasks fail loudly: --ui-listen and
	// further --listen addresses need the proxy listener multiplexer.
	// --no-open needs no gate: no browser is opened yet.
	switch {
	case cfg.UIListen != "":
		return nil, fmt.Errorf("app: --ui-listen: %w (the proxy listener multiplexer lands with T-011)", ErrNotImplemented)
	case len(cfg.Listen) > 1:
		return nil, fmt.Errorf("app: %d --listen addresses: %w (the proxy listener multiplexer lands with T-011)",
			len(cfg.Listen), ErrNotImplemented)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	data, err := openDataRoot(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	a := &App{
		cfg:   cfg,
		log:   logger,
		logs:  opts.Logs,
		data:  data,
		grace: shutdownGrace,
	}

	if info, err := data.Stat("."); err == nil && info.Mode().Perm()&0o077 != 0 {
		a.log.Warn("data dir is accessible to other users",
			"data_dir", cfg.DataDir, "perm", info.Mode().Perm().String())
	}

	if err := a.openStores(context.WithoutCancel(ctx)); err != nil {
		_ = a.Close()

		return nil, err
	}

	a.handler = a.buildHandler(buildInfo(opts.Version, opts.Commit))

	return a, nil
}

// openStores opens instance.db and the project service (which selects the
// default project, or a temporary one).
func (a *App) openStores(ctx context.Context) error {
	inst, err := store.OpenInstance(ctx, a.data)
	if err != nil {
		return fmt.Errorf("app: open instance db: %w", err)
	}

	a.inst = inst

	svc, err := project.Open(ctx, project.Deps{Instance: inst, Root: a.data, Logger: a.log})
	if err != nil {
		return fmt.Errorf("app: open projects: %w", err)
	}

	a.project = svc

	caSvc, err := ca.Open(ctx, ca.Deps{Instance: inst, Root: a.data, Logger: a.log})
	if err != nil {
		return fmt.Errorf("app: open CA: %w", err)
	}

	a.ca = caSvc

	return nil
}

// Run listens on the configured address and serves until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	// New guarantees exactly one address until the multiplexer lands.
	addr := a.cfg.Listen[0]

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("app: listen on %q: %w", addr, err)
	}

	return a.Serve(ctx, ln)
}

// Serve serves on ln until ctx is cancelled, then drains in-flight requests for at
// most the shutdown grace. The listener is passed in so the bound address is known
// even for --listen 127.0.0.1:0, and so later tasks can multiplex it.
func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	addr := ln.Addr().String()

	a.log.Info("ratatoskr listening", "addr", addr, "config", a.cfg)
	a.warnIfExposed(addr)

	srv := &http.Server{
		Handler:             a.handler,
		ReadHeaderTimeout:   readHeaderTimeout,
		IdleTimeout:         idleTimeout,
		MaxHeaderBytes:      maxHeaderBytes,
		MaxHeaderValueCount: maxHeaderValues,
		ErrorLog:            slog.NewLogLogger(a.log.Handler(), slog.LevelWarn),
	}

	// Serve returns as soon as Shutdown or Close is called, so the goroutine
	// always ends with this function.
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("app: serve %s: %w", addr, err)
	case <-ctx.Done():
	}

	// ctx is already cancelled, so the drain needs a deadline of its own.
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.grace)
	defer cancel()

	if shutdownErr := srv.Shutdown(drainCtx); shutdownErr != nil {
		// Close cuts off the requests that ignored the deadline, which also
		// unblocks Serve.
		_ = srv.Close()

		if serveErr := <-serveErr; !errors.Is(serveErr, http.ErrServerClosed) {
			a.log.Warn("serve after a failed shutdown", "addr", addr, "error", serveErr)
		}

		return fmt.Errorf("app: shutdown %s: %w", addr, shutdownErr)
	}

	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("app: serve %s: %w", addr, err)
	}

	a.log.Info("ratatoskr stopped", "addr", addr)

	return nil
}

// Close closes the project service (deleting an unsaved temporary project),
// then instance.db, then the data dir. Run or Serve must have returned first.
func (a *App) Close() error {
	var errs []error

	if a.project != nil {
		if err := a.project.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close projects: %w", err))
		}

		a.project = nil
	}

	if a.inst != nil {
		if err := a.inst.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close instance db: %w", err))
		}

		a.inst = nil
	}

	if a.data != nil {
		if err := a.data.Close(); err != nil {
			errs = append(errs, fmt.Errorf("app: close data dir: %w", err))
		}

		a.data = nil
	}

	return errors.Join(errs...)
}

// DataRoot is the data dir, opened as an [os.Root]. The store, the CA and the
// project registry do all their file access through it
// (docs/05-go-conventions.md §1).
func (a *App) DataRoot() *os.Root {
	return a.data
}

// Logs is the in-memory log ring the Logs page streams from, or nil when the
// instance was created without one.
func (a *App) Logs() *logging.Ring {
	return a.logs
}

// buildHandler mounts the UI/API handler: the API under /api/, the embedded SPA,
// plus /debug/pprof on the same server when --debug is set
// (docs/03-architecture.md §6, §9).
func (a *App) buildHandler(build resolvers.BuildInfo) http.Handler {
	auth := middleware.Auth{Token: a.cfg.AuthToken, AllowGuests: a.cfg.AllowGuests}

	mux := http.NewServeMux()
	if a.cfg.Debug {
		mountDebug(mux, auth)
	}

	mux.Handle("/api/", api.NewHandler(api.Options{
		Resolvers: resolvers.Deps{Build: build, StartedAt: time.Now(), Projects: a.project},
		Auth:      auth,
		Logger:    a.log,
		CA:        a.ca,
	}))

	if a.ca != nil {
		// F-NET-2 also offers the certificate on the bare origin, where a browser
		// installs it: http://ratatoskr.localhost/ca.crt.
		caFiles := rest.CAHandler(a.ca)
		mux.Handle("GET /ca.crt", caFiles)
		mux.Handle("GET /ca.der", caFiles)
	}

	mux.Handle("/", static.Handler())

	return mux
}

// buildInfo fills in the defaults for an unstamped binary.
func buildInfo(version, commit string) resolvers.BuildInfo {
	if version == "" {
		version = "dev"
	}

	if commit == "" {
		commit = "unknown"
	}

	return resolvers.BuildInfo{Version: version, Commit: commit, GoVersion: runtime.Version()}
}

// openDataRoot creates dir with 0700 (if needed) and opens it as an [os.Root], so
// every later access below the data dir is path-traversal safe.
func openDataRoot(dir string) (*os.Root, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("app: create data dir %q: %w", dir, err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("app: open data dir %q: %w", dir, err)
	}

	return root, nil
}

// isLoopback reports whether addr only accepts local connections.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	if host == "localhost" {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.IsLoopback()
}

// warnIfExposed warns when addr is reachable from the network without
// --auth-token (docs/03-architecture.md §8).
func (a *App) warnIfExposed(addr string) {
	if isLoopback(addr) || a.cfg.AuthToken != "" {
		return
	}

	a.log.Warn("UI/API is reachable from the network without authentication; set --auth-token", "addr", addr)
}
