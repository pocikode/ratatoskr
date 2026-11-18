package project

import (
	"context"
	"sync"
)

// Kind is what happened to a project.
type Kind string

// Change kinds, matching the GraphQL enum ProjectChangeKind.
const (
	Created  Kind = "CREATED"
	Updated  Kind = "UPDATED"
	Deleted  Kind = "DELETED"
	Selected Kind = "SELECTED"
)

// subBuffer is the capacity of each subscriber's channel.
const subBuffer = 64

// Change is one event about the set of projects.
type Change struct {
	Kind      Kind
	ProjectID int64
	// Project is the project after the change; nil for [Deleted].
	Project *Project
}

// Subscribe returns a channel of changes, in the order they happen, until ctx
// ends, the service closes, or the subscriber falls subBuffer events behind. A
// subscriber that falls behind is closed instead of blocking the service: it
// must resubscribe and re-read the state.
func (s *Service) Subscribe(ctx context.Context) <-chan Change {
	return s.hub.subscribe(ctx)
}

type hub struct {
	mu   sync.Mutex
	subs map[chan Change]struct{}
	done bool
}

func (h *hub) subscribe(ctx context.Context) <-chan Change {
	ch := make(chan Change, subBuffer)

	h.mu.Lock()
	if h.done {
		close(ch)
		h.mu.Unlock()

		return ch
	}

	if h.subs == nil {
		h.subs = map[chan Change]struct{}{}
	}

	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	context.AfterFunc(ctx, func() { h.remove(ch) })

	return ch
}

func (h *hub) remove(ch chan Change) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
}

func (h *hub) emit(c Change) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs {
		select {
		case ch <- c:
		default:
			delete(h.subs, ch)
			close(ch)
		}
	}
}

func (h *hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.done = true

	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}
