// Package history persists what the proxy sees into the project database and
// announces it (docs/03-architecture.md §4, §5).
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/project"
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
	Truncated bool
}

// Entry is a captured request and its optional response.
type Entry struct {
	Source     string
	Host       string
	Port       int
	TLS        bool
	Method     string
	Target     string // request target; path and query are derived from it
	CreatedAt  time.Time
	Truncated  bool
	Dropped    bool
	EditedFrom *int64
	Req        Side
	Resp       *Response
}

// Kind is what happened to a request.
type Kind uint8

// Event kinds. A stored exchange publishes Added, then Updated when it already
// carries a response. Updated also announces a new colour or note; Deleted
// announces removed requests.
const (
	Added Kind = iota + 1
	Updated
	Deleted
)

// Event announces a stored, changed or deleted request.
type Event struct {
	Kind      Kind
	RequestID int64
	// IDs are the removed requests of a Deleted event (at most MaxPage).
	IDs []int64
}

// Deps are the inputs to [New].
type Deps struct {
	// DB returns the open project database, or nil when no project is open;
	// capture is then paused and entries are dropped (requests still flow).
	DB        func() *store.DB
	Subscribe func(context.Context) <-chan project.Change
	Bus       *events.Bus[Event]
	Logger    *slog.Logger
}

// Capture batches entries into the project database.
type Capture struct {
	deps    Deps
	in      chan captureOp
	paused  atomic.Bool
	dropped atomic.Int64

	mu      sync.Mutex
	db      *store.DB
	lastID  int64
	pending map[int64]*store.DB
}

type captureOp struct {
	db       *store.DB
	id       int64
	entry    Entry
	complete bool
	written  chan struct{}
}

// New creates a Capture; start its writer with [Capture.Run].
func New(deps Deps) *Capture {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}

	return &Capture{deps: deps, in: make(chan captureOp, BatchSize), pending: make(map[int64]*store.DB)}
}

// SetPaused pauses or resumes capture; paused entries are dropped.
func (c *Capture) SetPaused(p bool) { c.paused.Store(p) }

// Paused reports whether entries are currently dropped: paused explicitly or
// no project is open.
func (c *Capture) Paused() bool {
	return c.paused.Load() || c.deps.DB == nil || c.deps.DB() == nil
}

// Dropped counts capture operations that could not be stored.
func (c *Capture) Dropped() int64 { return c.dropped.Load() }

// Add queues e. It blocks while the queue is full (bounded memory beats lost
// history) and gives up when ctx ends.
func (c *Capture) Add(ctx context.Context, e Entry) {
	c.add(ctx, e, false)
}

// NewPending returns the ID as soon as the request enters the capture queue.
// It returns zero when the request could not be queued.
func (c *Capture) NewPending(ctx context.Context, e Entry) int64 {
	e.Resp = nil

	return c.add(ctx, e, true)
}

func (c *Capture) add(ctx context.Context, e Entry, pending bool) int64 {
	if c.paused.Load() || c.deps.DB == nil {
		c.dropped.Add(1)

		return 0
	}

	c.mu.Lock()
	db, err := c.refreshProject(ctx)
	if err != nil || db == nil {
		c.mu.Unlock()
		c.dropped.Add(1)
		if err != nil {
			c.deps.Logger.Error("capture: allocate request ID", "error", err)
		}

		return 0
	}

	// An outstanding ID from a previous project still names that exchange.
	c.lastID++
	for c.pending[c.lastID] != nil {
		c.lastID++
	}
	id := c.lastID
	if pending {
		c.pending[id] = db
	}
	c.mu.Unlock()

	if !c.enqueue(ctx, captureOp{db: db, id: id, entry: e}) {
		if pending {
			c.takePending(id)
		}

		return 0
	}

	return id
}

// refreshProject must be called with mu held. The DB pointer distinguishes
// different opens of the same project.
func (c *Capture) refreshProject(ctx context.Context) (*store.DB, error) {
	if c.deps.DB == nil {
		return nil, nil
	}

	db := c.deps.DB()
	if db == c.db {
		return db, nil
	}

	var lastID int64
	if db != nil {
		if err := db.Reader().QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM requests").Scan(&lastID); err != nil {
			return nil, fmt.Errorf("history: read maximum request ID: %w", err)
		}
	}

	c.db, c.lastID = db, lastID

	return db, nil
}

func (c *Capture) takePending(id int64) *store.DB {
	c.mu.Lock()
	defer c.mu.Unlock()

	db := c.pending[id]
	delete(c.pending, id)

	return db
}

// Written waits until the pending request row is readable in its project.
func (c *Capture) Written(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	owner := c.pending[id]
	c.mu.Unlock()

	var db *store.DB
	if c.deps.DB != nil {
		db = c.deps.DB()
	}
	if db == nil || owner != nil && owner != db {
		return errCaptureProjectChanged
	}

	found, err := c.requestWritten(ctx, db, id)
	if err != nil || found {
		return err
	}

	// The barrier follows the insert through the queue even when they share
	// a transaction. Only callers that need to wait allocate one.
	ready := make(chan struct{})
	select {
	case c.in <- captureOp{db: db, written: ready}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	}

	found, err = c.requestWritten(ctx, db, id)
	if err != nil || found {
		return err
	}

	return fmt.Errorf("history: pending request %d was not written", id)
}

