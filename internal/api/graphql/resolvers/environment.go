package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/environment"
)

// EnvironmentService is the environment storage used by the GraphQL API.
type EnvironmentService interface {
	List(context.Context) ([]environment.Environment, error)
	Get(context.Context, int64) (environment.Environment, error)
	Create(context.Context, string) (environment.Environment, error)
	Rename(context.Context, int64, string) (environment.Environment, error)
	Delete(context.Context, int64) error
	SetVariables(context.Context, int64, []environment.Variable) (environment.Environment, error)
	Select(context.Context, *int64) (*environment.Environment, error)
}

var _ EnvironmentService = (*environment.Service)(nil)

func (r *Resolver) environments() (EnvironmentService, error) {
	if r.deps.Environment == nil {
		return nil, apperr.ErrNotImplemented
	}
	return r.deps.Environment, nil
}

func environmentID(id string) (int64, error) {
	if id == "global" {
		return environment.GlobalID, nil
	}
	return parsePresetID("environment", id)
}

func toEnvironment(e environment.Environment) *generated.Environment {
	id := strconv.FormatInt(e.ID, 10)
	if e.IsGlobal {
		id = "global"
	}
	vars := make([]generated.EnvironmentVariable, len(e.Variables))
	for i, v := range e.Variables {
		vars[i] = generated.EnvironmentVariable{Name: v.Name, Value: v.Value, Secret: v.Secret}
	}
	return &generated.Environment{ID: id, Name: e.Name, IsGlobal: e.IsGlobal, Selected: e.Selected, Variables: vars}
}

func (r *queryResolver) Environments(ctx context.Context) ([]generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	es, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]generated.Environment, len(es))
	for i, e := range es {
		out[i] = *toEnvironment(e)
	}
	return out, nil
}

func (r *queryResolver) Environment(ctx context.Context, id string) (*generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	n, err := environmentID(id)
	if err != nil {
		n = -1 // Invalid IDs must not alias Global (zero).
	}
	e, err := s.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toEnvironment(e), nil
}

func (r *mutationResolver) CreateEnvironment(ctx context.Context, name string) (*generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	e, err := s.Create(ctx, name)
	if err != nil {
		return nil, err
	}
	return toEnvironment(e), nil
}

func (r *mutationResolver) RenameEnvironment(ctx context.Context, id, name string) (*generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	n, err := environmentID(id)
	if err != nil {
		return nil, err
	}
	e, err := s.Rename(ctx, n, name)
	if err != nil {
		return nil, err
	}
	return toEnvironment(e), nil
}

func (r *mutationResolver) DeleteEnvironment(ctx context.Context, id string) (string, error) {
	s, err := r.environments()
	if err != nil {
		return "", err
	}
	n, err := environmentID(id)
	if err != nil {
		return "", err
	}
	return id, s.Delete(ctx, n)
}

func (r *mutationResolver) SetEnvironmentVariables(ctx context.Context, id string, vars []generated.EnvironmentVariableInput) (*generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	n, err := environmentID(id)
	if err != nil {
		return nil, err
	}
	values := make([]environment.Variable, len(vars))
	for i, v := range vars {
		values[i] = environment.Variable{Name: v.Name, Value: v.Value, Secret: v.Secret}
	}
	e, err := s.SetVariables(ctx, n, values)
	if err != nil {
		return nil, err
	}
	return toEnvironment(e), nil
}

func (r *mutationResolver) SelectEnvironment(ctx context.Context, id *string) (*generated.Environment, error) {
	s, err := r.environments()
	if err != nil {
		return nil, err
	}
	var n *int64
	if id != nil {
		parsed, err := environmentID(*id)
		if err != nil {
			return nil, err
		}
		n = &parsed
	}
	e, err := s.Select(ctx, n)
	if err != nil || e == nil {
		return nil, err
	}
	return toEnvironment(*e), nil
}
