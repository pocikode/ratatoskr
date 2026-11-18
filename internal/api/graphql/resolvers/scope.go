package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/scope"
)

// ScopeService is what the scope resolvers need from [scope.Service].
type ScopeService interface {
	List(ctx context.Context) ([]scope.Preset, error)
	Get(ctx context.Context, id int64) (scope.Preset, error)
	Create(ctx context.Context, p scope.Preset) (scope.Preset, error)
	Update(ctx context.Context, p scope.Preset) (scope.Preset, error)
	Duplicate(ctx context.Context, id int64) (scope.Preset, error)
	Delete(ctx context.Context, id int64) error
	Reorder(ctx context.Context, ids []int64) error
}

var _ ScopeService = (*scope.Service)(nil)

func (r *Resolver) scopes() (ScopeService, error) {
	if r.deps.Scopes == nil {
		return nil, apperr.ErrNotImplemented
	}

	return r.deps.Scopes, nil
}

// parsePresetID maps a GraphQL ID to a preset row ID; anything else is NOT_FOUND.
func parsePresetID(kind, id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, apperr.New(apperr.NotFound, "%s %q not found", kind, id)
	}

	return n, nil
}

// parseOrderIDs is for reorder: an unparsable ID can never be a current preset, so BAD_INPUT.
func parseOrderIDs(ids []string) ([]int64, error) {
	out := make([]int64, len(ids))

	for i, id := range ids {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, apperr.New(apperr.BadInput, "unknown preset %q in order", id)
		}

		out[i] = n
	}

	return out, nil
}

func toScopePreset(p scope.Preset) *generated.ScopePreset {
	return &generated.ScopePreset{ID: strconv.FormatInt(p.ID, 10), Name: p.Name, Allow: p.Allow, Deny: p.Deny}
}

func toScopePresets(ps []scope.Preset) []generated.ScopePreset {
	out := make([]generated.ScopePreset, len(ps))
	for i, p := range ps {
		out[i] = *toScopePreset(p)
	}

	return out
}

func (r *queryResolver) ScopePresets(ctx context.Context) ([]generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	ps, err := s.List(ctx)

	return toScopePresets(ps), err
}

func (r *queryResolver) ScopePreset(ctx context.Context, id string) (*generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	n, _ := parsePresetID("scope", id) // 0 on a bad ID: Get reports NOT_FOUND

	p, err := s.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil //nolint:nilnil // unknown ID reads as null per the schema
	}

	if err != nil {
		return nil, err
	}

	return toScopePreset(p), nil
}

func (r *mutationResolver) CreateScopePreset(ctx context.Context, name string, allow, deny []string) (*generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	p, err := s.Create(ctx, scope.Preset{Name: name, Allow: allow, Deny: deny})
	if err != nil {
		return nil, err
	}

	return toScopePreset(p), nil
}

func (r *mutationResolver) UpdateScopePreset(ctx context.Context, id, name string, allow, deny []string) (*generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	n, err := parsePresetID("scope", id)
	if err != nil {
		return nil, err
	}

	p, err := s.Update(ctx, scope.Preset{ID: n, Name: name, Allow: allow, Deny: deny})
	if err != nil {
		return nil, err
	}

	return toScopePreset(p), nil
}

func (r *mutationResolver) DuplicateScopePreset(ctx context.Context, id string) (*generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	n, err := parsePresetID("scope", id)
	if err != nil {
		return nil, err
	}

	p, err := s.Duplicate(ctx, n)
	if err != nil {
		return nil, err
	}

	return toScopePreset(p), nil
}

func (r *mutationResolver) DeleteScopePreset(ctx context.Context, id string) (string, error) {
	s, err := r.scopes()
	if err != nil {
		return "", err
	}

	n, err := parsePresetID("scope", id)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(n, 10), s.Delete(ctx, n)
}

func (r *mutationResolver) ReorderScopePresets(ctx context.Context, ids []string) ([]generated.ScopePreset, error) {
	s, err := r.scopes()
	if err != nil {
		return nil, err
	}

	order, err := parseOrderIDs(ids)
	if err != nil {
		return nil, err
	}

	if err := s.Reorder(ctx, order); err != nil {
		return nil, err
	}

	ps, err := s.List(ctx)

	return toScopePresets(ps), err
}
