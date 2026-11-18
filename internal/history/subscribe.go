package history

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
)

// Update is a changed request and whether it matches the subscription's
// filter after the change.
type Update struct {
	Request
	Matches bool
}

// SubscribeAdded streams every newly stored request that matches query and
// scope. The channel closes when ctx ends or when the subscriber fell so far
// behind that events were dropped: the caller then resubscribes and refetches.
// A slow subscriber never blocks capture.
func (s *Service) SubscribeAdded(ctx context.Context, query, scopeID *string) (<-chan Request, error) {
	return subscribe(ctx, s, query, scopeID, func(ctx context.Context, ev Event, m func(Request) (bool, error), out chan<- Request) bool {
		if ev.Kind != Added {
			return true
		}

		r, ok, err := s.matching(ctx, ev.RequestID, m)
		if err != nil || !ok {
			return true
		}

		return send(ctx, out, r)
	})
}

// SubscribeUpdated streams every change to a stored request (response
// stored, colour or note set) with whether it matches afterwards. Closing
// rules are those of [Service.SubscribeAdded].
func (s *Service) SubscribeUpdated(ctx context.Context, query, scopeID *string) (<-chan Update, error) {
	return subscribe(ctx, s, query, scopeID, func(ctx context.Context, ev Event, m func(Request) (bool, error), out chan<- Update) bool {
		if ev.Kind != Updated {
			return true
		}

		db, err := s.db()
		if err != nil {
			return true
		}

		r, err := get(ctx, db.Reader(), ev.RequestID)
		if err != nil {
			return true // deleted meanwhile
		}

		ok, err := m(r)
		if err != nil {
			return true
		}

		return send(ctx, out, Update{Request: r, Matches: ok})
	})
}

// SubscribeDeleted streams the IDs of deleted requests in batches. Closing
// rules are those of [Service.SubscribeAdded].
func (s *Service) SubscribeDeleted(ctx context.Context) (<-chan []int64, error) {
	return subscribe(ctx, s, nil, nil, func(ctx context.Context, ev Event, _ func(Request) (bool, error), out chan<- []int64) bool {
		if ev.Kind != Deleted {
			return true
		}

		return send(ctx, out, ev.IDs)
	})
}

func send[T any](ctx context.Context, out chan<- T, v T) bool {
	select {
	case out <- v:
		return true
	case <-ctx.Done():
		return false
	}
}

// subscribe runs handle for each bus event until it returns false, ctx ends
// or the bus marks the subscriber stale. match tells whether a request
// satisfies the subscription's query and scope.
func subscribe[T any](ctx context.Context, s *Service, q, scopeID *string,
	handle func(context.Context, Event, func(Request) (bool, error), chan<- T) bool,
) (<-chan T, error) {
	if s.deps.Bus == nil {
		return nil, apperr.ErrNotImplemented
	}

	f, err := s.filter(ctx, q, scopeID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	src := s.deps.Bus.Subscribe(ctx)
	out := make(chan T, 16)

	match := func(r Request) (bool, error) { return s.eval(ctx, f, r) }

	go func() {
		defer close(out)
		defer cancel() // drops the bus subscription as well

		for m := range src {
			// Stale: events were dropped, the stream can no longer be trusted.
			if m.Stale || !handle(ctx, m.Value, match, out) {
				return
			}
		}
	}()

	return out, nil
}

// matching loads request id and returns it when it satisfies match.
func (s *Service) matching(ctx context.Context, id int64, match func(Request) (bool, error)) (Request, bool, error) {
	db, err := s.db()
	if err != nil {
		return Request{}, false, err
	}

	r, err := get(ctx, db.Reader(), id)
	if err != nil {
		return Request{}, false, err
	}

	ok, err := match(r)

	return r, ok, err
}

// eval reports whether r satisfies f using the in-memory evaluator, which
// needs the raw bytes and headers: they are only loaded for a non-empty filter.
func (s *Service) eval(ctx context.Context, f filter, r Request) (bool, error) {
	if f.node == nil {
		return true, nil
	}

	db, err := s.db()
	if err != nil {
		return false, err
	}

	m := &message{r: r}

	if m.req, err = loadSide(ctx, db.Reader(), "requests", "id", r.ID); err != nil {
		return false, err
	}

	if r.Resp != nil {
		side, err := loadSide(ctx, db.Reader(), "responses", "request_id", r.ID)
		if err != nil {
			return false, err
		}

		m.resp = &query.Response{
			Side: side, Code: r.Resp.Code, RoundTrip: time.Duration(r.Resp.RoundTripMs) * time.Millisecond,
		}
	}

	return httpql.Eval(f.node, m)
}

// loadSide reads raw bytes and header fields of one message. table and key
// are constants of this package.
func loadSide(ctx context.Context, db *sql.DB, table, key string, id int64) (query.Side, error) {
	var sd query.Side

	err := db.QueryRowContext(ctx, `SELECT raw, body_offset FROM `+table+` WHERE `+key+` = ?`, id).Scan(&sd.Raw, &sd.BodyOffset)
	if err != nil {
		return sd, fmt.Errorf("history: load %s %d: %w", table, id, err)
	}

	hdr := "request_headers"
	if table == "responses" {
		hdr = "response_headers"
	}

	rows, err := db.QueryContext(ctx, `SELECT name, value FROM `+hdr+` WHERE request_id = ? ORDER BY idx`, id)
	if err != nil {
		return sd, fmt.Errorf("history: load headers %d: %w", id, err)
	}
	defer rows.Close()

	for rows.Next() {
		var h query.Header
		if err := rows.Scan(&h.Name, &h.Value); err != nil {
			return sd, fmt.Errorf("history: load headers %d: %w", id, err)
		}

		sd.Headers = append(sd.Headers, h)
	}

	return sd, rows.Err() //nolint:wrapcheck // T-016: iteration error, no extra context to add
}

// message adapts a stored request to [query.Message].
type message struct {
	r    Request
	req  query.Side
	resp *query.Response
}

func (m *message) ID() int64                 { return m.r.ID }
func (m *message) Source() string            { return m.r.Source }
func (m *message) Host() string              { return m.r.Host }
func (m *message) Port() int                 { return m.r.Port }
func (m *message) TLS() bool                 { return m.r.TLS }
func (m *message) Method() string            { return m.r.Method }
func (m *message) Path() string              { return m.r.Path }
func (m *message) Query() string             { return m.r.Query }
func (m *message) Ext() string               { return m.r.Ext }
func (m *message) CreatedAt() time.Time      { return m.r.CreatedAt }
func (m *message) Color() string             { return m.r.Color }
func (m *message) Note() string              { return m.r.Note }
func (m *message) Edited() bool              { return m.r.EditedFrom != nil }
func (m *message) Request() query.Side       { return m.req }
func (m *message) Response() *query.Response { return m.resp }
