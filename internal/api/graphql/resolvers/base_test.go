package resolvers

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestPingTicksUntilCancelled(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		start := time.Now()

		ch, err := New(Deps{}).Subscription().Ping(ctx)
		if err != nil {
			t.Fatalf("Ping: %v", err)
		}

		for want := int64(1); want <= 3; want++ {
			p := <-ch
			if p.Seq != want {
				t.Fatalf("seq = %d, want %d", p.Seq, want)
			}

			if elapsed := p.At.Sub(start); elapsed != time.Duration(want)*time.Second {
				t.Errorf("tick %d after %v, want %ds", want, elapsed, want)
			}
		}

		cancel()

		if _, open := <-ch; open {
			t.Error("channel still open after cancel")
		}
	})
}

func TestServerInfo(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	r := New(Deps{
		Build:        BuildInfo{Version: "v1.2.3", Commit: "abc", GoVersion: "go1.27.4"},
		StartedAt:    started,
		AuthRequired: true,
	})

	info, err := r.Query().ServerInfo(t.Context())
	if err != nil {
		t.Fatalf("ServerInfo: %v", err)
	}

	if info.Version != "v1.2.3" || info.Commit != "abc" || info.GoVersion != "go1.27.4" ||
		!info.StartedAt.Equal(started) || !info.AuthRequired {
		t.Errorf("ServerInfo = %+v", info)
	}
}
