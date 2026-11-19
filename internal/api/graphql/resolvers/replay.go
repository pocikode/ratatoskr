package resolvers

import (
	"context"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/replay"
)

// ReplayService is the collection/session persistence API used by these resolvers.
type ReplayService interface {
	Collections(context.Context) ([]replay.Collection, error)
	Collection(context.Context, int64) (replay.Collection, error)
	CreateCollection(context.Context, string) (replay.Collection, error)
	RenameCollection(context.Context, int64, string) (replay.Collection, error)
	DeleteCollection(context.Context, int64) error
	ReorderCollections(context.Context, []int64) ([]replay.Collection, error)
	Session(context.Context, int64) (replay.Session, error)
	Sessions(context.Context, int64) ([]replay.Session, error)
	CreateSession(context.Context, replay.CreateInput) (replay.Session, error)
	UpdateSession(context.Context, int64, replay.UpdateInput) (replay.Session, error)
	SetSessionColor(context.Context, int64, string) (replay.Session, error)
	DuplicateSession(context.Context, int64) (replay.Session, error)
	MoveSession(context.Context, int64, int64, int) (replay.Session, error)
	DeleteSession(context.Context, int64) error
	Entries(context.Context, int64, int, *string) (replay.EntryPage, error)
}

var _ ReplayService = (*replay.Service)(nil)

func (r *Resolver) replay() (ReplayService, error) {
	if r.deps.Replay == nil {
		return nil, apperr.ErrNotImplemented
	}
	return r.deps.Replay, nil
}

// ReplayCollection returns the collection field resolver.
func (r *Resolver) ReplayCollection() generated.ReplayCollectionResolver {
	return &replayCollectionResolver{r}
}

// ReplaySession returns the session field resolver.
func (r *Resolver) ReplaySession() generated.ReplaySessionResolver { return &replaySessionResolver{r} }

// ReplayEntry returns the entry field resolver.
func (r *Resolver) ReplayEntry() generated.ReplayEntryResolver { return &replayEntryResolver{r} }

type replayCollectionResolver struct{ *Resolver }
type replaySessionResolver struct{ *Resolver }
type replayEntryResolver struct{ *Resolver }

func toReplayCollection(c replay.Collection) *generated.ReplayCollection {
	return &generated.ReplayCollection{ID: strconv.FormatInt(c.ID, 10), Name: c.Name, IsDefault: c.IsDefault}
}

func toReplayCollections(cs []replay.Collection) []generated.ReplayCollection {
	out := make([]generated.ReplayCollection, len(cs))
	for i, c := range cs {
		out[i] = *toReplayCollection(c)
	}
	return out
}

func toReplaySession(s replay.Session) *generated.ReplaySession {
	out := &generated.ReplaySession{
		ID: strconv.FormatInt(s.ID, 10), Kind: generated.ReplaySessionKindHTTP, Name: s.Name,
		Collection: &generated.ReplayCollection{ID: strconv.FormatInt(s.CollectionID, 10)},
		Connection: &generated.ReplayConnection{Host: s.Connection.Host, Port: s.Connection.Port, TLS: s.Connection.TLS, Sni: s.Connection.SNI},
		Options:    &generated.ReplayOptions{FollowRedirects: s.Options.FollowRedirects, UpdateContentLength: s.Options.UpdateContentLength, TimeoutMs: s.Options.TimeoutMs, ApplyMatchReplace: s.Options.ApplyMatchReplace},
		Draft:      s.Draft, CreatedAt: s.CreatedAt,
	}
	if s.Color != "" {
		out.Color = new(generated.RequestColor(s.Color))
	}
	return out
}

func toReplayEntry(e replay.Entry) *generated.ReplayEntry {
	return &generated.ReplayEntry{ID: strconv.FormatInt(e.ID, 10), Request: &generated.HTTPRequest{ID: strconv.FormatInt(e.RequestID, 10)}, Error: e.Error, CreatedAt: e.CreatedAt}
}

func replayConnection(c *generated.ReplayConnectionInput) *replay.Connection {
	if c == nil {
		return nil
	}
	return &replay.Connection{Host: c.Host, Port: c.Port, TLS: c.TLS, SNI: c.Sni}
}

func replayOptions(o *generated.ReplayOptionsInput) *replay.OptionsInput {
	if o == nil {
		return nil
	}
	return &replay.OptionsInput{FollowRedirects: o.FollowRedirects, UpdateContentLength: o.UpdateContentLength, TimeoutMs: o.TimeoutMs, ApplyMatchReplace: o.ApplyMatchReplace}
}

func (r *queryResolver) ReplayCollections(ctx context.Context) ([]generated.ReplayCollection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	cs, err := svc.Collections(ctx)
	return toReplayCollections(cs), err
}

func (r *queryResolver) ReplaySession(ctx context.Context, id string) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, _ := parsePresetID("replay session", id)
	s, err := svc.Session(ctx, n)
	if apperr.CodeOf(err) == apperr.NotFound {
		return nil, nil //nolint:nilnil // T-033: unknown session reads as null per schema.
	}
	if err != nil {
		return nil, err
	}
	return toReplaySession(s), nil
}

func (r *mutationResolver) CreateReplayCollection(ctx context.Context, name string) (*generated.ReplayCollection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	c, err := svc.CreateCollection(ctx, name)
	if err != nil {
		return nil, err
	}
	return toReplayCollection(c), nil
}

func (r *mutationResolver) ReorderReplayCollections(ctx context.Context, ids []string) ([]generated.ReplayCollection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	ns := make([]int64, len(ids))
	for i, id := range ids {
		ns[i], err = parsePresetID("replay collection", id)
		if err != nil {
			return nil, apperr.New(apperr.BadInput, "ids must list every collection exactly once")
		}
	}
	cs, err := svc.ReorderCollections(ctx, ns)
	return toReplayCollections(cs), err
}

