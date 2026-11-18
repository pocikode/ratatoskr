package history_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/store"
)

type seedRow struct {
	host, method, path, source string
	port                       int // 0: 80
	code                       int // 0: no response
	color, note                string
	editedFrom                 int64
	req, resp                  string // raw messages; defaults are generated
}

type env struct {
	db  *store.DB
	bus *events.Bus[history.Event]
	svc *history.Service
}

func newEnv(t *testing.T) *env {
	t.Helper()

	db := openDB(t)
	bus := &events.Bus[history.Event]{}

	return &env{db: db, bus: bus, svc: history.NewService(history.ServiceDeps{DB: func() *store.DB { return db }, Bus: bus})}
}

func (e *env) seed(t *testing.T, r seedRow) int64 {
	t.Helper()

	r.source = cmpOr(r.source, "proxy")
	r.method = cmpOr(r.method, "GET")
	r.path = cmpOr(r.path, "/")
	r.host = cmpOr(r.host, "a.test")
	r.req = cmpOr(r.req, fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\nX-A: 1\r\n\r\n", r.method, r.path, r.host))

	var id int64

	err := e.db.Write(t.Context(), func(tx *sql.Tx) error {
		var err error

		id, err = store.InsertRequestTx(t.Context(), tx, store.Request{
			Source: r.source, Host: r.host, Port: cmpOr(r.port, 80), Method: r.method, Path: r.path,
			Raw: []byte(r.req), BodyOffset: strings.Index(r.req, "\r\n\r\n") + 4, CreatedAt: 1000,
		})
		if err != nil {
			return err
		}

		if _, err = tx.Exec(`INSERT INTO request_headers VALUES (?, 0, 'X-A', 'x-a', '1')`, id); err != nil {
			return err
		}

		if r.color != "" {
			if _, err = tx.Exec(`UPDATE requests SET color = ? WHERE id = ?`, r.color, id); err != nil {
				return err
			}
		}

		if r.note != "" {
			if _, err = tx.Exec(`UPDATE requests SET note = ? WHERE id = ?`, r.note, id); err != nil {
				return err
			}
		}

		if r.editedFrom != 0 {
			if _, err = tx.Exec(`UPDATE requests SET edited_from = ? WHERE id = ?`, r.editedFrom, id); err != nil {
				return err
			}
		}

		if r.code == 0 {
			return nil
		}

		raw := cmpOr(r.resp, fmt.Sprintf("HTTP/1.1 %d X\r\nContent-Type: text/plain\r\n\r\nhello", r.code))
		if _, err = tx.Exec(`INSERT INTO responses (request_id, code, raw, body_offset, len, roundtrip_ms, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 2000)`, id, r.code, []byte(raw), strings.Index(raw, "\r\n\r\n")+4, len(raw), r.code%50); err != nil {
			return err
		}

		_, err = tx.Exec(`INSERT INTO response_headers VALUES (?, 0, 'Content-Type', 'content-type', 'text/plain')`, id)

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func cmpOr[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}

	return v
}

func ids(items []history.Item) []int64 {
	out := make([]int64, len(items))
	for i, it := range items {
		out[i] = it.ID
	}

	return out
}

func seedMixed(t *testing.T, e *env) {
	t.Helper()

	hosts := []string{"b.test", "a.test", "c.test", "a.test", "b.test", "c.test", "a.test"}
	colors := []string{"", "RED", "PINK", "", "RED", "BLUE", ""}
	codes := []int{200, 404, 0, 200, 500, 301, 404}

	for i, h := range hosts {
		e.seed(t, seedRow{host: h, code: codes[i], color: colors[i], note: strings.Repeat("n", i%3), path: fmt.Sprintf("/p%d", i%4)})
	}
}

func TestListPagesAreConsistentForEverySort(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	fields := []history.SortField{
		history.SortID, history.SortHost, history.SortMethod, history.SortPath, history.SortQuery, history.SortStatus,
		history.SortResponseLength, history.SortExt, history.SortRoundtrip, history.SortCreatedAt, history.SortTLS,
		history.SortPort, history.SortSource, history.SortColor, history.SortNote, history.SortEdited,
	}

	for _, f := range fields {
		for _, desc := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/desc=%v", f, desc), func(t *testing.T) {
				t.Parallel()

				sort := history.Sort{Field: f, Desc: desc}

				all, err := e.svc.List(t.Context(), history.PageArgs{First: new(100), Sort: sort})
				if err != nil {
					t.Fatal(err)
				}

				if all.Total != 7 || len(all.Items) != 7 || all.HasNext || all.HasPrevious {
					t.Fatalf("full page: total=%d n=%d next=%v prev=%v", all.Total, len(all.Items), all.HasNext, all.HasPrevious)
				}

				// Forward by threes.
				var fwd []int64

				var after *string

				for {
					p, err := e.svc.List(t.Context(), history.PageArgs{First: new(3), After: after, Sort: sort})
					if err != nil {
						t.Fatal(err)
					}

					fwd = append(fwd, ids(p.Items)...)

					if !p.HasNext {
						break
					}

					after = &p.Items[len(p.Items)-1].Cursor
				}

				if !slices.Equal(fwd, ids(all.Items)) {
					t.Fatalf("forward = %v, want %v", fwd, ids(all.Items))
				}

				// Backward by threes from the end.
				var back []int64

				var before *string

				for {
					p, err := e.svc.List(t.Context(), history.PageArgs{Last: new(3), Before: before, Sort: sort})
					if err != nil {
						t.Fatal(err)
					}

					back = append(ids(p.Items), back...)

					if !p.HasPrevious {
						break
					}

					before = &p.Items[0].Cursor
				}

				if !slices.Equal(back, ids(all.Items)) {
					t.Fatalf("backward = %v, want %v", back, ids(all.Items))
				}
			})
		}
	}
}