var errCaptureProjectChanged = errors.New("history: capture project closed or changed")

func (c *Capture) requestWritten(ctx context.Context, db *store.DB, id int64) (bool, error) {
	if c.deps.DB() != db {
		return false, errCaptureProjectChanged
	}

	var found bool
	err := db.Reader().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM requests WHERE id = ?)", id).Scan(&found)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return false, fmt.Errorf("history: check pending request %d: %w", id, err)
	}

	return found, nil
}

// Complete queues the response to a pending request. A project switch
// invalidates outstanding completions from the previous database. A nil
// response releases the pending ID without storing a response.
func (c *Capture) Complete(ctx context.Context, id int64, resp *Response) {
	db := c.takePending(id)
	if db == nil || db != c.deps.DB() {
		c.dropped.Add(1)

		return
	}

	if resp != nil {
		c.enqueue(ctx, captureOp{db: db, id: id, entry: Entry{Resp: resp}, complete: true})
	}
}

func (c *Capture) enqueue(ctx context.Context, op captureOp) bool {
	select {
	case c.in <- op:
		return true
	case <-ctx.Done():
		c.dropped.Add(1)

		return false
	}
}

// Run writes batches until ctx ends, then flushes what is queued.
func (c *Capture) Run(ctx context.Context) {
	tick := time.NewTicker(FlushInterval)
	defer tick.Stop()

	var changes <-chan project.Change
	if c.deps.Subscribe != nil {
		changes = c.deps.Subscribe(ctx)
		c.selectProject(ctx)
	}

	batch := make([]captureOp, 0, BatchSize)

	flush := func(ctx context.Context) {
		if len(batch) > 0 {
			c.flush(ctx, batch)
			clear(batch)
			batch = batch[:0]
		}
	}

	for {
		select {
		case change, ok := <-changes:
			if !ok {
				changes = nil
			} else if change.Kind == project.Selected {
				c.selectProject(ctx)
			}
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

func (c *Capture) selectProject(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.refreshProject(ctx); err != nil {
		c.deps.Logger.Error("capture: initialize project IDs", "error", err)
	}
}

func (c *Capture) flush(ctx context.Context, batch []captureOp) {
	db := c.deps.DB()
	var err error

	for i := range batch {
		if batch[i].db == db && batch[i].written == nil {
			err = c.write(ctx, db, batch[i:])
			break
		}
	}

	if err != nil && db == c.deps.DB() {
		c.deps.Logger.Error("capture: write batch", "component", "history", "rows", len(batch), "error", err)
	}

	for _, op := range batch {
		switch {
		case op.written != nil:
			close(op.written)
		case op.db != db || op.id == 0 || err != nil:
			c.dropped.Add(1)
		case c.deps.Bus == nil || db != c.deps.DB():
		default:
			if !op.complete {
				c.deps.Bus.Publish(Event{Kind: Added, RequestID: op.id})
			}
			if op.entry.Resp != nil {
				c.deps.Bus.Publish(Event{Kind: Updated, RequestID: op.id})
			}
		}
	}
}

func (c *Capture) write(ctx context.Context, db *store.DB, batch []captureOp) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		var insertResponse *sql.Stmt
		for i := range batch {
			op := &batch[i]
			if op.db != db || op.written != nil {
				continue
			}
			if !op.complete {
				row := requestRow(&op.entry)
				row.ID = op.id
				if _, err := store.InsertRequestTx(ctx, tx, row); err != nil {
					return err
				}
				if err := insertHeaders(ctx, tx, insertRequestHeaderSQL, op.id, op.entry.Req.Headers); err != nil {
					return err
				}
			}

			if resp := op.entry.Resp; resp != nil {
				if insertResponse == nil {
					stmt, err := tx.PrepareContext(ctx, insertResponseSQL)
					if err != nil {
						return fmt.Errorf("history: prepare response insert: %w", err)
					}
					insertResponse = stmt
				}
				if err := op.writeResponse(ctx, tx, insertResponse, resp); err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func (op *captureOp) writeResponse(ctx context.Context, tx *sql.Tx, insertResponse *sql.Stmt, resp *Response) error {
	res, err := insertResponse.ExecContext(ctx,
		resp.Code, resp.Raw, resp.BodyOffset, len(resp.Raw),
		resp.RoundTrip.Milliseconds(), resp.CreatedAt.UnixMilli(), op.id)
	if err != nil {
		return fmt.Errorf("history: insert response: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("history: count inserted responses: %w", err)
	}
	if n == 0 {
		op.id = 0 // The pending request was deleted or its insert failed.

		return nil
	}
	if resp.Truncated {
		if _, err := tx.ExecContext(ctx, "UPDATE requests SET truncated = 1 WHERE id = ?", op.id); err != nil {
			return fmt.Errorf("history: mark truncated response: %w", err)
		}
	}

	return insertHeaders(ctx, tx, insertResponseHeaderSQL, op.id, resp.Headers)
}

const (
	insertResponseSQL = `INSERT INTO responses
		(request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
		SELECT id, ?, ?, ?, ?, ?, ? FROM requests WHERE id = ?`
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
		Dropped:    e.Dropped,
		EditedFrom: e.EditedFrom,
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}

	return s
}