func (r *mutationResolver) CreateReplaySession(ctx context.Context, input generated.CreateReplaySessionInput) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	in := replay.CreateInput{Connection: replayConnection(input.Connection), Raw: input.Raw, Name: input.Name}
	if input.CollectionID != nil {
		n, err := parsePresetID("replay collection", *input.CollectionID)
		if err != nil {
			return nil, err
		}
		in.CollectionID = &n
	}
	if input.FromRequestID != nil {
		n, err := parseRowID(*input.FromRequestID)
		if err != nil {
			return nil, err
		}
		in.FromRequestID = &n
	}
	s, err := svc.CreateSession(ctx, in)
	if err != nil {
		return nil, err
	}
	return toReplaySession(s), nil
}

func (r *mutationResolver) RenameReplayCollection(ctx context.Context, id string, name string) (*generated.ReplayCollection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, err := parsePresetID("replay collection", id)
	if err != nil {
		return nil, err
	}
	out, err := svc.RenameCollection(ctx, n, name)
	if err != nil {
		return nil, err
	}
	return toReplayCollection(out), nil
}

func (r *mutationResolver) UpdateReplaySession(ctx context.Context, id string, input generated.UpdateReplaySessionInput) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return nil, err
	}
	out, err := svc.UpdateSession(ctx, n, replay.UpdateInput{Name: input.Name, Connection: replayConnection(input.Connection), Options: replayOptions(input.Options), Draft: input.Draft})
	if err != nil {
		return nil, err
	}
	return toReplaySession(out), nil
}

func (r *mutationResolver) SetReplaySessionColor(ctx context.Context, id string, color *generated.RequestColor) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return nil, err
	}
	c := ""
	if color != nil {
		c = string(*color)
	}
	out, err := svc.SetSessionColor(ctx, n, c)
	if err != nil {
		return nil, err
	}
	return toReplaySession(out), nil
}

func (r *mutationResolver) DuplicateReplaySession(ctx context.Context, id string) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return nil, err
	}
	out, err := svc.DuplicateSession(ctx, n)
	if err != nil {
		return nil, err
	}
	return toReplaySession(out), nil
}

func (r *mutationResolver) MoveReplaySession(ctx context.Context, id string, collectionID string, index int) (*generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return nil, err
	}
	target, err := parsePresetID("replay collection", collectionID)
	if err != nil {
		return nil, err
	}
	out, err := svc.MoveSession(ctx, n, target, index)
	if err != nil {
		return nil, err
	}
	return toReplaySession(out), nil
}

func (r *mutationResolver) DeleteReplayCollection(ctx context.Context, id string) (string, error) {
	svc, err := r.replay()
	if err != nil {
		return "", err
	}
	n, err := parsePresetID("replay collection", id)
	if err != nil {
		return "", err
	}
	if err := svc.DeleteCollection(ctx, n); err != nil {
		return "", err
	}
	return id, nil
}

func (r *mutationResolver) DeleteReplaySession(ctx context.Context, id string) (string, error) {
	svc, err := r.replay()
	if err != nil {
		return "", err
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return "", err
	}
	if err := svc.DeleteSession(ctx, n); err != nil {
		return "", err
	}
	return id, nil
}

func (r *replayCollectionResolver) Sessions(ctx context.Context, obj *generated.ReplayCollection) ([]generated.ReplaySession, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	id, err := parsePresetID("replay collection", obj.ID)
	if err != nil {
		return nil, err
	}
	ss, err := svc.Sessions(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]generated.ReplaySession, len(ss))
	for i, s := range ss {
		out[i] = *toReplaySession(s)
	}
	return out, nil
}

func (r *replaySessionResolver) Collection(ctx context.Context, obj *generated.ReplaySession) (*generated.ReplayCollection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	id, err := parsePresetID("replay collection", obj.Collection.ID)
	if err != nil {
		return nil, err
	}
	c, err := svc.Collection(ctx, id)
	if err != nil {
		return nil, err
	}
	return toReplayCollection(c), nil
}

func (r *replaySessionResolver) Entries(ctx context.Context, obj *generated.ReplaySession, first int, after *string) (*generated.ReplayEntryConnection, error) {
	svc, err := r.replay()
	if err != nil {
		return nil, err
	}
	id, err := parsePresetID("replay session", obj.ID)
	if err != nil {
		return nil, err
	}
	p, err := svc.Entries(ctx, id, first, after)
	if err != nil {
		return nil, err
	}
	out := &generated.ReplayEntryConnection{Edges: make([]generated.ReplayEntryEdge, len(p.Items)), TotalCount: p.Total, PageInfo: &generated.PageInfo{HasNextPage: p.HasNext, HasPreviousPage: p.HasPrevious}}
	for i, entry := range p.Items {
		out.Edges[i] = generated.ReplayEntryEdge{Cursor: entry.Cursor, Node: toReplayEntry(entry)}
	}
	if len(p.Items) > 0 {
		out.PageInfo.StartCursor = new(p.Items[0].Cursor)
		out.PageInfo.EndCursor = new(p.Items[len(p.Items)-1].Cursor)
	}
	return out, nil
}

func (r *replayEntryResolver) Request(ctx context.Context, obj *generated.ReplayEntry) (*generated.HTTPRequest, error) {
	svc, err := r.history()
	if err != nil {
		return nil, err
	}
	id, err := parseRowID(obj.Request.ID)
	if err != nil {
		return nil, err
	}
	req, err := svc.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toHTTPRequest(req), nil
}
