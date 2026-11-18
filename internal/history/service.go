package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
	"github.com/pocikode/ratatoskr/internal/store"
)

// MaxPage is the largest page a client may ask for.
const MaxPage = 1000

// Request is one stored request without its raw bytes (those are read on
// demand by [Service.Head] and [Service.Body]).
type Request struct {
	ID         int64
	Source     string
	Host       string
	Port       int
	TLS        bool
	Method     string
	Path       string
	Query      string
	Ext        string
	Length     int64 // whole request as sent
	BodyLength int64 // stored body bytes
	CreatedAt  time.Time
	Color      string // "" for none
	Note       string // "" for none
	EditedFrom *int64
	Dropped    bool
	Truncated  bool
	Resp       *ResponseInfo
}

// ResponseInfo describes the stored response of a [Request].
type ResponseInfo struct {
	Code        int
	Length      int64
	BodyLength  int64
	RoundTripMs int64
	CreatedAt   time.Time
}

// Colors are the row colours in sort order (F-HIST-1).
var Colors = []string{"RED", "ORANGE", "YELLOW", "GREEN", "CYAN", "BLUE", "PURPLE", "PINK"}

// ScopeFunc resolves a scope preset ID to the filter it stands for. It returns
// an [apperr.NotFound] error for an unknown ID.
type ScopeFunc func(ctx context.Context, id string) (core.Node, error)

// ServiceDeps are the inputs to [NewService].
type ServiceDeps struct {
	// DB returns the open project database, or nil when none is open.
	DB func() *store.DB
	// Bus carries stored-request events from capture and from this service.
	Bus *events.Bus[Event]
	// Scope resolves scopeId arguments; nil means no scope exists.
	Scope ScopeFunc
}

// Service reads and changes the stored history and streams its changes.
type Service struct {
	deps ServiceDeps
}

// NewService returns a Service.
func NewService(deps ServiceDeps) *Service { return &Service{deps: deps} }

func (s *Service) db() (*store.DB, error) {
	if s.deps.DB != nil {
		if db := s.deps.DB(); db != nil {
			return db, nil
		}
	}

	return nil, apperr.New(apperr.Conflict, "no project is open")
}

func (s *Service) publish(ev Event) {
	if s.deps.Bus != nil {
		s.deps.Bus.Publish(ev)
	}
}

// filter is a parsed query plus scope: the AST for the evaluator and its SQL.
type filter struct {
	node core.Node
	sql  httpql.SQL
}

func (s *Service) filter(ctx context.Context, query, scopeID *string) (filter, error) {
	var node core.Node

	if query != nil {
		n, err := httpql.Parse(*query)
		if err != nil {
			return filter{}, queryErr(err)
		}

		node = n
	}

	if scopeID != nil {
		// TODO(T-090): wire Scope in internal/app; until then every scopeId is NOT_FOUND.
		if s.deps.Scope == nil {
			return filter{}, apperr.New(apperr.NotFound, "scope %q not found", *scopeID)
		}

		sc, err := s.deps.Scope(ctx, *scopeID)
		if err != nil {
			return filter{}, err
		}

		node = and(node, sc)
	}

	q, err := httpql.Compile(node)
	if err != nil {
		return filter{}, queryErr(err)
	}

	return filter{node: node, sql: q}, nil
}

func and(l, r core.Node) core.Node {
	switch {
	case l == nil:
		return r
	case r == nil:
		return l
	}

	return &core.And{L: l, R: r}
}

// where returns the WHERE clause of f ("" when it matches everything).
func (f filter) where() string {
	if f.sql.Where == "" {
		return "1"
	}

	return f.sql.Where
}

// queryErr maps a query-language error to BAD_QUERY with its span.
func queryErr(err error) error {
	if ce, ok := errors.AsType[*core.Error](err); ok {
		return apperr.Query(apperr.Span{Start: ce.Span.Start, End: ce.Span.End}, "%s", ce.Msg)
	}

	return err
}

const (
	fromJoin = ` FROM requests r LEFT JOIN responses s ON s.request_id = r.id`

	requestCols = `r.id, r.source, r.host, r.port, r.tls, r.method, r.path, r.query, r.ext,
		r.len, r.len - r.body_offset, r.created_at, r.color, r.note, r.edited_from, r.is_dropped, r.truncated,
		s.code, s.len, s.len - s.body_offset, s.roundtrip_ms, s.created_at`
)

type scanner interface{ Scan(dest ...any) error }

// scanRequest reads requestCols followed by extra destinations.
func scanRequest(sc scanner, extra ...any) (Request, error) {
	var (
		r                 Request
		created           int64
		color, note       sql.NullString
		edited            sql.NullInt64
		code, rlen, rbody sql.NullInt64
		rtt, rcreated     sql.NullInt64
	)

	dest := append([]any{
		&r.ID, &r.Source, &r.Host, &r.Port, &r.TLS, &r.Method, &r.Path, &r.Query, &r.Ext,
		&r.Length, &r.BodyLength, &created, &color, &note, &edited, &r.Dropped, &r.Truncated,
		&code, &rlen, &rbody, &rtt, &rcreated,
	}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return Request{}, err //nolint:wrapcheck // T-016: wrapped by the caller with its operation
	}

	r.CreatedAt = time.UnixMilli(created).UTC()
	r.Color, r.Note = color.String, note.String

	if edited.Valid {
		r.EditedFrom = &edited.Int64
	}

	if code.Valid {
		r.Resp = &ResponseInfo{
			Code: int(code.Int64), Length: rlen.Int64, BodyLength: rbody.Int64,
			RoundTripMs: rtt.Int64, CreatedAt: time.UnixMilli(rcreated.Int64).UTC(),
		}
	}

	return r, nil
}

// Get returns one request; [apperr.NotFound] when there is none.
func (s *Service) Get(ctx context.Context, id int64) (Request, error) {
	db, err := s.db()
	if err != nil {
		return Request{}, err
	}

	return get(ctx, db.Reader(), id)
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func get(ctx context.Context, q queryer, id int64) (Request, error) {
	r, err := scanRequest(q.QueryRowContext(ctx, `SELECT `+requestCols+fromJoin+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Request{}, apperr.New(apperr.NotFound, "request %d not found", id)
	}

	if err != nil {
		return Request{}, fmt.Errorf("history: get request %d: %w", id, err)
	}

	return r, nil
}

// placeholders returns "?,?,…" for n values.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
