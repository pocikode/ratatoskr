package app

import (
	"net/http"
	"net/http/pprof"
)

// mountDebug registers the profiling handlers on the UI/API mux. Importing
// net/http/pprof also registers them on http.DefaultServeMux, which this process
// never serves.
//
// pprof.Index serves every runtime/pprof profile by name, including the
// goroutineleak profile that the leak checks use
// (docs/05-go-conventions.md §1), so /debug/pprof/goroutineleak works without an
// extra route.
func mountDebug(mux *http.ServeMux) {
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	// cmdline exposes the process arguments, and --debug may bind a
	// non-loopback address: authentication has to cover /debug/pprof before
	// --auth-token is un-gated.
	//
	// TODO(T-007): wrap /debug/pprof in the auth middleware (or drop cmdline).
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	// `go tool pprof` resolves addresses by POSTing to symbol, which also
	// answers GET with the ?0x… query form. Both are registered method-scoped so
	// they don't widen the other profiles.
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}
