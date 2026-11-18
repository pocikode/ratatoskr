package history

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

// SortField is a column the history can be sorted by; the values are the
// GraphQL enum RequestSortField.
type SortField string

// Sort fields.
const (
	SortID             SortField = "ID"
	SortHost           SortField = "HOST"
	SortMethod         SortField = "METHOD"
	SortPath           SortField = "PATH"
	SortQuery          SortField = "QUERY"
	SortStatus         SortField = "STATUS"
	SortResponseLength SortField = "RESPONSE_LENGTH"
	SortExt            SortField = "EXT"
	SortRoundtrip      SortField = "ROUNDTRIP"
	SortCreatedAt      SortField = "CREATED_AT"
	SortTLS            SortField = "TLS"
	SortPort           SortField = "PORT"
	SortSource         SortField = "SOURCE"
	SortColor          SortField = "COLOR"
	SortNote           SortField = "NOTE"
	SortEdited         SortField = "EDITED"
)

// sortKey is the SQL expression a field sorts by. It is never NULL, so a
// cursor can compare it with plain operators; text says which cursor slot
// holds the value.
type sortKey struct {
	expr string
	text bool
}

var sortKeys = map[SortField]sortKey{
	SortID:             {expr: "r.id"},
	SortHost:           {expr: "r.host", text: true},
	SortMethod:         {expr: "r.method", text: true},
	SortPath:           {expr: "r.path", text: true},
	SortQuery:          {expr: "r.query", text: true},
	SortStatus:         {expr: "coalesce(s.code, 0)"},
	SortResponseLength: {expr: "coalesce(s.len, 0)"},
	SortExt:            {expr: "r.ext", text: true},
	SortRoundtrip:      {expr: "coalesce(s.roundtrip_ms, 0)"},
	SortCreatedAt:      {expr: "r.created_at"},
	SortTLS:            {expr: "r.tls"},
	SortPort:           {expr: "r.port"},
	SortSource:         {expr: "r.source", text: true},
	SortColor:          {expr: colorKey()},
	SortNote:           {expr: "coalesce(r.note, '')", text: true},
	SortEdited:         {expr: "(r.edited_from IS NOT NULL)"},
}

// colorKey ranks colours in declaration order; no colour is 0.
func colorKey() string {
	var b strings.Builder

	b.WriteString("CASE r.color")

	for i, c := range Colors {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", c, i+1)
	}

	b.WriteString(" ELSE 0 END")

	return b.String()
}

// Sort orders a page; ties are always broken by ID in the same direction.
type Sort struct {
	Field SortField
	Desc  bool
}

// PageArgs are the arguments of [Service.List].
type PageArgs struct {
	First, Last   *int
	After, Before *string
	Query         *string
	ScopeID       *string
	Sort          Sort
}

// Item is one request of a page with its cursor.
type Item struct {
	Request
	Cursor string
}

// Page is the result of [Service.List].
type Page struct {
	Items       []Item
	HasNext     bool
	HasPrevious bool
	Total       int
}

// cursor is the position of a row in a sort order: its sort key and its ID.
// It is opaque to clients (base64url of JSON).
type cursor struct {
	Field SortField `json:"f"`
	N     *int64    `json:"n,omitzero"`
	S     *string   `json:"s,omitzero"`
	ID    int64     `json:"i"`
}

func (c cursor) encode() string {
	b, _ := json.Marshal(c) // a struct of plain values cannot fail

	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string, field SortField, key sortKey) (cursor, error) {
	bad := apperr.New(apperr.BadInput, "invalid cursor")

	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, bad
	}

	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return cursor{}, bad
	}

	if c.Field != field || (c.S != nil) != key.text || (c.N != nil) == key.text {
		return cursor{}, apperr.New(apperr.BadInput, "cursor belongs to a different sort order")
	}

	return c, nil
}