func TestListSortOrder(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	tests := []struct {
		name string
		sort history.Sort
		want []int64
	}{
		{"id desc", history.Sort{Field: history.SortID, Desc: true}, []int64{7, 6, 5, 4, 3, 2, 1}},
		{"host asc ties by id", history.Sort{Field: history.SortHost}, []int64{2, 4, 7, 1, 5, 3, 6}},
		{"status asc, none lowest", history.Sort{Field: history.SortStatus}, []int64{3, 1, 4, 6, 2, 7, 5}},
		{"colour in declaration order, none lowest", history.Sort{Field: history.SortColor}, []int64{1, 4, 7, 2, 5, 6, 3}},
		{"edited desc ties by id desc", history.Sort{Field: history.SortEdited, Desc: true}, []int64{7, 6, 5, 4, 3, 2, 1}},
	}

	for _, tt := range tests {
		p, err := e.svc.List(t.Context(), history.PageArgs{Sort: tt.sort})
		if err != nil {
			t.Fatal(tt.name, err)
		}

		if got := ids(p.Items); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestListCursorSurvivesConcurrentInserts(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	sort := history.Sort{Field: history.SortHost}

	p1, err := e.svc.List(t.Context(), history.PageArgs{First: new(3), Sort: sort})
	if err != nil {
		t.Fatal(err)
	}

	// New rows land before, inside and after the boundary of page 1.
	e.seed(t, seedRow{host: "0.test"})
	e.seed(t, seedRow{host: "a.test"})
	e.seed(t, seedRow{host: "zz.test"})

	p2, err := e.svc.List(t.Context(), history.PageArgs{First: new(100), After: &p1.Items[2].Cursor, Sort: sort})
	if err != nil {
		t.Fatal(err)
	}

	seen := map[int64]bool{}
	for _, id := range ids(p1.Items) {
		seen[id] = true
	}

	for _, id := range ids(p2.Items) {
		if seen[id] {
			t.Fatalf("row %d on both pages", id)
		}
	}

	// Page 1 was the a.test rows {2,4,7}. Row 8 ("0.test") sorts before the
	// boundary and stays out; the new a.test row 9 sorts after it; the old
	// tail and "zz.test" follow, none missing and none twice.
	if want := []int64{9, 1, 5, 3, 6, 10}; !slices.Equal(ids(p2.Items), want) {
		t.Fatalf("page 2 = %v, want %v", ids(p2.Items), want)
	}
}

func TestListFiltersAndErrors(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	p, err := e.svc.List(t.Context(), history.PageArgs{First: new(2), Query: new(`req.host.eq:"a.test" AND resp.code.gte:400`)})
	if err != nil {
		t.Fatal(err)
	}

	if p.Total != 2 || !slices.Equal(ids(p.Items), []int64{2, 7}) {
		t.Fatalf("filtered = %v total %d", ids(p.Items), p.Total)
	}

	p, err = e.svc.List(t.Context(), history.PageArgs{Query: new(`req.header["x-a"].eq:"1" AND source:proxy`)})
	if err != nil || p.Total != 7 {
		t.Fatalf("header+source filter: total %d, err %v", p.Total, err)
	}

	_, err = e.svc.List(t.Context(), history.PageArgs{Query: new(`req.host.eq:`)})
	if ae, ok := errorsAsApp(err); !ok || ae.Code != apperr.BadQuery || ae.Span == nil {
		t.Fatalf("parse error = %v, want BAD_QUERY with span", err)
	}

	bad := []struct {
		name string
		args history.PageArgs
		want apperr.Code
	}{
		{"first and last", history.PageArgs{First: new(1), Last: new(1)}, apperr.BadInput},
		{"zero page", history.PageArgs{First: new(0)}, apperr.BadInput},
		{"huge page", history.PageArgs{First: new(1001)}, apperr.BadInput},
		{"garbage cursor", history.PageArgs{After: new("!!")}, apperr.BadInput},
		{"unknown scope", history.PageArgs{ScopeID: new("9")}, apperr.NotFound},
		{"unknown sort", history.PageArgs{Sort: history.Sort{Field: "NOPE"}}, apperr.BadInput},
	}

	for _, tt := range bad {
		if _, err := e.svc.List(t.Context(), tt.args); apperr.CodeOf(err) != tt.want {
			t.Errorf("%s: code %v, want %v (%v)", tt.name, apperr.CodeOf(err), tt.want, err)
		}
	}

	// A cursor from one sort order is rejected under another.
	byHost, _ := e.svc.List(t.Context(), history.PageArgs{First: new(1), Sort: history.Sort{Field: history.SortHost}})
	if _, err := e.svc.List(t.Context(), history.PageArgs{After: &byHost.Items[0].Cursor}); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("foreign cursor: %v", err)
	}
}

func errorsAsApp(err error) (*apperr.Error, bool) {
	ae, ok := err.(*apperr.Error) //nolint:errorlint // T-016: services return *apperr.Error unwrapped here
	return ae, ok
}

func TestListFieldsAndResponse(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	orig := e.seed(t, seedRow{host: "o.test", code: 200})
	edited := e.seed(t, seedRow{host: "o.test", code: 200, editedFrom: orig, color: "CYAN", note: "hi"})

	r, err := e.svc.Get(t.Context(), edited)
	if err != nil {
		t.Fatal(err)
	}

	if r.Color != "CYAN" || r.Note != "hi" || r.EditedFrom == nil || *r.EditedFrom != orig || r.Resp == nil ||
		r.Resp.Code != 200 || r.Resp.BodyLength != 5 || r.Resp.RoundTripMs != 0 || r.Source != "proxy" || r.Length == 0 {
		t.Fatalf("request = %+v resp %+v", r, r.Resp)
	}

	if _, err := e.svc.Get(t.Context(), 999); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatalf("missing: %v", err)
	}
}

