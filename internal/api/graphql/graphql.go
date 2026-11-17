// Package graphql serves the GraphQL API: POST /api/graphql and the
// graphql-transport-ws endpoint /api/graphql/ws (ADR-0004,
// docs/03-architecture.md §6). Schema: ./schema; generated executor: ./generated;
// resolvers: ./resolvers.
package graphql

// gqlgen resolves gqlgen.yml paths against the working directory, so run it
// from the repository root.
//go:generate go -C ../../.. tool gqlgen generate

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/api/middleware"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

const (
	queryCacheSize = 1000
	// wsKeepAlive keeps idle subscription sockets open through proxies and lets
	// the client notice a dead server.
	wsKeepAlive   = 10 * time.Second
	wsInitTimeout = 10 * time.Second
	// CloseUnauthenticated is the WebSocket close code for a missing or wrong token.
	CloseUnauthenticated = 4401
)

// Options configure [NewHandler].
type Options struct {
	Resolvers generated.ResolverRoot
	// Auth decides whether a WebSocket without HTTP credentials may continue
	// after connection_init. Requests must already have passed Auth.Handler.
	Auth   middleware.Auth
	Logger *slog.Logger
}

// NewHandler returns the GraphQL handler. It accepts POST (JSON) and WebSocket
// (graphql-transport-ws) requests; the caller mounts the CSRF middleware.
func NewHandler(opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	srv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: opts.Resolvers}))

	srv.AddTransport(transport.Websocket{
		InitFunc:              wsInit(opts.Auth),
		InitTimeout:           wsInitTimeout,
		KeepAlivePingInterval: wsKeepAlive,
	})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](queryCacheSize))
	srv.Use(extension.Introspection{})

	srv.SetErrorPresenter(presentError(logger))
	srv.SetRecoverFunc(func(ctx context.Context, p any) error {
		logger.ErrorContext(ctx, "graphql resolver panic", "panic", p, "path", graphql.GetPath(ctx).String())

		return &apperr.Error{Code: apperr.Internal, Msg: "internal error"}
	})

	return srv
}

// wsInit authenticates a WebSocket that arrived without HTTP credentials by the
// "token" field of its connection_init payload.
func wsInit(auth middleware.Auth) transport.WebsocketInitFunc {
	return func(ctx context.Context, payload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
		if !auth.Required() || middleware.Authenticated(ctx) || auth.CheckToken(payload.GetString("token")) {
			return ctx, nil, nil
		}

		return transport.WithWebsocketCloseCode(ctx, CloseUnauthenticated), nil, errors.New("authentication required")
	}
}

// presentError sets extensions.code (and extensions.span for query errors) on
// every error, following docs/03-architecture.md §6:
//   - an *apperr.Error in the chain decides the code and the message;
//   - errors gqlgen already coded (parse/validation) pass through;
//   - other gqlerror values are argument errors from gqlgen (BAD_INPUT);
//   - anything else is INTERNAL: logged, with a generic message.
func presentError(logger *slog.Logger) graphql.ErrorPresenterFunc {
	return func(ctx context.Context, err error) *gqlerror.Error {
		presented := *graphql.DefaultErrorPresenter(ctx, err)
		presented.Extensions = cloneExtensions(presented.Extensions)

		_, isGQL := errors.AsType[*gqlerror.Error](err)

		switch appErr, ok := errors.AsType[*apperr.Error](err); {
		case ok:
			code := appErr.Code
			if code == "" {
				code = apperr.Internal
			}

			presented.Message = appErr.Error()
			if code == apperr.Internal {
				logger.ErrorContext(ctx, "graphql internal error", "error", err, "path", presented.Path.String())
				presented.Message = "internal error"
			}

			presented.Extensions["code"] = string(code)
			if appErr.Span != nil {
				presented.Extensions["span"] = map[string]int{"start": appErr.Span.Start, "end": appErr.Span.End}
			}
		case presented.Extensions["code"] != nil:
		case isGQL:
			presented.Extensions["code"] = string(apperr.BadInput)
		default:
			logger.ErrorContext(ctx, "graphql internal error", "error", err, "path", presented.Path.String())
			presented.Message = "internal error"
			presented.Extensions["code"] = string(apperr.Internal)
		}

		return &presented
	}
}

// cloneExtensions copies ext so that presenting never mutates an error gqlgen
// may hand out again (validation errors of cached queries).
func cloneExtensions(ext map[string]any) map[string]any {
	out := make(map[string]any, len(ext)+2)
	maps.Copy(out, ext)

	return out
}
