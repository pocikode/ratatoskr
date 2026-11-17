package resolvers

import (
	"context"
	"time"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
)

func (r *queryResolver) ServerInfo(context.Context) (*generated.ServerInfo, error) {
	return &generated.ServerInfo{
		Version:      r.deps.Build.Version,
		Commit:       r.deps.Build.Commit,
		GoVersion:    r.deps.Build.GoVersion,
		StartedAt:    r.deps.StartedAt,
		AuthRequired: r.deps.AuthRequired,
	}, nil
}

// Ping emits a tick every PingInterval until ctx ends. The channel is unbuffered,
// so a slow client delays ticks instead of piling them up.
func (r *subscriptionResolver) Ping(ctx context.Context) (<-chan *generated.Ping, error) {
	ch := make(chan *generated.Ping)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(r.deps.PingInterval)
		defer ticker.Stop()

		for seq := int64(1); ; seq++ {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				select {
				case ch <- &generated.Ping{Seq: seq, At: now}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}
