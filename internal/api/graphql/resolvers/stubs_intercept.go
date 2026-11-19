package resolvers

// Contract stubs for intercept.graphqls (T-030). T-031 replaces this file with intercept.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// InterceptService serves the intercept operations. Placeholder until T-031 defines its methods.
type InterceptService any

func (r *queryResolver) InterceptSettings(context.Context) (*generated.InterceptSettings, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) InterceptQueue(context.Context, *generated.InterceptItemKind) ([]generated.InterceptItem, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) UpdateInterceptSettings(
	context.Context, generated.InterceptSettingsInput,
) (*generated.InterceptSettings, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) ForwardInterceptItem(context.Context, string, []byte, bool) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) DropInterceptItem(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ForwardAllInterceptItems(context.Context, *generated.InterceptItemKind) (int, error) {
	return 0, apperr.ErrNotImplemented
}

func (r *mutationResolver) DropAllInterceptItems(context.Context, *generated.InterceptItemKind) (int, error) {
	return 0, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) InterceptQueueChanged(context.Context) (<-chan *generated.InterceptQueueChange, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) InterceptSettingsChanged(context.Context) (<-chan *generated.InterceptSettings, error) {
	return nil, apperr.ErrNotImplemented
}
