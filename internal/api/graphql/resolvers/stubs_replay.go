package resolvers

// Contract stub for ReplaySession.requestTarget (T-110). T-033 implements it in replay.go.

import (
	"context"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func (r *replaySessionResolver) RequestTarget(context.Context, *generated.ReplaySession) (string, error) {
	return "", apperr.ErrNotImplemented
}
