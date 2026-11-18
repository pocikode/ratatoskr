package resolvers

// Contract stubs for filter.graphqls (T-091); T-092 replaces them.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func (r *queryResolver) FilterPresets(context.Context) ([]generated.FilterPreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) FilterPreset(context.Context, string) (*generated.FilterPreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) ExportFilterPresets(context.Context) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateFilterPreset(context.Context, string, *string, string) (*generated.FilterPreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) UpdateFilterPreset(context.Context, string, string, *string, string) (*generated.FilterPreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteFilterPreset(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ReorderFilterPresets(context.Context, []string) ([]generated.FilterPreset, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) ImportFilterPresets(context.Context, string) (int, error) {
	return 0, apperr.ErrNotImplemented
}
