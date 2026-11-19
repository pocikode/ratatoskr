package resolvers

// Contract stubs for replay.graphqls (T-098), except send/cancel (stubs_replay_send.go).
// T-033 replaces this file with replay.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// ReplayService serves the Replay collection and session operations. Placeholder until T-033 defines its methods.
type ReplayService any

func (r *Resolver) ReplayCollection() generated.ReplayCollectionResolver {
	return &replayCollectionResolver{r}
}

func (r *Resolver) ReplaySession() generated.ReplaySessionResolver { return &replaySessionResolver{r} }

func (r *Resolver) ReplayEntry() generated.ReplayEntryResolver { return &replayEntryResolver{r} }

type (
	replayCollectionResolver struct{ *Resolver }
	replaySessionResolver    struct{ *Resolver }
	replayEntryResolver      struct{ *Resolver }
)

func (r *queryResolver) ReplayCollections(context.Context) ([]generated.ReplayCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *queryResolver) ReplaySession(context.Context, string) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateReplayCollection(context.Context, string) (*generated.ReplayCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) RenameReplayCollection(
	context.Context, string, string,
) (*generated.ReplayCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteReplayCollection(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *mutationResolver) ReorderReplayCollections(context.Context, []string) ([]generated.ReplayCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CreateReplaySession(
	context.Context, generated.CreateReplaySessionInput,
) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) UpdateReplaySession(
	context.Context, string, generated.UpdateReplaySessionInput,
) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) SetReplaySessionColor(
	context.Context, string, *generated.RequestColor,
) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DuplicateReplaySession(context.Context, string) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) MoveReplaySession(
	context.Context, string, string, int,
) (*generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) DeleteReplaySession(context.Context, string) (string, error) {
	return "", apperr.ErrNotImplemented
}

func (r *replayCollectionResolver) Sessions(
	context.Context, *generated.ReplayCollection,
) ([]generated.ReplaySession, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *replaySessionResolver) Collection(
	context.Context, *generated.ReplaySession,
) (*generated.ReplayCollection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *replaySessionResolver) Entries(
	context.Context, *generated.ReplaySession, int, *string,
) (*generated.ReplayEntryConnection, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *replayEntryResolver) Request(context.Context, *generated.ReplayEntry) (*generated.HTTPRequest, error) {
	return nil, apperr.ErrNotImplemented
}