func TestDeleteByQueryCascades(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	del, err := e.svc.SubscribeDeleted(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Row 8 is an edited copy of row 2 (a.test, 404): it survives with editedFrom null.
	e.seed(t, seedRow{host: "keep.test", code: 200, editedFrom: 2})

	n, err := e.svc.Delete(t.Context(), history.DeleteArgs{Query: new(`req.host.eq:"a.test"`)})
	if err != nil || n != 3 {
		t.Fatalf("deleted %d, err %v", n, err)
	}

	if got := <-del; !slices.Equal(got, []int64{2, 4, 7}) {
		t.Fatalf("event ids = %v", got)
	}

	var reqs, resps, rh, sh int

	err = e.db.Reader().QueryRow(`SELECT (SELECT count(*) FROM requests), (SELECT count(*) FROM responses),
		(SELECT count(*) FROM request_headers), (SELECT count(*) FROM response_headers)`).Scan(&reqs, &resps, &rh, &sh)
	if err != nil {
		t.Fatal(err)
	}

	// 8 requests - 3; responses: 8 seeded with a code except row 3, minus 3 deleted (2,4,7 all had one).
	if reqs != 5 || resps != 4 || rh != 5 || sh != 4 {
		t.Fatalf("left: requests %d responses %d req headers %d resp headers %d", reqs, resps, rh, sh)
	}

	kept, err := e.svc.Get(t.Context(), 8)
	if err != nil || kept.EditedFrom != nil {
		t.Fatalf("edited copy = %+v, %v", kept, err)
	}

	// Ids: unknown ones are ignored.
	n, err = e.svc.Delete(t.Context(), history.DeleteArgs{IDs: []int64{1, 1000}})
	if err != nil || n != 1 {
		t.Fatalf("by ids: %d, %v", n, err)
	}

	// Empty query deletes everything.
	n, err = e.svc.Delete(t.Context(), history.DeleteArgs{Query: new("")})
	if err != nil || n != 4 {
		t.Fatalf("delete all: %d, %v", n, err)
	}

	for _, a := range []history.DeleteArgs{{}, {IDs: []int64{1}, Query: new("")}} {
		if _, err := e.svc.Delete(t.Context(), a); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("args %+v: %v", a, err)
		}
	}
}

