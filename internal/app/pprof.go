package app

import (
	"net/http"
	"net/http/pprof"

	"github.com/pocikode/ratatoskr/internal/api/middleware"
)

// mountDebug registers the profiling handlers on the UI/API mux. Importing
// net/http/pprof also registers them on http.DefaultServeMux, which this process
// never serves.
//
// pprof.Index serves every runtime/pprof profile by name, including the
// goroutineleak profile that the leak checks use
// (docs/05-go-conventions.md §1), so /debug/pprof/goroutineleak works without an
// extra route.
//
// The routes sit behind the same token auth as the API (auth), since profiles,
// heap dumps and cmdline must not leak on a host protected by --auth-token.
// With a token, fetch profiles with an "Authorization: Bearer" header.
func mountDebug(mux *http.ServeMux, auth middleware.Auth) {
	handle := func(pattern string, f http.HandlerFunc) { mux.Handle(pattern, auth.Handler(f)) }

	handle("GET /debug/pprof/", pprof.Index)
	handle("GET /debug/pprof/cmdline", pprof.Cmdline)
	handle("GET /debug/pprof/profile", pprof.Profile)
	// `go tool pprof` resolves addresses by POSTing to symbol, which also
	// answers GET with the ?0x… query form. Both are registered method-scoped so
	// they don't widen the other profiles.
	handle("GET /debug/pprof/symbol", pprof.Symbol)
	handle("POST /debug/pprof/symbol", pprof.Symbol)
	handle("GET /debug/pprof/trace", pprof.Trace)
}
