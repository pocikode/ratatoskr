// Package events is the in-process fan-out the proxy and the services publish
// to (docs/03-architecture.md §7). Publishing never blocks: a subscriber that
// cannot keep up loses events and is told to resync.
package events

import (
	"context"
	"sync"
)

// SubBuffer is the capacity of each subscriber's channel.
const SubBuffer = 1024

// Msg is one delivery. Stale means events were dropped before this point (the
// subscriber was SubBuffer behind): re-read the state instead of trusting the
// stream. Value is the zero value on a stale marker.
type Msg[T any] struct {
	Value T
	Stale bool
}

type sub[T any] struct {
	ch    chan Msg[T]
	stale bool // guarded by Bus.mu
}

// Bus fans values of type T out to its subscribers. The zero value is ready.
type Bus[T any] struct {
	mu   sync.Mutex
	subs map[*sub[T]]struct{}
}

// Subscribe returns a channel of events published from now on. It is closed
// when ctx ends.
func (b *Bus[T]) Subscribe(ctx context.Context) <-chan Msg[T] {
	s := &sub[T]{ch: make(chan Msg[T], SubBuffer)}

	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[*sub[T]]struct{}{}
	}

	b.subs[s] = struct{}{}
	b.mu.Unlock()

	// AfterFunc runs in its own goroutine and only ends once ctx does.
	context.AfterFunc(ctx, func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if _, ok := b.subs[s]; ok {
			delete(b.subs, s)
			close(s.ch)
		}
	})

	return s.ch
}

// Publish delivers v to every subscriber without blocking.
func (b *Bus[T]) Publish(v T) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for s := range b.subs {
		if s.stale {
			select {
			case s.ch <- Msg[T]{Stale: true}:
				s.stale = false
			default:
				continue
			}
		}

		select {
		case s.ch <- Msg[T]{Value: v}:
		default:
			s.stale = true
		}
	}
}
