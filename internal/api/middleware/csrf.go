// Package middleware holds the HTTP middleware of the UI/API server: CSRF
// defences and token auth (docs/03-architecture.md §8).
package middleware

import (
	"mime"
	"net/http"
)

// APIHeader is the custom header every GraphQL HTTP request must carry. Browsers
// do not let a cross-site form or simple request set it, and setting it from
// script on another origin triggers a CORS preflight that we never approve.
const APIHeader = "X-Ratatoskr"

// CrossOrigin rejects non-safe cross-origin browser requests using
// [http.CrossOriginProtection] (Sec-Fetch-Site, then Origin vs Host).
func CrossOrigin(next http.Handler) http.Handler {
	return http.NewCrossOriginProtection().Handler(next)
}

// RequireAPIRequest rejects requests that a cross-site page could send without a
// preflight: the media type must be application/json (415 otherwise) and the
// [APIHeader] header must be "1" (403 otherwise).
func RequireAPIRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}

		if r.Header.Get(APIHeader) != "1" {
			writeError(w, http.StatusForbidden, "missing "+APIHeader+": 1 header")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// writeError writes a plain-text error. These responses happen before GraphQL
// runs, so they are not GraphQL-shaped.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg + "\n"))
}
