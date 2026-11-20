package resolvers

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/replay"
)

// ReplaySendService sends Replay requests (F-REP-2).
type ReplaySendService interface {
	Send(context.Context, int64, replay.SendInput) (replay.Entry, error)
	Cancel(context.Context, int64) (bool, error)
	Sending(int64) bool
}

var _ ReplaySendService = (*replay.Sender)(nil)

func (r *mutationResolver) SendReplaySession(ctx context.Context, id string, input generated.SendReplaySessionInput) (*generated.ReplayEntry, error) {
	if r.deps.ReplaySend == nil {
		return nil, apperr.ErrNotImplemented
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return nil, err
	}
	in := replay.SendInput{Raw: input.Raw, Options: replayOptions(input.Options)}
	if c := replayConnection(input.Connection); c != nil {
		in.Connection = *c
	}
	e, err := r.deps.ReplaySend.Send(ctx, n, in)
	if err != nil {
		return nil, err
	}
	return toReplayEntry(e), nil
}

func (r *mutationResolver) CancelReplaySend(ctx context.Context, id string) (bool, error) {
	if r.deps.ReplaySend == nil {
		return false, apperr.ErrNotImplemented
	}
	n, err := parsePresetID("replay session", id)
	if err != nil {
		return false, err
	}
	return r.deps.ReplaySend.Cancel(ctx, n)
}
