package resolvers

// Contract stubs for project.graphqls. T-006 replaces them with calls into
// internal/project.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func (r *queryResolver) Projects(context.Context) ([]generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) Project(context.Context, string) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) CurrentProject(context.Context) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateProject(context.Context, string, bool) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) RenameProject(context.Context, string, string) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DuplicateProject(context.Context, string, *string) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteProject(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) SetDefaultProject(context.Context, *string) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SelectProject(context.Context, string) (*generated.Project, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *subscriptionResolver) ProjectChanged(context.Context) (<-chan *generated.ProjectChange, error) {
	return nil, apperr.ErrNotImplemented
}
