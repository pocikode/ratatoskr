// Package api assembles the HTTP API under /api/: GraphQL (POST /api/graphql,
// WebSocket /api/graphql/ws) and REST (/api/rest/), behind the CSRF and auth
// middleware (docs/03-architecture.md §6, §8). It depends on services, never the
// reverse.
package api

import (
	"log/slog"
	"net/http"

	"github.com/pocikode/ratatoskr/internal/api/graphql"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/api/middleware"
	"github.com/pocikode/ratatoskr/internal/api/rest"
)

// MaxGraphQLRequestBytes caps the body of POST /api/graphql. It leaves room for
// base64 request bodies sent to Replay/Automate (docs/03-architecture.md §6);
// WebSocket messages are capped separately by gqlgen.
const MaxGraphQLRequestBytes = 32 << 20

// Options configure [NewHandler].
type Options struct {
	Resolvers resolvers.Deps
	Auth      middleware.Auth
	Logger    *slog.Logger
}

// NewHandler returns the handler for everything under /api/. Mount it at "/api/".
func NewHandler(opts Options) http.Handler {
	opts.Resolvers.AuthRequired = opts.Auth.Required()

	gql := graphql.NewHandler(graphql.Options{
		Resolvers: resolvers.New(opts.Resolvers),
		Auth:      opts.Auth,
		Logger:    opts.Logger,
	})

	// Everything but the WebSocket endpoint requires credentials on the HTTP
	// request itself.
	strict := http.NewServeMux()
	strict.Handle("POST /api/graphql",
		limitBody(middleware.RequireAPIRequest(gql), MaxGraphQLRequestBytes))
	strict.HandleFunc("/api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
	})
	strict.Handle("/api/rest/", rest.NewRouter())
	strict.Handle("/api/", http.NotFoundHandler())

	// The WebSocket endpoint alone may be opened without credentials: the token
	// can arrive in connection_init (docs/03-architecture.md §8).
	mux := http.NewServeMux()
	mux.Handle("GET /api/graphql/ws", opts.Auth.WebSocket(gql))
	mux.Handle("/api/", opts.Auth.Handler(strict))

	return middleware.CrossOrigin(mux)
}

// limitBody answers 413 to a request whose declared length exceeds limit and
// caps the read of any other body at limit (a longer chunked body then fails
// as a GraphQL error).
func limitBody(next http.Handler, limit int64) http.Handler {
	capped := http.MaxBytesHandler(next, limit)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > limit {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)

			return
		}

		capped.ServeHTTP(w, r)
	})
}
