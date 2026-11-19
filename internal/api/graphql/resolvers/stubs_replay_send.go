package resolvers

// Contract stubs for sendReplaySession and cancelReplaySend (T-098). T-106 replaces this file with replay_send.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

// ReplaySendService sends Replay requests. Placeholder until T-106 defines its methods.
type ReplaySendService any

func (r *mutationResolver) SendReplaySession(
	context.Context, string, generated.SendReplaySessionInput,
) (*generated.ReplayEntry, error) {
	return nil, apperr.ErrNotImplemented
}

func (r *mutationResolver) CancelReplaySend(context.Context, string) (bool, error) {
	return false, apperr.ErrNotImplemented
}
