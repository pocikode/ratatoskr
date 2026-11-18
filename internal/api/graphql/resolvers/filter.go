package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
)

// FilterService is what the filter preset resolvers need from [filterpreset.Service].
type FilterService interface {
	List(ctx context.Context) ([]filterpreset.Preset, error)
	Get(ctx context.Context, id int64) (filterpreset.Preset, error)
	Create(ctx context.Context, p filterpreset.Preset) (filterpreset.Preset, error)
	Update(ctx context.Context, p filterpreset.Preset) (filterpreset.Preset, error)
	Delete(ctx context.Context, id int64) error
	Reorder(ctx context.Context, ids []int64) error
	Export(ctx context.Context) ([]byte, error)
	Import(ctx context.Context, data []byte) (int, error)
}

var _ FilterService = (*filterpreset.Service)(nil)

func (r *Resolver) filters() (FilterService, error) {
	if r.deps.Filters == nil {
		return nil, apperr.ErrNotImplemented
	}

	return r.deps.Filters, nil
}

func toFilterPreset(p filterpreset.Preset) *generated.FilterPreset {
	out := &generated.FilterPreset{ID: strconv.FormatInt(p.ID, 10), Name: p.Name, Query: p.Query}
	if p.Alias != "" {
		out.Alias = &p.Alias
	}

	return out
}

func toFilterPresets(ps []filterpreset.Preset) []generated.FilterPreset {
	out := make([]generated.FilterPreset, len(ps))
	for i, p := range ps {
		out[i] = *toFilterPreset(p)
	}

	return out
}

func (r *queryResolver) FilterPresets(ctx context.Context) ([]generated.FilterPreset, error) {
	s, err := r.filters()
	if err != nil {
		return nil, err
	}

	ps, err := s.List(ctx)

	return toFilterPresets(ps), err
}

func (r *queryResolver) FilterPreset(ctx context.Context, id string) (*generated.FilterPreset, error) {
	s, err := r.filters()
	if err != nil {
		return nil, err
	}

	n, _ := parsePresetID("filter preset", id) // 0 on a bad ID: Get reports NOT_FOUND

	p, err := s.Get(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil //nolint:nilnil // unknown ID reads as null per the schema
	}

	if err != nil {
		return nil, err
	}

	return toFilterPreset(p), nil
}

func (r *queryResolver) ExportFilterPresets(ctx context.Context) (string, error) {
	s, err := r.filters()
	if err != nil {
		return "", err
	}

	b, err := s.Export(ctx)

	return string(b), err
}

func (r *mutationResolver) CreateFilterPreset(ctx context.Context, name string, alias *string, query string) (*generated.FilterPreset, error) {
	s, err := r.filters()
	if err != nil {
		return nil, err
	}

	p, err := s.Create(ctx, filterpreset.Preset{Name: name, Alias: deref(alias), Query: query})
	if err != nil {
		return nil, err
	}

	return toFilterPreset(p), nil
}

func (r *mutationResolver) UpdateFilterPreset(ctx context.Context, id, name string, alias *string, query string) (*generated.FilterPreset, error) {
	s, err := r.filters()
	if err != nil {
		return nil, err
	}

	n, err := parsePresetID("filter preset", id)
	if err != nil {
		return nil, err
	}

	p, err := s.Update(ctx, filterpreset.Preset{ID: n, Name: name, Alias: deref(alias), Query: query})
	if err != nil {
		return nil, err
	}

	return toFilterPreset(p), nil
}

func (r *mutationResolver) DeleteFilterPreset(ctx context.Context, id string) (string, error) {
	s, err := r.filters()
	if err != nil {
		return "", err
	}

	n, err := parsePresetID("filter preset", id)
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(n, 10), s.Delete(ctx, n)
}

func (r *mutationResolver) ReorderFilterPresets(ctx context.Context, ids []string) ([]generated.FilterPreset, error) {
	s, err := r.filters()
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

	return toFilterPresets(ps), err
}

func (r *mutationResolver) ImportFilterPresets(ctx context.Context, json string) (int, error) {
	s, err := r.filters()
	if err != nil {
		return 0, err
	}

	return s.Import(ctx, []byte(json))
}
