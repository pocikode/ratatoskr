// Package history persists what the proxy sees into the project database and
// announces it (docs/03-architecture.md §4, §5).
package history

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Batching budget (docs/03-architecture.md §5): flush every 50 ms or 200 rows.
const (
	FlushInterval = 50 * time.Millisecond
	BatchSize     = 200
	// flushGrace bounds the final flush after the context ends.
	flushGrace = 5 * time.Second
)

// Field is one header field, as sent.
type Field struct{ Name, Value string }

// Side is the stored form of one message.
type Side struct {
	// Raw is the message as seen on the wire; the body may be cut short.
	Raw        []byte
	BodyOffset int
	Headers    []Field
}

// Response is the answer to a captured request.
type Response struct {
	Side
	Code      int
	RoundTrip time.Duration
	CreatedAt time.Time
}

// Entry is one completed exchange. Resp is nil when the request was never
// answered at all.
type Entry struct {
	Source    string
	Host      string
	Port      int
	TLS       bool
	Method    string
	Target    string // request target; path and query are derived from it
	CreatedAt time.Time
	Truncated bool
	Req       Side
	Resp      *Response
}

// Kind is what happened to a request.
type Kind uint8

// Event kinds. A stored exchange publishes Added, then Updated when it already
// carries a response.
const (
	Added Kind = iota + 1
	Updated
)

// Event announces a stored request.
type Event struct {
	Kind      Kind
	RequestID int64
}

// Deps are the inputs to [New].
type Deps struct {
	// DB returns the open project database, or nil when no project is open;
	// capture is then paused and entries are dropped (requests still flow).
	DB     func() *store.DB
	Bus    *events.Bus[Event]
	Logger *slog.Logger
}

// Capture batches entries into the project database.
type Capture struct {
	deps    Deps
	in      chan Entry
	paused  atomic.Bool
	dropped atomic.Int64
}

// New creates a Capture; start its writer with [Capture.Run].
func New(deps Deps) *Capture {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	return &Capture{deps: deps, in: make(chan Entry, BatchSize)}
}

// SetPaused pauses or resumes capture; paused entries are dropped.
func (c *Capture) SetPaused(p bool) { c.paused.Store(p) }

// Paused reports whether entries are currently dropped: paused explicitly or
// no project is open.
func (c *Capture) Paused() bool {
	return c.paused.Load() || c.deps.DB == nil || c.deps.DB() == nil
}

// Dropped counts the entries discarded while paused or after a failed write.
func (c *Capture) Dropped() int64 { return c.dropped.Load() }

// Add queues e. It blocks while the queue is full (bounded memory beats lost
// history) and gives up when ctx ends.
func (c *Capture) Add(ctx context.Context, e Entry) {
	if c.Paused() {
		c.dropped.Add(1)

		return
	}

	select {
	case c.in <- e:
	case <-ctx.Done():
		c.dropped.Add(1)
	}
}

// Run writes batches until ctx ends, then flushes what is queued.
func (c *Capture) Run(ctx context.Context) {
	tick := time.NewTicker(FlushInterval)
	defer tick.Stop()

	batch := make([]Entry, 0, BatchSize)

	flush := func(ctx context.Context) {
		if len(batch) > 0 {
			c.flush(ctx, batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case e := <-c.in:
			if batch = append(batch, e); len(batch) >= BatchSize {
				flush(ctx)
			}
		case <-tick.C:
			flush(ctx)
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushGrace)
			defer cancel()

			for {
				select {
				case e := <-c.in:
					if batch = append(batch, e); len(batch) >= BatchSize {
						flush(fctx)
					}
				default:
					flush(fctx)

					return
				}
			}
		}
	}
}

func (c *Capture) flush(ctx context.Context, batch []Entry) {
	var db *store.DB
	if c.deps.DB != nil {
		db = c.deps.DB()
	}

	if db == nil {
		c.dropped.Add(int64(len(batch)))

		return
	}

	ids, err := c.write(ctx, db, batch)
	if err != nil {
		c.dropped.Add(int64(len(batch)))
		c.deps.Logger.Error("capture: write batch", "component", "history", "rows", len(batch), "error", err)

		return
	}

	if c.deps.Bus == nil {
		return
	}

	for i, id := range ids {
		c.deps.Bus.Publish(Event{Kind: Added, RequestID: id})

		if batch[i].Resp != nil {
			c.deps.Bus.Publish(Event{Kind: Updated, RequestID: id})
		}
	}
}

func (c *Capture) write(ctx context.Context, db *store.DB, batch []Entry) ([]int64, error) {
	// Requests, responses and headers share one transaction: a failure leaves
	// no request without its answer.
	ids := make([]int64, 0, len(batch))

	err := db.Write(ctx, func(tx *sql.Tx) error {
		for i := range batch {
			e := &batch[i]

			id, err := store.InsertRequestTx(ctx, tx, requestRow(e))
			if err != nil {
				return err
			}

			ids = append(ids, id)

			if err := insertHeaders(ctx, tx, insertRequestHeaderSQL, ids[i], e.Req.Headers); err != nil {
				return err
			}

			if e.Resp == nil {
				continue
			}

			if _, err := tx.ExecContext(ctx, `INSERT INTO responses
				(request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ids[i], e.Resp.Code, e.Resp.Raw, e.Resp.BodyOffset, len(e.Resp.Raw),
				e.Resp.RoundTrip.Milliseconds(), e.Resp.CreatedAt.UnixMilli()); err != nil {
				return fmt.Errorf("history: insert response: %w", err)
			}

			if err := insertHeaders(ctx, tx, insertResponseHeaderSQL, ids[i], e.Resp.Headers); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

const (
	insertRequestHeaderSQL  = "INSERT INTO request_headers (request_id, idx, name, name_lc, value) VALUES (?, ?, ?, ?, ?)"
	insertResponseHeaderSQL = "INSERT INTO response_headers (request_id, idx, name, name_lc, value) VALUES (?, ?, ?, ?, ?)"
)

// insertHeaders writes the header rows of one message with query, one of the
// two constants above.
func insertHeaders(ctx context.Context, tx *sql.Tx, query string, id int64, fields []Field) error {
	for i, f := range fields {
		if _, err := tx.ExecContext(ctx, query, id, i, f.Name, strings.ToLower(f.Name), f.Value); err != nil {
			return fmt.Errorf("history: insert header: %w", err)
		}
	}

	return nil
}

func requestRow(e *Entry) store.Request {
	target, _, _ := strings.Cut(e.Target, "#")
	p, q, _ := strings.Cut(target, "?")

	// An absolute-form target carries scheme and authority before the path.
	if _, rest, ok := strings.Cut(p, "://"); ok {
		_, p2, _ := strings.Cut(rest, "/")
		p = "/" + p2
	}

	return store.Request{
		Source:     orDefault(e.Source, "proxy"),
		Host:       e.Host,
		Port:       e.Port,
		TLS:        e.TLS,
		Method:     e.Method,
		Path:       p,
		Query:      q,
		Ext:        path.Ext(p), // includes the leading dot (docs/04 req.ext)
		Raw:        e.Req.Raw,
		BodyOffset: e.Req.BodyOffset,
		CreatedAt:  e.CreatedAt.UnixMilli(),
		Truncated:  e.Truncated,
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}

	return s
}