// List returns one page of the history matching the query and scope, and the
// number of matching requests overall.
//
// Pagination is keyset-based (sort key, then ID), so a page boundary survives
// inserts and deletes between calls.
func (s *Service) List(ctx context.Context, a PageArgs) (Page, error) {
	n, backward, err := pageSize(a)
	if err != nil {
		return Page{}, err
	}

	field := cmp.Or(a.Sort.Field, SortID)

	key, ok := sortKeys[field]
	if !ok {
		return Page{}, apperr.New(apperr.BadInput, "unknown sort field %q", a.Sort.Field)
	}

	f, err := s.filter(ctx, a.Query, a.ScopeID)
	if err != nil {
		return Page{}, err
	}

	db, err := s.db()
	if err != nil {
		return Page{}, err
	}

	// Walking backwards reads the reversed order and flips the rows after.
	desc := a.Sort.Desc != backward
	where, args := f.where(), slices.Clone(f.sql.Args)

	after := a.After
	if backward {
		after = a.Before
	}

	if after != nil {
		c, err := decodeCursor(*after, field, key)
		if err != nil {
			return Page{}, err
		}

		op := ">"
		if desc {
			op = "<"
		}

		kv := any(c.N)
		if key.text {
			kv = c.S
		}

		where = "(" + where + ") AND (" + key.expr + " " + op + " ? OR (" + key.expr + " = ? AND r.id " + op + " ?))"
		args = append(args, kv, kv, c.ID)
	}

	dir := " ASC"
	if desc {
		dir = " DESC"
	}

	rd := db.Reader()
	q := `SELECT ` + requestCols + `, ` + key.expr + ` AS k` + fromJoin + //nolint:gosec // T-016: parts are package constants or httpql SQL with bind parameters
		` WHERE ` + where + ` ORDER BY k` + dir + `, r.id` + dir + ` LIMIT ?`

	rows, err := rd.QueryContext(ctx, q, append(args, n+1)...)
	if err != nil {
		return Page{}, fmt.Errorf("history: list: %w", err)
	}
	defer rows.Close()

	var items []Item

	for rows.Next() {
		var kn sql.NullInt64

		var ks sql.NullString

		var extra any = &kn
		if key.text {
			extra = &ks
		}

		r, err := scanRequest(rows, extra)
		if err != nil {
			return Page{}, fmt.Errorf("history: list: %w", err)
		}

		c := cursor{Field: field, ID: r.ID}
		if key.text {
			c.S = &ks.String
		} else {
			c.N = &kn.Int64
		}

		items = append(items, Item{Request: r, Cursor: c.encode()})
	}

	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("history: list: %w", err)
	}

	more := len(items) > n
	if more {
		items = items[:n]
	}

	if backward {
		slices.Reverse(items)
	}

	p := Page{Items: items}
	if backward {
		p.HasPrevious, p.HasNext = more, a.Before != nil
	} else {
		p.HasNext, p.HasPrevious = more, a.After != nil
	}

	// shortcut: totalCount is a fresh count(*) per page, not cached per
	// query; cache it behind an event-invalidated generation when 100k-row
	// scrolling shows it in profiles.
	err = rd.QueryRowContext(ctx, `SELECT count(*)`+fromJoin+` WHERE `+f.where(), f.sql.Args...).Scan(&p.Total)
	if err != nil {
		return Page{}, fmt.Errorf("history: count: %w", err)
	}

	return p, nil
}

// pageSize validates first/last and returns the page size and direction.
func pageSize(a PageArgs) (n int, backward bool, err error) {
	switch {
	case a.First != nil && a.Last != nil:
		return 0, false, apperr.New(apperr.BadInput, "give only one of first and last")
	case a.Last != nil:
		n, backward = *a.Last, true
	case a.First != nil:
		n = *a.First
	default:
		n = 100
	}

	if n < 1 || n > MaxPage {
		return 0, false, apperr.New(apperr.BadInput, "page size must be between 1 and %d", MaxPage)
	}

	return n, backward, nil
}
