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
