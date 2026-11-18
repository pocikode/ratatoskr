package httpql

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/store"
)

// sample is one conformance row: the wire bytes of an exchange plus the
// columns the wire does not carry (see the corpus archives).
type sample struct {
	id                    int64
	source, host          string
	color, note           string
	port, roundtripMS     int
	tls, edited           bool
	createdAt             time.Time
	method, target, p, qs string // request target split into path and query
	ext                   string
	reqRaw, respRaw       []byte
	reqBodyAt, respBodyAt int
	reqHeaders, respHdrs  []query.Header
	code                  int
}

func newSample(tb testing.TB, c conformanceCase) *sample {
	tb.Helper()
	s := &sample{
		id:          atoi(tb, c.meta["id"]),
		source:      c.meta["source"],
		host:        c.meta["host"],
		color:       c.meta["color"],
		note:        c.meta["note"],
		port:        int(atoi(tb, c.meta["port"])),
		roundtripMS: int(atoi(tb, c.meta["roundtrip"])),
		tls:         c.meta["tls"] == "true",
		edited:      c.meta["edited"] == "true",
		reqRaw:      c.request,
		respRaw:     c.response,
	}
	created, err := time.Parse(time.RFC3339, c.meta["created_at"])
	if err != nil {
		tb.Fatalf("%s: created_at: %v", c.name, err)
	}
	s.createdAt = created

	var start string
	start, s.reqHeaders, s.reqBodyAt = splitHead(tb, c.name, c.request)
	fields := strings.Fields(start)
	if len(fields) != 3 {
		tb.Fatalf("%s: request line %q", c.name, start)
	}
	s.method, s.target = fields[0], fields[1]
	s.p, s.qs, _ = strings.Cut(s.target, "?")
	s.ext = path.Ext(s.p)

	start, s.respHdrs, s.respBodyAt = splitHead(tb, c.name, c.response)
	fields = strings.Fields(start)
	if len(fields) < 2 {
		tb.Fatalf("%s: status line %q", c.name, start)
	}
	s.code = int(atoi(tb, fields[1]))
	return s
}

func atoi(tb testing.TB, s string) int64 {
	tb.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		tb.Fatalf("%q is not an integer", s)
	}
	return n
}

func splitHead(tb testing.TB, name string, raw []byte) (start string, headers []query.Header, bodyAt int) {
	tb.Helper()
	i := bytes.Index(raw, []byte("\r\n\r\n"))
	if i < 0 {
		tb.Fatalf("%s: message has no blank line", name)
	}
	lines := strings.Split(string(raw[:i]), "\r\n")
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			tb.Fatalf("%s: header line %q", name, line)
		}
		headers = append(headers, query.Header{Name: key, Value: strings.TrimSpace(value)})
	}
	return lines[0], headers, i + 4
}

// sampleMessage is the in-memory view of a conformance row.
type sampleMessage struct{ s *sample }

func (m sampleMessage) ID() int64            { return m.s.id }
func (m sampleMessage) Source() string       { return m.s.source }
func (m sampleMessage) Host() string         { return m.s.host }
func (m sampleMessage) Port() int            { return m.s.port }
func (m sampleMessage) TLS() bool            { return m.s.tls }
func (m sampleMessage) Method() string       { return m.s.method }
func (m sampleMessage) Path() string         { return m.s.p }
func (m sampleMessage) Query() string        { return m.s.qs }
func (m sampleMessage) Ext() string          { return m.s.ext }
func (m sampleMessage) CreatedAt() time.Time { return m.s.createdAt }
func (m sampleMessage) Color() string        { return m.s.color }
func (m sampleMessage) Note() string         { return m.s.note }
func (m sampleMessage) Edited() bool         { return m.s.edited }
func (m sampleMessage) Request() query.Side {
	return query.Side{Raw: m.s.reqRaw, BodyOffset: m.s.reqBodyAt, Headers: m.s.reqHeaders}
}
func (m sampleMessage) Response() *query.Response {
	if m.s.respRaw == nil {
		return nil
	}
	return &query.Response{
		Raw: m.s.respRaw, BodyOffset: m.s.respBodyAt, Headers: m.s.respHdrs,
		Code:      m.s.code,
		RoundTrip: time.Duration(m.s.roundtripMS) * time.Millisecond,
	}
}

func insertSample(tb testing.TB, db *store.DB, s *sample) {
	tb.Helper()
	err := db.Write(tb.Context(), func(tx *sql.Tx) error {
		var editedFrom any
		if s.edited {
			// The corpus only says "edited"; a self-reference satisfies the FK.
			editedFrom = s.id
		}
		if _, err := tx.ExecContext(tb.Context(), `INSERT INTO requests
			(id, source, host, port, tls, method, path, query, ext, raw, body_offset, len,
			 created_at, edited_from, color, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.source, s.host, s.port, boolInt(s.tls), s.method, s.p, s.qs, s.ext,
			s.reqRaw, s.reqBodyAt, len(s.reqRaw), s.createdAt.UnixMilli(), editedFrom,
			nullIfEmpty(s.color), nullIfEmpty(s.note)); err != nil {
			return err
		}
		if s.respRaw != nil {
			if _, err := tx.ExecContext(tb.Context(), `INSERT INTO responses
				(request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				s.id, s.code, s.respRaw, s.respBodyAt, len(s.respRaw), s.roundtripMS,
				s.createdAt.UnixMilli()); err != nil {
				return err
			}
		}
		for _, h := range []struct {
			table   string
			headers []query.Header
		}{{"request_headers", s.reqHeaders}, {"response_headers", s.respHdrs}} {
			for i, hdr := range h.headers {
				if _, err := tx.ExecContext(tb.Context(), `INSERT INTO `+h.table+`
					(request_id, idx, name, name_lc, value) VALUES (?, ?, ?, ?, ?)`,
					s.id, i, hdr.Name, asciiLower(hdr.Name), hdr.Value); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("insert sample %d: %v", s.id, err)
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// TestConformanceBackends runs the shared corpus through the SQL compiler and
// the in-memory evaluator; both must give the expected result
// (docs/04-query-language.md §4-§5).
func TestConformanceBackends(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cases := loadConformance(t)
	samples := make(map[string]*sample)
	for _, c := range cases {
		key := c.meta["id"]
		if _, ok := samples[key]; ok {
			continue
		}
		s := newSample(t, c)
		samples[key] = s
		insertSample(t, db, s)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := Parse(c.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			m := samples[c.meta["id"]]

			got, err := Eval(n, sampleMessage{m})
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got != c.want {
				t.Errorf("evaluator = %v, want %v", got, c.want)
			}

			compiled, err := Compile(n)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := strings.Count(compiled.Where, "?"); got != len(compiled.Args) {
				t.Fatalf("%d placeholders for %d args: %s", got, len(compiled.Args), compiled.Where)
			}
			gotSQL, err := sqlMatches(t.Context(), db, compiled, m.id)
			if err != nil {
				t.Fatalf("sql: %v", err)
			}
			if gotSQL != c.want {
				t.Errorf("sql compiler = %v, want %v (where %q, args %v)", gotSQL, c.want, compiled.Where, compiled.Args)
			}
		})
	}
}

func sqlMatches(ctx context.Context, db *store.DB, q SQL, id int64) (bool, error) {
	var n int
	err := db.Reader().QueryRowContext(ctx,
		`SELECT count(*) FROM requests r LEFT JOIN responses s ON s.request_id = r.id
		 WHERE `+q.Where+` AND r.id = ?`, append(q.Args, id)...).Scan(&n)
	return n > 0, err
}
