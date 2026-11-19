// Package intercept queues fully buffered proxy messages for manual review.
package intercept

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/scope"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Kind is the side held in the queue. The zero value selects both sides in queue operations.
type Kind string

// Request and Response are the two sides that can be held.
const (
	Request  Kind = "REQUEST"
	Response Kind = "RESPONSE"
)

const (
	maxQueueItems = 1_000
	maxQueueBytes = 256 << 20
)

// Item is a held message. Raw is read-only and includes the entire message.
type Item struct {
	ID          string
	Kind        Kind
	RequestID   int64
	Raw         []byte
	HeldAt      time.Time
	DB          *store.DB
	projectDone <-chan struct{}
}

// ProjectDone closes when the service leaves the item's project.
func (i Item) ProjectDone() <-chan struct{} { return i.projectDone }

// Decision is the outcome of a hold. For a drop, DropStatusCode is set and Raw is nil.
type Decision struct {
	Raw            []byte
	Edited         bool
	DropStatusCode int
}

// QueueChange describes an enqueue or removal.
type QueueChange struct {
	Item  Item
	Added bool
}

type settingsChange struct {
	db       *store.DB
	settings Settings
}

type held struct {
	Item
	method string
	result chan Decision
}

type holdRules struct {
	db                          *store.DB
	settings                    Settings
	requestQuery, responseQuery core.Node
}

func (r *holdRules) enabled(kind Kind) bool {
	return r.settings.Queuing &&
		(kind == Request && r.settings.Requests || kind == Response && r.settings.Responses)
}

// Service owns the current project's settings and the FIFO queue.
type Service struct {
	selected func() *store.DB
	rules    atomic.Pointer[holdRules]

	mu          sync.Mutex
	projectDone chan struct{}
	queue       []*held
	queueBytes  int
	sequence    uint64
	events      events.Bus[QueueChange]
	changes     events.Bus[settingsChange]
}

// New loads settings from the current project. selected must return the live selection.
func New(ctx context.Context, selected func() *store.DB) (*Service, error) {
	s := &Service{selected: selected}
	s.rules.Store(&holdRules{settings: defaults()})
	if err := s.ProjectChanged(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

// IsCurrent reports whether db is still the selected project.
func (s *Service) IsCurrent(db *store.DB) bool { return db == s.selected() }

// ProjectChanged releases the old queue and loads the selected project's settings.
func (s *Service) ProjectChanged(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.selectProject(ctx)
}

func (s *Service) selectProject(ctx context.Context) error {
	db := s.selected()
	old := s.rules.Load()
	if db == old.db {
		return nil
	}

	s.releaseMatching(old.db, func(*held) bool { return true }, false, false)
	if s.projectDone != nil {
		close(s.projectDone)
		s.projectDone = nil
	}
	settings, err := load(ctx, db)
	if err != nil {
		return err
	}
	node, err := compile(ctx, db, settings.Query)
	if err != nil {
		if apperr.CodeOf(err) != apperr.BadQuery {
			return err
		}
		// A referenced preset may have been deleted. Keep the text editable.
		node = &core.Not{}
	}

	s.projectDone = make(chan struct{})
	s.rules.Store(&holdRules{db: db, settings: settings, requestQuery: requestCondition(node), responseQuery: node})
	s.changes.Publish(settingsChange{db, settings})

	return nil
}

// WouldHold reports whether buffering this side may lead to a hold.
func (s *Service) WouldHold(kind Kind) bool {
	rules := s.rules.Load()

	return rules.db != nil && s.IsCurrent(rules.db) && rules.enabled(kind)
}

// HoldRequest holds a matching, fully buffered request. The caller must wait
// for its history record in db to be readable and keep the supplied Raw immutable.
func (s *Service) HoldRequest(ctx context.Context, db *store.DB, msg query.Message) (Decision, error) {
	return s.hold(ctx, db, Request, msg, msg.Request().Raw)
}

// HoldResponse is HoldRequest for an exchange with a fully buffered response.
func (s *Service) HoldResponse(ctx context.Context, db *store.DB, msg query.Message) (Decision, error) {
	return s.hold(ctx, db, Response, msg, msg.Response().Raw)
}

func (s *Service) hold(ctx context.Context, db *store.DB, kind Kind, msg query.Message, raw []byte) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}

	h := s.enqueue(ctx, db, kind, msg, raw)
	if h == nil {
		if err := ctx.Err(); err != nil {
			return Decision{}, err
		}

		return Decision{Raw: raw}, nil
	}

	select {
	case result := <-h.result:
		return result, nil
	case <-ctx.Done():
		s.mu.Lock()
		defer s.mu.Unlock()

		if i := slices.Index(s.queue, h); i >= 0 {
			s.queue = slices.Delete(s.queue, i, i+1)
			s.queueBytes -= len(h.Raw)
			s.publishRemoval(h)
		}

		return Decision{}, ctx.Err()
	}
}

func (s *Service) enqueue(ctx context.Context, db *store.DB, kind Kind, msg query.Message, raw []byte) *held {
	for {
		rules := s.rules.Load()
		if db == nil || db != rules.db || !s.IsCurrent(db) || !rules.enabled(kind) || !rules.matches(ctx, kind, msg) {
			return nil
		}

		s.mu.Lock()
		if !s.IsCurrent(db) || ctx.Err() != nil {
			s.mu.Unlock()
			return nil
		}
		if rules != s.rules.Load() {
			s.mu.Unlock()
			continue
		}
		if len(s.queue) >= maxQueueItems || len(raw) > maxQueueBytes-s.queueBytes {
			s.mu.Unlock()
			slog.InfoContext(ctx, "intercept queue limit reached; forwarding unchanged", "kind", kind, "request_id", msg.ID())
			return nil
		}
		item := s.add(db, kind, msg, raw)
		s.mu.Unlock()

		return item
	}
}

