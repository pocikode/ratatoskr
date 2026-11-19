package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/intercept"
	"github.com/pocikode/ratatoskr/internal/store"
)

// InterceptService is the queue and settings API used by the intercept resolvers.
type InterceptService interface {
	IsCurrent(*store.DB) bool
	Settings(context.Context) (intercept.Settings, error)
	UpdateSettings(context.Context, intercept.Update) (intercept.Settings, error)
	Queue(intercept.Kind) []intercept.Item
	Forward(string, []byte, bool) error
	Drop(string) error
	ReleaseAll(intercept.Kind, bool) int
	SubscribeQueue(context.Context) <-chan intercept.QueueChange
	SubscribeSettings(context.Context) <-chan intercept.Settings
}

var _ InterceptService = (*intercept.Service)(nil)

func toInterceptSettings(s intercept.Settings) *generated.InterceptSettings {
	out := &generated.InterceptSettings{
		Queuing: s.Queuing, Requests: s.Requests, Responses: s.Responses,
		Query: s.Query, DropStatusCode: s.DropStatusCode,
	}
	if s.ScopeID != 0 {
		out.ScopeID = new(strconv.FormatInt(s.ScopeID, 10))
	}

	return out
}

func interceptKind(k *generated.InterceptItemKind) intercept.Kind {
	if k == nil {
		return ""
	}

	return intercept.Kind(*k)
}

// interceptItem loads the history record only while the item's DB is selected.
func (r *Resolver) interceptItem(ctx context.Context, item intercept.Item) (*generated.InterceptItem, error) {
	svc := r.deps.Intercept
	if !svc.IsCurrent(item.DB) {
		return nil, nil //nolint:nilnil // T-031: the project switched before history could be read.
	}
	history, err := r.history()
	if err != nil {
		return nil, err
	}
	req, err := history.Get(ctx, item.RequestID)
	if !svc.IsCurrent(item.DB) {
		return nil, nil //nolint:nilnil // T-031: discard reads that raced a project switch.
	}
	if err != nil {
		return nil, err
	}

	return &generated.InterceptItem{
		ID: item.ID, Kind: generated.InterceptItemKind(item.Kind),
		Request: toHTTPRequest(req), Raw: item.Raw, HeldAt: item.HeldAt,
	}, nil
}

func (r *queryResolver) InterceptSettings(ctx context.Context) (*generated.InterceptSettings, error) {
	if r.deps.Intercept == nil {
		return nil, apperr.ErrNotImplemented
	}
	settings, err := r.deps.Intercept.Settings(ctx)
	if err != nil {
		return nil, err
	}

	return toInterceptSettings(settings), nil
}

func (r *queryResolver) InterceptQueue(ctx context.Context, kind *generated.InterceptItemKind) ([]generated.InterceptItem, error) {
	svc := r.deps.Intercept
	if svc == nil {
		return nil, apperr.ErrNotImplemented
	}

	items := svc.Queue(interceptKind(kind))
	out := make([]generated.InterceptItem, 0, len(items))
	for _, item := range items {
		view, err := r.interceptItem(ctx, item)
		if err != nil {
			return nil, err
		}
		if view != nil {
			out = append(out, *view)
		}
	}
	if len(items) > 0 && !svc.IsCurrent(items[0].DB) {
		return out[:0], nil
	}

	return out, nil
}

func (r *mutationResolver) UpdateInterceptSettings(ctx context.Context, input generated.InterceptSettingsInput) (*generated.InterceptSettings, error) {
	svc := r.deps.Intercept
	if svc == nil {
		return nil, apperr.ErrNotImplemented
	}

	change := intercept.Update{
		Queuing: input.Queuing, Requests: input.Requests, Responses: input.Responses,
		Query: input.Query, DropStatusCode: input.DropStatusCode,
	}
	if id, set := input.ScopeID.ValueOK(); set {
		change.ScopeID = new(int64)
		if id != nil {
			n, err := strconv.ParseInt(*id, 10, 64)
			if err != nil || n <= 0 {
				return nil, apperr.New(apperr.NotFound, "scope %q not found", *id)
			}
			*change.ScopeID = n
		}
	}

	settings, err := svc.UpdateSettings(ctx, change)
	if err != nil {
		return nil, err
	}

	return toInterceptSettings(settings), nil
}

func (r *mutationResolver) ForwardInterceptItem(_ context.Context, id string, raw []byte, updateContentLength bool) (string, error) {
	if r.deps.Intercept == nil {
		return "", apperr.ErrNotImplemented
	}
	if err := r.deps.Intercept.Forward(id, raw, updateContentLength); err != nil {
		return "", err
	}

	return id, nil
}

func (r *mutationResolver) DropInterceptItem(_ context.Context, id string) (string, error) {
	if r.deps.Intercept == nil {
		return "", apperr.ErrNotImplemented
	}
	if err := r.deps.Intercept.Drop(id); err != nil {
		return "", err
	}

	return id, nil
}

func (r *mutationResolver) ForwardAllInterceptItems(_ context.Context, kind *generated.InterceptItemKind) (int, error) {
	if r.deps.Intercept == nil {
		return 0, apperr.ErrNotImplemented
	}

	return r.deps.Intercept.ReleaseAll(interceptKind(kind), false), nil
}

func (r *mutationResolver) DropAllInterceptItems(_ context.Context, kind *generated.InterceptItemKind) (int, error) {
	if r.deps.Intercept == nil {
		return 0, apperr.ErrNotImplemented
	}

	return r.deps.Intercept.ReleaseAll(interceptKind(kind), true), nil
}

func (r *subscriptionResolver) InterceptQueueChanged(ctx context.Context) (<-chan *generated.InterceptQueueChange, error) {
	svc := r.deps.Intercept
	if svc == nil {
		return nil, apperr.ErrNotImplemented
	}
	if _, err := r.history(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	src := svc.SubscribeQueue(ctx)
	out := make(chan *generated.InterceptQueueChange)
	go func() {
		defer cancel()
		defer close(out)

		for e := range src {
			item, err := r.interceptItem(ctx, e.Item)
			if err != nil {
				return
			}
			if item == nil {
				continue
			}

			change := &generated.InterceptQueueChange{Kind: generated.InterceptQueueChangeKindRemoved, Item: item}
			if e.Added {
				change.Kind = generated.InterceptQueueChangeKindAdded
			}
			select {
			case <-ctx.Done():
				return
			case <-e.Item.ProjectDone():
			case out <- change:
			}
		}
	}()

	return out, nil
}

func (r *subscriptionResolver) InterceptSettingsChanged(ctx context.Context) (<-chan *generated.InterceptSettings, error) {
	if r.deps.Intercept == nil {
		return nil, apperr.ErrNotImplemented
	}

	return forward(ctx, r.deps.Intercept.SubscribeSettings(ctx), toInterceptSettings), nil
}
