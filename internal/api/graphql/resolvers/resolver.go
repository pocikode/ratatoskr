// Package resolvers implements the GraphQL resolvers. They are hand-written (gqlgen
// generates only the executor, see gqlgen.yml) and stay thin: each one calls a
// service and maps its result (docs/03-architecture.md §6). One file per schema
// domain; contract stubs return [apperr.ErrNotImplemented].
package resolvers

import (
	"time"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
)

// BuildInfo describes the running binary for the serverInfo query.
type BuildInfo struct {
	Version   string
	Commit    string
	GoVersion string
}

// Deps are the services the resolvers call. Fields are added as services land.
type Deps struct {
	Build        BuildInfo
	StartedAt    time.Time
	AuthRequired bool
	// PingInterval is the period of the ping subscription; zero means one second.
	PingInterval time.Duration
	// Projects serves the project operations; nil reports NOT_IMPLEMENTED.
	Projects ProjectService
	// History serves the history operations; nil reports NOT_IMPLEMENTED.
	History HistoryService
	// Scopes serves the scope preset operations; nil reports NOT_IMPLEMENTED.
	Scopes ScopeService
	// Filters serves the filter preset operations; nil reports NOT_IMPLEMENTED.
	Filters FilterService
	// Intercept serves the intercept operations; until T-031 every intercept field reports NOT_IMPLEMENTED.
	Intercept InterceptService
	// Replay serves Replay collections and sessions; until T-033 those fields report NOT_IMPLEMENTED.
	Replay ReplayService
	// ReplaySend sends Replay requests; until T-106 send and cancel report NOT_IMPLEMENTED.
	ReplaySend ReplaySendService
	// Environment serves the environment operations; until T-037 those fields report NOT_IMPLEMENTED.
	Environment EnvironmentService
	// MatchReplace serves Match & Replace; until T-102 those fields report NOT_IMPLEMENTED.
	MatchReplace MatchReplaceService
}

// Resolver is the root resolver.
type Resolver struct {
	deps Deps
}

var _ generated.ResolverRoot = (*Resolver)(nil)

// New returns the root resolver.
func New(deps Deps) *Resolver {
	if deps.PingInterval <= 0 {
		deps.PingInterval = time.Second
	}

	return &Resolver{deps: deps}
}

// Query returns the Query resolver.
func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

// Mutation returns the Mutation resolver.
func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }

// Subscription returns the Subscription resolver.
func (r *Resolver) Subscription() generated.SubscriptionResolver { return &subscriptionResolver{r} }

type (
	queryResolver        struct{ *Resolver }
	mutationResolver     struct{ *Resolver }
	subscriptionResolver struct{ *Resolver }
)
