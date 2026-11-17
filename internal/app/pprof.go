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
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
}
