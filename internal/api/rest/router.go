// Package rest serves the small REST surface under /api/rest/ for binary
// transfers (docs/03-architecture.md §6). Routes land with their features
// (files, exports, backups, CA download, response preview).
package rest

import "net/http"

// Deps are the services the REST routes read from.
type Deps struct {
	// CA serves the root certificate downloads. Nil leaves them 404.
	CA Certificates
}

// NewRouter returns the /api/rest/ router. Paths whose service is nil in deps
// stay 404 until that feature lands.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	if deps.CA != nil {
		registerCA(mux, "/api/rest", deps.CA)
	}

	mux.Handle("/api/rest/", http.NotFoundHandler())

	return mux
}