func TestDeleteBatchesEvents(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	const total = history.MaxPage + 5

	err := e.db.Write(t.Context(), func(tx *sql.Tx) error {
		for range total {
			if _, err := store.InsertRequestTx(t.Context(), tx, store.Request{Source: "proxy", Host: "h", Method: "GET", Path: "/", Raw: []byte("x")}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	del, err := e.svc.SubscribeDeleted(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if n, err := e.svc.Delete(t.Context(), history.DeleteArgs{Query: new("")}); err != nil || n != total {
		t.Fatalf("deleted %d, %v", n, err)
	}

	if a, b := len(<-del), len(<-del); a != history.MaxPage || b != 5 {
		t.Fatalf("batches %d, %d", a, b)
	}
}

func TestSetColorAndNote(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	seedMixed(t, e)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	sub := e.bus.Subscribe(ctx)

	rs, err := e.svc.SetColor(t.Context(), []int64{3, 1}, "GREEN")
	if err != nil || len(rs) != 2 || rs[0].ID != 3 || rs[1].ID != 1 || rs[0].Color != "GREEN" {
		t.Fatalf("set colour: %+v, %v", rs, err)
	}

	for _, want := range []int64{3, 1} {
		if m := <-sub; m.Value.Kind != history.Updated || m.Value.RequestID != want {
			t.Fatalf("event %+v, want Updated %d", m.Value, want)
		}
	}

	// One unknown ID: nothing changes.
	if _, err := e.svc.SetColor(t.Context(), []int64{2, 99}, "RED"); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatalf("unknown id: %v", err)
	}

	if r, _ := e.svc.Get(t.Context(), 2); r.Color != "RED" { // seeded RED
		t.Fatalf("row 2 changed despite the error: %q", r.Color)
	}

	if _, err := e.svc.SetColor(t.Context(), []int64{1}, "MAUVE"); apperr.CodeOf(err) != apperr.BadInput {
		t.Fatalf("bad colour: %v", err)
	}

	cleared, err := e.svc.SetColor(t.Context(), []int64{1}, "")
	if err != nil || cleared[0].Color != "" {
		t.Fatalf("clear: %+v, %v", cleared, err)
	}

	r, err := e.svc.SetNote(t.Context(), 1, "  look here \n")
	if err != nil || r.Note != "look here" {
		t.Fatalf("note: %q, %v", r.Note, err)
	}

	if r, _ = e.svc.SetNote(t.Context(), 1, "   "); r.Note != "" {
		t.Fatalf("blank note kept: %q", r.Note)
	}

	if _, err := e.svc.SetNote(t.Context(), 99, "x"); apperr.CodeOf(err) != apperr.NotFound {
		t.Fatalf("note unknown: %v", err)
	}
}

func gz(t *testing.T, s string) string {
	t.Helper()

	var b bytes.Buffer

	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()

	return b.String()
}

func encode(t *testing.T, coding, s string) string {
	t.Helper()

	var b bytes.Buffer

	var w io.WriteCloser

	switch coding {
	case "br":
		w = brotli.NewWriter(&b)
	default:
		zw, err := zstd.NewWriter(&b)
		if err != nil {
			t.Fatal(err)
		}

		w = zw
	}

	_, _ = w.Write([]byte(s))
	_ = w.Close()

	return b.String()
}

func TestBodyDecodeIsStreamed(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	var zipped bytes.Buffer

	zw := gzip.NewWriter(&zipped)
	chunk := make([]byte, 1<<20)

	for range 64 { // 64 MiB of zeros
		if _, err := zw.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}

	zw.Close()

	resp := "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: " + strconv.Itoa(zipped.Len()) + "\r\n\r\n" + zipped.String()
	id := e.seed(t, seedRow{code: 200, resp: resp})

	for _, off := range []int64{0, 60 << 20, 64<<20 - 4, 64 << 20} {
		got, err := e.svc.Body(t.Context(), id, history.PartResponse, off, 16, true)
		want := min(16, max(0, 64<<20-int(off)))

		if err != nil || len(got) != want || len(bytes.TrimLeft(got, "\x00")) != 0 {
			t.Errorf("offset %d: len %d (want %d), %v", off, len(got), want, err)
		}
	}
}

func TestHeadAndBody(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	zipped := gz(t, "decoded body text")
	chunked := "5\r\nhello\r\n6\r\n world\r\n0\r\n\r\n"

	plain := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"})
	gzipped := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: " + strconv.Itoa(len(zipped)) + "\r\n\r\n" + zipped})
	chunk := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" + chunked})
	brBody := encode(t, "br", "brotli text")
	zstdBody := encode(t, "zstd", "zstd text")
	br := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nContent-Encoding: br\r\nContent-Length: " + strconv.Itoa(len(brBody)) + "\r\n\r\n" + brBody})
	zst := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nContent-Encoding: zstd\r\nContent-Length: " + strconv.Itoa(len(zstdBody)) + "\r\n\r\n" + zstdBody})
	badBr := e.seed(t, seedRow{code: 200, resp: "HTTP/1.1 200 OK\r\nContent-Encoding: br\r\nContent-Length: 3\r\n\r\nabc"})
	post := e.seed(t, seedRow{method: "POST", req: "POST / HTTP/1.1\r\nHost: a.test\r\nContent-Length: 4\r\n\r\ndata"})

	ctx := t.Context()

	head, err := e.svc.Head(ctx, plain, history.PartResponse)
	if err != nil || string(head) != "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n" {
		t.Fatalf("head = %q, %v", head, err)
	}

	tests := []struct {
		name   string
		id     int64
		part   history.Part
		off    int64
		limit  int
		decode bool
		want   string
	}{
		{"whole", plain, history.PartResponse, 0, 100, false, "hello"},
		{"window", plain, history.PartResponse, 1, 3, false, "ell"},
		{"past end", plain, history.PartResponse, 9, 3, false, ""},
		{"request body", post, history.PartRequest, 0, 10, false, "data"},
		{"gzip as stored is gzip", gzipped, history.PartResponse, 0, 2, false, zipped[:2]},
		{"gzip decoded", gzipped, history.PartResponse, 0, 100, true, "decoded body text"},
		{"gzip decoded window", gzipped, history.PartResponse, 8, 4, true, "body"},
		{"chunked decoded", chunk, history.PartResponse, 0, 100, true, "hello world"},
		{"chunked stored", chunk, history.PartResponse, 0, 100, false, chunked},
		{"br decoded", br, history.PartResponse, 0, 100, true, "brotli text"},
		{"zstd decoded", zst, history.PartResponse, 0, 100, true, "zstd text"},
		{"bad br falls back to stored", badBr, history.PartResponse, 0, 100, true, "abc"},
		{"request decoded", post, history.PartRequest, 0, 100, true, "data"},
	}

	for _, tt := range tests {
		got, err := e.svc.Body(ctx, tt.id, tt.part, tt.off, tt.limit, tt.decode)
		if err != nil || string(got) != tt.want {
			t.Errorf("%s: got %q, %v; want %q", tt.name, got, err, tt.want)
		}
	}

	for _, c := range []struct {
		off   int64
		limit int
	}{{-1, 1}, {0, 0}, {0, history.MaxBodyChunk + 1}} {
		if _, err := e.svc.Body(ctx, plain, history.PartResponse, c.off, c.limit, false); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("offset %d limit %d: %v", c.off, c.limit, err)
		}
	}

	if _, err := e.svc.Body(ctx, post, history.PartResponse, 0, 1, false); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("no response: %v", err)
	}

	if _, err := e.svc.Head(ctx, 999, history.PartRequest); apperr.CodeOf(err) != apperr.NotFound {
		t.Errorf("no request: %v", err)
	}
}

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}

		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
	}

	panic("unreachable")
}

