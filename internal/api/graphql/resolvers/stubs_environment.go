package resolvers

// Contract stubs for environment.graphqls (T-098). T-037 replaces this file with environment.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// EnvironmentService serves the environment operations. Placeholder until T-037 defines its methods.
type EnvironmentService any

func (r *queryResolver) Environments(context.Context) ([]generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) Environment(context.Context, string) (*generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateEnvironment(context.Context, string) (*generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) RenameEnvironment(context.Context, string, string) (*generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteEnvironment(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) SetEnvironmentVariables(
	context.Context, string, []generated.EnvironmentVariableInput,
) (*generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SelectEnvironment(context.Context, *string) (*generated.Environment, error) {
	return nil, apperr.ErrNotImplemented
}