func (r *holdRules) matches(ctx context.Context, kind Kind, msg query.Message) bool {
	node := r.requestQuery
	if kind == Response {
		node = r.responseQuery
	}
	matched, err := httpql.Eval(node, msg)
	if err != nil {
		slog.WarnContext(ctx, "intercept query evaluation failed", "error", err)
		return false
	}
	if !matched {
		return false
	}
	if r.settings.ScopeID == 0 {
		return true
	}

	preset, err := scope.New(func() *store.DB { return r.db }).Get(ctx, r.settings.ScopeID)
	if err != nil {
		slog.WarnContext(ctx, "intercept scope lookup failed", "scope_id", r.settings.ScopeID, "error", err)
		return false
	}
	matcher, err := preset.Matcher()
	if err != nil {
		slog.WarnContext(ctx, "intercept scope matcher failed", "scope_id", r.settings.ScopeID, "error", err)
		return false
	}

	return matcher.InScope(msg.Host(), msg.Port(), msg.Path())
}

func (s *Service) add(db *store.DB, kind Kind, msg query.Message, raw []byte) *held {
	id := msg.ID()
	s.sequence++
	h := &held{
		ID:   string(kind) + ":" + strconv.FormatInt(id, 10) + ":" + strconv.FormatUint(s.sequence, 10),
		Kind: kind, RequestID: id, Raw: raw, HeldAt: time.Now(), DB: db,
		projectDone: s.projectDone,
		result:      make(chan Decision, 1),
	}
	if kind == Response {
		h.method = msg.Method()
	}
	s.queue = append(s.queue, h)
	s.queueBytes += len(raw)
	s.events.Publish(QueueChange{Item: h.Item, Added: true})

	return h
}

// Queue returns the selected project's held items in FIFO order.
func (s *Service) Queue(kind Kind) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.selected()
	var items []Item
	for _, h := range s.queue {
		if h.DB == db && (kind == "" || h.Kind == kind) {
			items = append(items, h.Item)
		}
	}

	return items
}

// Forward releases id with the original bytes or a validated replacement.
func (s *Service) Forward(id string, raw []byte, updateContentLength bool) error {
	return s.releaseOne(id, raw, updateContentLength, false)
}

// Drop releases id with the configured drop status code.
func (s *Service) Drop(id string) error { return s.releaseOne(id, nil, false, true) }

func (s *Service) releaseOne(id string, raw []byte, updateLength, drop bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.selected()
	for i, h := range s.queue {
		if h.DB != db || h.ID != id {
			continue
		}

		result := s.decision(h, drop)
		if raw != nil {
			if len(raw) > MaxEditBytes || len(h.Raw) > MaxEditBytes {
				return apperr.New(apperr.BadInput, "intercept edits are limited to %d bytes", MaxEditBytes)
			}
			var err error
			if raw, err = replacement(h.Kind, h.method, raw, updateLength); err != nil {
				return err
			}
			result = Decision{Raw: raw, Edited: true}
		}
		if !s.IsCurrent(db) {
			break
		}

		s.queue = slices.Delete(s.queue, i, i+1)
		s.queueBytes -= len(h.Raw)
		s.publishRemoval(h)
		h.result <- result

		return nil
	}

	return apperr.New(apperr.NotFound, "intercept item %q not found", id)
}

// ReleaseAll forwards or drops all selected items, in FIFO order.
func (s *Service) ReleaseAll(kind Kind, drop bool) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.releaseMatching(s.selected(), func(h *held) bool { return kind == "" || h.Kind == kind }, drop, true)
}

func (s *Service) releaseMatching(db *store.DB, match func(*held) bool, drop, notify bool) int {
	kept := s.queue[:0]
	for _, h := range s.queue {
		if h.DB != db || !match(h) {
			kept = append(kept, h)
			continue
		}
		if notify {
			s.publishRemoval(h)
		}
		s.queueBytes -= len(h.Raw)
		h.result <- s.decision(h, drop)
	}
	count := len(s.queue) - len(kept)
	clear(s.queue[len(kept):])
	s.queue = kept

	return count
}

func (s *Service) decision(h *held, drop bool) Decision {
	if drop {
		return Decision{DropStatusCode: s.rules.Load().settings.DropStatusCode}
	}

	return Decision{Raw: h.Raw}
}

func (s *Service) publishRemoval(h *held) {
	if s.IsCurrent(h.DB) {
		s.events.Publish(QueueChange{Item: h.Item})
	}
}

// SubscribeQueue delivers ADDED and REMOVED events from the selected project.
func (s *Service) SubscribeQueue(ctx context.Context) <-chan QueueChange {
	return subscribe(ctx, &s.events, func(e QueueChange) (QueueChange, <-chan struct{}, bool) {
		return e, e.Item.ProjectDone(), s.IsCurrent(e.Item.DB)
	})
}

// SubscribeSettings delivers changes to the selected project's settings.
func (s *Service) SubscribeSettings(ctx context.Context) <-chan Settings {
	return subscribe(ctx, &s.changes, func(e settingsChange) (Settings, <-chan struct{}, bool) {
		return e.settings, nil, s.IsCurrent(e.db)
	})
}

func subscribe[T, U any](ctx context.Context, bus *events.Bus[T], filter func(T) (U, <-chan struct{}, bool)) <-chan U {
	ctx, cancel := context.WithCancel(ctx)
	in := bus.Subscribe(ctx)
	out := make(chan U)
	go func() {
		defer cancel()
		defer close(out)

		for event := range in {
			if event.Stale {
				return
			}
			value, projectDone, ok := filter(event.Value)
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-projectDone:
			case out <- value:
			}
		}
	}()

	return out
}
