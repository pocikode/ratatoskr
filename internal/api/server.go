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

	mux := http.NewServeMux()
	mux.Handle("POST /api/graphql", middleware.RequireAPIRequest(gql))
	mux.HandleFunc("/api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
	})
	mux.Handle("GET /api/graphql/ws", gql)
	mux.Handle("/api/rest/", rest.NewRouter())
	mux.Handle("/api/", http.NotFoundHandler())

	return middleware.CrossOrigin(opts.Auth.Handler(mux))
}