func TestSubscribeAddedFiltersWithEvaluator(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch, err := e.svc.SubscribeAdded(ctx, new(`req.host.eq:"hit.test" AND resp.code.eq:200 AND resp.header["content-type"].cont:"text"`), nil)
	if err != nil {
		t.Fatal(err)
	}

	miss1 := e.seed(t, seedRow{host: "miss.test", code: 200})
	miss2 := e.seed(t, seedRow{host: "hit.test", code: 404})
	hit := e.seed(t, seedRow{host: "hit.test", code: 200})

	for _, id := range []int64{miss1, miss2, hit} {
		e.bus.Publish(history.Event{Kind: history.Added, RequestID: id})
	}

	if r := recv(t, ch); r.ID != hit || r.Host != "hit.test" || r.Resp == nil {
		t.Fatalf("got %+v, want row %d", r, hit)
	}

	if _, err := e.svc.SubscribeAdded(ctx, new(`req.host.eq:`), nil); apperr.CodeOf(err) != apperr.BadQuery {
		t.Fatalf("bad query: %v", err)
	}

	cancel()

	for range ch { // closes once ctx ends
	}
}

func TestSubscribeUpdatedReportsMatches(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	id := e.seed(t, seedRow{code: 200})

	ch, err := e.svc.SubscribeUpdated(ctx, new(`req.color.eq:"RED"`), nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.SetColor(ctx, []int64{id}, "RED"); err != nil {
		t.Fatal(err)
	}

	if u := recv(t, ch); u.ID != id || !u.Matches {
		t.Fatalf("after setting RED: %+v", u)
	}

	if _, err := e.svc.SetColor(ctx, []int64{id}, "BLUE"); err != nil {
		t.Fatal(err)
	}

	if u := recv(t, ch); u.Matches {
		t.Fatalf("after setting BLUE the row must stop matching: %+v", u)
	}
}

func TestSlowSubscriberIsResyncedNotBlocking(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	id := e.seed(t, seedRow{code: 200})

	ch, err := e.svc.SubscribeAdded(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Nobody reads ch. Publishing far beyond every buffer must not block.
	published := make(chan struct{})

	go func() {
		for range 5 * events.SubBuffer {
			e.bus.Publish(history.Event{Kind: history.Added, RequestID: id})
		}

		close(published)
	}()

	select {
	case <-published:
	case <-time.After(10 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// The bus tells a lagging subscriber at its next publish; the stream then
	// ends instead of silently losing events.
	got := 0
	deadline := time.After(10 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				if got >= 5*events.SubBuffer {
					t.Fatal("no events were dropped")
				}

				return
			}

			got++
		case <-deadline:
			t.Fatal("stream never ended")
		default:
			e.bus.Publish(history.Event{Kind: history.Added, RequestID: id})
		}
	}
}

func TestComplete(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.seed(t, seedRow{host: "a.test", code: 200})
	e.seed(t, seedRow{host: "a.test", code: 404})
	e.seed(t, seedRow{host: "b.test", method: "POST", code: 200})

	tests := []struct {
		name       string
		lang       history.Language
		text       string
		cursor     int
		from, to   int
		want       []string // labels, in order
		wantKind   history.CompletionKind
		wantInsert string // first item's insert text, when set
	}{
		{"empty", history.HTTPQL, "", 0, 0, 0, []string{"req", "resp", "row", "source:", "preset:"}, "", ""},
		{"namespace prefix", history.HTTPQL, "re", 2, 0, 2, []string{"req", "resp"}, history.KindNamespace, "req."},
		{"fields", history.HTTPQL, "req.ho", 6, 0, 6, []string{"req.host"}, history.KindField, ""},
		{"operators", history.HTTPQL, "req.port.", 9, 0, 9, []string{"eq", "ne", "gt", "gte", "lt", "lte"}, history.KindOperator, "req.port.eq"},
		{"host values by count", history.HTTPQL, `req.host.eq:`, 12, 12, 12, []string{"a.test", "b.test"}, history.KindValue, `"a.test"`},
		{"host value prefix", history.HTTPQL, `req.host.eq:b`, 13, 12, 13, []string{"b.test"}, history.KindValue, ""},
		{"status values bare", history.HTTPQL, `resp.code.eq:`, 13, 13, 13, []string{"200", "404"}, history.KindValue, "200"},
		{"source", history.HTTPQL, `source:re`, 9, 7, 9, []string{"replay"}, history.KindValue, "replay"},
		{"colour", history.HTTPQL, `req.color.eq:PI`, 15, 13, 15, []string{"PINK"}, history.KindValue, `"PINK"`},
		{"bool", history.HTTPQL, `req.tls.eq:`, 11, 11, 11, []string{"true", "false"}, history.KindValue, "true"},
		{"inside a string", history.HTTPQL, `req.host.cont:"a`, 16, 16, 16, nil, "", ""},
		{"mid token replaces to its end", history.HTTPQL, "req.host", 4, 0, 8, nil, "", ""},
		{"stream fields", history.StreamQL, "ws.d", 4, 0, 4, []string{"ws.direction"}, history.KindField, ""},
		{"stream values", history.StreamQL, "ws.format.eq:", 13, 13, 13, []string{"text", "binary"}, history.KindValue, `"text"`},
		{"garbage input", history.HTTPQL, `((( req.host.`, 13, 4, 13, nil, "", ""},
	}

	for _, tt := range tests {
		got, err := e.svc.Complete(t.Context(), tt.lang, tt.text, tt.cursor)
		if err != nil {
			t.Fatal(tt.name, err)
		}

		if got.From != tt.from || got.To != tt.to {
			t.Errorf("%s: range [%d,%d), want [%d,%d)", tt.name, got.From, got.To, tt.from, tt.to)
		}

		var labels []string
		for _, it := range got.Items {
			labels = append(labels, it.Label)
		}

		if tt.want != nil && !slices.Equal(labels, tt.want) {
			t.Errorf("%s: labels %v, want %v", tt.name, labels, tt.want)
		}

		if len(got.Items) > 0 && tt.wantKind != "" && got.Items[0].Kind != tt.wantKind {
			t.Errorf("%s: kind %v, want %v", tt.name, got.Items[0].Kind, tt.wantKind)
		}

		if len(got.Items) > 0 && tt.wantInsert != "" && got.Items[0].InsertText != tt.wantInsert {
			t.Errorf("%s: insert %q, want %q", tt.name, got.Items[0].InsertText, tt.wantInsert)
		}
	}

	if _, err := e.svc.Complete(t.Context(), history.HTTPQL, "abc", 4); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("cursor out of range: %v", err)
	}

	ext, _ := e.svc.Complete(t.Context(), history.HTTPQL, "req.not", 7)
	if len(ext.Items) != 1 || !ext.Items[0].IsExtension {
		t.Errorf("req.note must be flagged as extension: %+v", ext.Items)
	}
}

func FuzzComplete(f *testing.F) {
	for _, s := range []string{"", "req.", `req.host.eq:"a`, "source:", "((( resp.code.", "\xff\"", "preset:"} {
		f.Add(s, len(s)/2)
	}

	f.Fuzz(func(t *testing.T, text string, cursor int) {
		svc := history.NewService(history.ServiceDeps{})

		got, err := svc.Complete(context.Background(), history.HTTPQL, text, cursor)
		if cursor < 0 || cursor > len(text) {
			if err == nil {
				t.Fatal("out-of-range cursor accepted")
			}

			return
		}

		if err != nil || got.From > cursor || got.To < cursor || got.To > len(text) || len(got.Items) > history.MaxCompletions {
			t.Fatalf("%q@%d -> %+v, %v", text, cursor, got, err)
		}
	})
}
