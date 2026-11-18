package resolvers

// Contract stubs for scope.graphqls (T-091); T-092 replaces them.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func (r *queryResolver) ScopePresets(context.Context) ([]generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) ScopePreset(context.Context, string) (*generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateScopePreset(context.Context, string, []string, []string) (*generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) UpdateScopePreset(context.Context, string, string, []string, []string) (*generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DuplicateScopePreset(context.Context, string) (*generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteScopePreset(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ReorderScopePresets(context.Context, []string) ([]generated.ScopePreset, error) {
	return nil, apperr.ErrNotImplemented
}
