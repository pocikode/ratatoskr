package events_test

import (
	"context"
	"testing"

	"github.com/pocikode/ratatoskr/internal/events"
)

func TestBusDeliversInOrder(t *testing.T) {
	t.Parallel()

	var b events.Bus[int]

	ch := b.Subscribe(t.Context())
	b.Publish(1)
	b.Publish(2)

	for _, want := range []int{1, 2} {
		if got := <-ch; got.Value != want || got.Stale {
			t.Fatalf("got %+v, want %d", got, want)
		}
	}
}

func TestBusSlowSubscriberGetsStale(t *testing.T) {
	t.Parallel()

	var b events.Bus[int]

	ch := b.Subscribe(t.Context())
	for i := range events.SubBuffer + 5 { // never blocks
		b.Publish(i)
	}

	for range events.SubBuffer { // consume the buffered ones
		<-ch
	}

	b.Publish(-1) // room again: stale marker first, then the value

	if got := <-ch; !got.Stale {
		t.Fatalf("got %+v, want stale marker", got)
	}

	if got := <-ch; got.Value != -1 {
		t.Fatalf("got %+v, want -1", got)
	}
}

func TestBusClosesOnCancel(t *testing.T) {
	t.Parallel()

	var b events.Bus[int]

	ctx, cancel := context.WithCancel(t.Context())
	ch := b.Subscribe(ctx)

	cancel()

	if _, ok := <-ch; ok {
		t.Fatal("channel still open after cancel")
	}

	b.Publish(1) // no panic on a removed subscriber
}
