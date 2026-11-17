// Package rest serves the small REST surface under /api/rest/ for binary
// transfers (docs/03-architecture.md §6). Routes land with their features
// (files, exports, backups, CA download, response preview).
package rest

import "net/http"

// NewRouter returns the /api/rest/ router. Every path is 404 until routes land.
func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/rest/", http.NotFoundHandler())

	return mux
}
