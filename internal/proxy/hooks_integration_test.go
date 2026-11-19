//go:build integration

package proxy_test

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/events"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/intercept"
	"github.com/pocikode/ratatoskr/internal/proxy"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/store"
	"github.com/pocikode/ratatoskr/internal/testutil"
)

// holdCall is one HoldRequest/HoldResponse the test answers.
type holdCall struct {
	kind  intercept.Kind
	msg   query.Message
	reply chan intercept.Decision
	ended chan struct{} // closed when the hold returns
}

// fakeInterceptor hands every hold to the test and checks that the record is
// readable when the hold starts.
type fakeInterceptor struct {
	t     *testing.T
	svc   *history.Service
	kinds map[intercept.Kind]bool
	calls chan holdCall
}

func (f *fakeInterceptor) WouldHold(kind intercept.Kind) bool { return f.kinds[kind] }

func (f *fakeInterceptor) HoldRequest(ctx context.Context, db *store.DB, msg query.Message) (intercept.Decision, error) {
	return f.hold(ctx, intercept.Request, db, msg)
}

func (f *fakeInterceptor) HoldResponse(ctx context.Context, db *store.DB, msg query.Message) (intercept.Decision, error) {
	return f.hold(ctx, intercept.Response, db, msg)
}

func (f *fakeInterceptor) hold(ctx context.Context, kind intercept.Kind, db *store.DB, msg query.Message) (intercept.Decision, error) {
	if db == nil {
		f.t.Errorf("hold %s %d without a project database", kind, msg.ID())
	}

	if _, err := f.svc.Get(ctx, msg.ID()); err != nil {
		f.t.Errorf("hold %s %d before its record is readable: %v", kind, msg.ID(), err)
	}

	call := holdCall{kind: kind, msg: msg, reply: make(chan intercept.Decision, 1), ended: make(chan struct{})}
	defer close(call.ended)

	select {
	case f.calls <- call:
	case <-ctx.Done():
		return intercept.Decision{}, ctx.Err()
	}

	select {
	case d := <-call.reply:
		return d, nil
	case <-ctx.Done():
		return intercept.Decision{}, ctx.Err()
	}
}

// fakeRewriter adds a header per source: "X-Mr-<source>: 1" to requests and
// responses whose side is active.
type fakeRewriter struct {
	active map[string][2]bool // source → {request, response}
	// edit replaces addHeader for requests when set.
	edit func(raw []byte) []byte

	mu       sync.Mutex
	calls    []string
	respReqs []string // req as given to Response
}

func (f *fakeRewriter) Active(source string, response bool) bool {
	return f.active[source][boolIndex(response)]
}

func (f *fakeRewriter) Request(_ context.Context, source string, raw []byte) ([]byte, error) {
	f.record("req:" + source)

	if f.edit != nil {
		return f.edit(raw), nil
	}

	return addHeader(raw, "X-Mr-"+source), nil
}

func (f *fakeRewriter) Response(_ context.Context, source string, req, resp []byte) ([]byte, error) {
	f.record("resp:" + source)

	f.mu.Lock()
	f.respReqs = append(f.respReqs, string(req))
	f.mu.Unlock()

	return addHeader(resp, "X-Mr-"+source), nil
}

func (f *fakeRewriter) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, s)
}

func (f *fakeRewriter) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.calls...)
}

func boolIndex(b bool) int {
	if b {
		return 1
	}

	return 0
}

func addHeader(raw []byte, name string) []byte {
	return []byte(strings.Replace(string(raw), "\r\n\r\n", "\r\n"+name+": 1\r\n\r\n", 1))
}

// hookRig is a proxy with real capture and history in front of a RawServer.
type hookRig struct {
	t      *testing.T
	ctx    context.Context
	proxy  *rig
	svc    *history.Service
	up     *testutil.RawServer
	addr   string // upstream address for send; up's when empty
	events <-chan events.Msg[history.Event]
	icpt   *fakeInterceptor
}

func newHookRig(t *testing.T, held []intercept.Kind, rw proxy.Rewriter, limits rawhttp.Limits) *hookRig {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	db, err := store.OpenProject(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bus := &events.Bus[history.Event]{}
	dbFn := func() *store.DB { return db }
	capture := history.New(history.Deps{DB: dbFn, Bus: bus})
	svc := history.NewService(history.ServiceDeps{DB: dbFn})
	sub := bus.Subscribe(ctx)

	runCtx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { capture.Run(runCtx); close(done) }()
	t.Cleanup(func() { stop(); <-done })

	icpt := &fakeInterceptor{t: t, svc: svc, kinds: map[intercept.Kind]bool{}, calls: make(chan holdCall)}
	for _, k := range held {
		icpt.kinds[k] = true
	}

	r := startProxy(t, func(d *proxy.Deps) {
		d.Capture, d.Limits = capture, limits
		d.Interceptor = icpt
		if rw != nil {
			d.Rewriter = rw
		}
	})

	return &hookRig{t: t, ctx: ctx, proxy: r, svc: svc, up: testutil.NewRawServer(t), events: sub, icpt: icpt}
}

// send writes a request through the proxy on a new connection; %s in format
// is replaced by the upstream address.
func (h *hookRig) send(format string) (net.Conn, *bufio.Reader) {
	h.t.Helper()

	conn, err := (&net.Dialer{}).DialContext(h.ctx, "tcp", h.proxy.addr)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = conn.Close() })

	deadline, _ := h.ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	if _, err := io.WriteString(conn, strings.ReplaceAll(format, "%s", h.upAddr())); err != nil {
		h.t.Fatal(err)
	}

	return conn, bufio.NewReader(conn)
}

func (h *hookRig) upAddr() string {
	if h.addr != "" {
		return h.addr
	}

	return fmt.Sprintf("%s:%d", h.up.Host, h.up.Port)
}

// nextHold returns the next hold of kind.
func (h *hookRig) nextHold(kind intercept.Kind) holdCall {
	h.t.Helper()

	select {
	case call := <-h.icpt.calls:
		if call.kind != kind {
			h.t.Fatalf("hold kind = %s, want %s", call.kind, kind)
		}

		return call
	case <-h.ctx.Done():
		h.t.Fatalf("waiting for a %s hold: %v", kind, h.ctx.Err())

		return holdCall{}
	}
}

// response reads one response off the client connection.
func (h *hookRig) response(br *bufio.Reader, method string) string {
	h.t.Helper()

	resp, err := rawhttp.ReadResponse(br, method, rawhttp.Limits{})
	if err != nil {
		h.t.Fatal(err)
	}

	if _, err := io.Copy(io.Discard, resp.Body()); err != nil {
		h.t.Fatal(err)
	}

	return string(resp.Raw())
}

// records waits until n records exist and ok accepts them, then returns them by ID.
func (h *hookRig) records(n int, ok func([]history.Item) bool) []history.Item {
	h.t.Helper()

	for {
		page, err := h.svc.List(h.ctx, history.PageArgs{})
		if err != nil {
			h.t.Fatal(err)
		}

		if len(page.Items) == n && ok(page.Items) {
			slices.SortFunc(page.Items, func(a, b history.Item) int { return cmp.Compare(a.ID, b.ID) })

			return page.Items
		}

		select {
		case <-h.events:
		case <-h.ctx.Done():
			h.t.Fatalf("waiting for %d records, have %+v: %v", n, page.Items, h.ctx.Err())
		}
	}
}

func complete(items []history.Item) bool {
	for _, it := range items {
		if it.Resp == nil {
			return false
		}
	}

	return true
}

// raw returns a stored side exactly.
func (h *hookRig) raw(id int64, part history.Part) string {
	h.t.Helper()

	head, err := h.svc.Head(h.ctx, id, part)
	if err != nil {
		h.t.Fatal(err)
	}

	body, err := h.svc.Body(h.ctx, id, part, 0, 1<<20, false)
	if err != nil {
		h.t.Fatal(err)
	}

	return string(head) + string(body)
}

func (h *hookRig) upstreamRequests() []string {
	var out []string
	for _, r := range h.up.Requests() {
		out = append(out, string(r))
	}

	return out
}

func TestInterceptRequestForwardWithEdits(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})

	sent := "POST http://%s/a HTTP/1.1\r\nHost: %s\r\nContent-Length: 4\r\nConnection: close\r\n\r\nbody"
	conn, br := h.send(sent)

	call := h.nextHold(intercept.Request)
	if got, want := string(call.msg.Request().Raw), strings.ReplaceAll(sent, "%s", h.upAddr()); got != want {
		t.Fatalf("held request = %q, want %q", got, want)
	}

	// The client stays blocked while the request is held.
	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, err := br.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Fatalf("client read %d bytes during the hold (err %v)", n, err)
	}
	deadline, _ := h.ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)

	// Editing Host and the target's authority must not change the destination.
	edited := "POST http://elsewhere.invalid:1/b HTTP/1.1\r\nHost: elsewhere.invalid\r\nContent-Length: 6\r\nConnection: close\r\n\r\nedited"
	call.reply <- intercept.Decision{Raw: []byte(edited), Edited: true}

	if got := h.response(br, "POST"); got != testutil.RawReply {
		t.Fatalf("client got %q", got)
	}

	wantUp := "POST /b HTTP/1.1\r\nHost: elsewhere.invalid\r\nContent-Length: 6\r\nConnection: close\r\n\r\nedited"
	if got := h.upstreamRequests(); len(got) != 1 || got[0] != wantUp {
		t.Fatalf("upstream got %q, want [%q]", got, wantUp)
	}

	recs := h.records(2, complete)
	orig, ed := recs[0], recs[1]

	if orig.ID != call.msg.ID() || orig.EditedFrom != nil || orig.Source != "proxy" || orig.Dropped {
		t.Fatalf("original = %+v", orig.Request)
	}

	if ed.EditedFrom == nil || *ed.EditedFrom != orig.ID || ed.Source != "intercept" || ed.Path != "/b" {
		t.Fatalf("edited = %+v", ed.Request)
	}

	checks := []struct {
		id   int64
		part history.Part
		want string
	}{
		{orig.ID, history.PartRequest, strings.ReplaceAll(sent, "%s", h.upAddr())},
		{orig.ID, history.PartResponse, testutil.RawReply},
		{ed.ID, history.PartRequest, wantUp}, // as the origin server received it
		{ed.ID, history.PartResponse, testutil.RawReply},
	}
	for _, c := range checks {
		if got := h.raw(c.id, c.part); got != c.want {
			t.Errorf("record %d part %d = %q, want %q", c.id, c.part, got, c.want)
		}
	}
}

func TestInterceptDropRequest(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})

	_, br := h.send("GET http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n")
	h.nextHold(intercept.Request).reply <- intercept.Decision{DropStatusCode: 418}

	if got := h.response(br, "GET"); !strings.HasPrefix(got, "HTTP/1.1 418 I'm a teapot\r\n") || !strings.Contains(got, "Ratatoskr") {
		t.Fatalf("client got %q", got)
	}

	recs := h.records(1, func(items []history.Item) bool { return items[0].Dropped })
	if recs[0].Resp != nil {
		t.Fatalf("dropped request has a response: %+v", recs[0].Resp)
	}

	if got := h.upstreamRequests(); len(got) != 0 {
		t.Fatalf("upstream got %q", got)
	}
}

func TestInterceptDropResponse(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Response}, nil, rawhttp.Limits{})

	_, br := h.send("GET http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n")

	call := h.nextHold(intercept.Response)
	if call.msg.Response() == nil || string(call.msg.Response().Raw) != testutil.RawReply {
		t.Fatalf("held response = %+v", call.msg.Response())
	}

	call.reply <- intercept.Decision{DropStatusCode: 503}

	if got := h.response(br, "GET"); !strings.HasPrefix(got, "HTTP/1.1 503 Service Unavailable\r\n") {
		t.Fatalf("client got %q", got)
	}

	recs := h.records(1, func(items []history.Item) bool { return items[0].Dropped && items[0].Resp != nil })
	if recs[0].Resp.Code != 200 || h.raw(recs[0].ID, history.PartResponse) != testutil.RawReply {
		t.Fatalf("dropped response record = %+v", recs[0].Resp)
	}
}

func TestInterceptResponseForwardWithEdits(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Response}, nil, rawhttp.Limits{})

	sent := "GET http://%s/r HTTP/1.1\r\nHost: %s\r\n\r\n"
	_, br := h.send(sent)

	edited := "HTTP/1.1 201 Created\r\nContent-Length: 3\r\n\r\nnew"
	h.nextHold(intercept.Response).reply <- intercept.Decision{Raw: []byte(edited), Edited: true}

	if got := h.response(br, "GET"); got != edited {
		t.Fatalf("client got %q, want %q", got, edited)
	}

	recs := h.records(2, complete)
	orig, ed := recs[0], recs[1]

	if ed.EditedFrom == nil || *ed.EditedFrom != orig.ID || ed.Source != "intercept" || ed.Resp.Code != 201 {
		t.Fatalf("edited = %+v / %+v", ed.Request, ed.Resp)
	}

	wantReq := strings.ReplaceAll(sent, "%s", h.upAddr())
	if a, b := h.raw(orig.ID, history.PartRequest), h.raw(ed.ID, history.PartRequest); a != wantReq || b != wantReq {
		t.Fatalf("request bytes: original %q, edited %q, want both %q", a, b, wantReq)
	}

	if a, b := h.raw(orig.ID, history.PartResponse), h.raw(ed.ID, history.PartResponse); a != testutil.RawReply || b != edited {
		t.Fatalf("responses: original %q, edited %q", a, b)
	}
}

func TestInterceptClientGoneLeavesQueue(t *testing.T) {
	for name, pipelined := range map[string]string{"idle": "", "pipelined": "GET http://%s/next HTTP/1.1\r\nHost: %s\r\n"} {
		t.Run(name, func(t *testing.T) { testClientGone(t, pipelined) })
	}
}

func testClientGone(t *testing.T, pipelined string) {
	h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})

	conn, _ := h.send("GET http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n" + pipelined)
	call := h.nextHold(intercept.Request)
	_ = conn.Close()

	select {
	case <-call.ended: // the hold context ended with the client
	case <-h.ctx.Done():
		t.Fatal("hold outlived its client")
	}

	recs := h.records(1, func([]history.Item) bool { return true })
	if recs[0].Resp != nil || recs[0].Dropped {
		t.Fatalf("record = %+v", recs[0].Request)
	}

	if got := h.upstreamRequests(); len(got) != 0 {
		t.Fatalf("upstream got %q", got)
	}
}

func TestOverCapPassesThroughUnheldAndUnchanged(t *testing.T) {
	tests := []struct {
		name string
		req  string
	}{
		{"content-length", "POST http://%s/u HTTP/1.1\r\nHost: %s\r\nContent-Length: 20\r\n\r\n01234567890123456789"},
		{"chunked", "POST http://%s/u HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n14\r\n01234567890123456789\r\n0\r\n\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw := &fakeRewriter{active: map[string][2]bool{"proxy": {true, true}}}
			// A 1-byte cap: both the request body and RawReply's "ok" are over it.
			h := newHookRig(t, []intercept.Kind{intercept.Request, intercept.Response}, rw, rawhttp.Limits{MaxBodyBytes: 1})

			_, br := h.send(tt.req)

			if got := h.response(br, "POST"); got != testutil.RawReply {
				t.Fatalf("client got %q", got)
			}

			want := strings.Replace(strings.ReplaceAll(tt.req, "%s", h.upAddr()), "http://"+h.upAddr(), "", 1)
			if got := h.upstreamRequests(); len(got) != 1 || got[0] != want {
				t.Fatalf("upstream got %q, want [%q]", got, want)
			}

			if calls := rw.seen(); len(calls) != 0 {
				t.Fatalf("rewriter ran: %q", calls)
			}

			recs := h.records(1, complete)
			if !recs[0].Truncated {
				t.Fatalf("record not truncated: %+v", recs[0].Request)
			}

			select {
			case call := <-h.icpt.calls:
				t.Fatalf("held %s", call.kind)
			default:
			}
		})
	}
}

func TestRewriterRequestPair(t *testing.T) {
	rw := &fakeRewriter{active: map[string][2]bool{"proxy": {true, false}, "intercept": {true, false}}}
	h := newHookRig(t, nil, rw, rawhttp.Limits{})

	_, br := h.send("GET http://%s/m HTTP/1.1\r\nHost: %s\r\n\r\n")
	if got := h.response(br, "GET"); got != testutil.RawReply {
		t.Fatalf("client got %q", got)
	}

	if got := h.upstreamRequests(); len(got) != 1 || !strings.Contains(got[0], "\r\nX-Mr-proxy: 1\r\n") {
		t.Fatalf("upstream got %q", got)
	}

	if calls := rw.seen(); len(calls) != 1 || calls[0] != "req:proxy" {
		t.Fatalf("rewriter calls = %q (intercept rules must wait for an edited Forward)", calls)
	}

	recs := h.records(2, complete)
	orig, ed := recs[0], recs[1]
	if ed.EditedFrom == nil || *ed.EditedFrom != orig.ID || ed.Source != "proxy" {
		t.Fatalf("edited = %+v", ed.Request)
	}

	if strings.Contains(h.raw(orig.ID, history.PartRequest), "X-Mr") || !strings.Contains(h.raw(ed.ID, history.PartRequest), "X-Mr-proxy") {
		t.Fatal("pair holds the wrong request bytes")
	}
}

func TestInterceptRulesRunOnlyAfterEditedForward(t *testing.T) {
	rw := &fakeRewriter{active: map[string][2]bool{"intercept": {true, true}}}
	h := newHookRig(t, []intercept.Kind{intercept.Request}, rw, rawhttp.Limits{})

	// Forward unchanged: no intercept rules, no pair.
	_, br := h.send("GET http://%s/1 HTTP/1.1\r\nHost: %s\r\n\r\n")
	call := h.nextHold(intercept.Request)
	call.reply <- intercept.Decision{Raw: call.msg.Request().Raw}
	h.response(br, "GET")

	if calls := rw.seen(); len(calls) != 0 {
		t.Fatalf("rewriter ran after an unchanged Forward: %q", calls)
	}

	// Forward with edits: intercept request rules run on the edited bytes.
	_, br = h.send("GET http://%s/2 HTTP/1.1\r\nHost: %s\r\n\r\n")
	call = h.nextHold(intercept.Request)
	edited := strings.Replace(string(call.msg.Request().Raw), "/2", "/3", 1)
	call.reply <- intercept.Decision{Raw: []byte(edited), Edited: true}
	h.response(br, "GET")

	ups := h.upstreamRequests()
	if len(ups) != 2 || !strings.HasPrefix(ups[1], "GET /3 ") || !strings.Contains(ups[1], "\r\nX-Mr-intercept: 1\r\n") {
		t.Fatalf("upstream got %q", ups)
	}

	if calls := rw.seen(); len(calls) != 1 || calls[0] != "req:intercept" {
		t.Fatalf("rewriter calls = %q", calls)
	}

	recs := h.records(3, complete)
	if ed := recs[2]; ed.EditedFrom == nil || *ed.EditedFrom != recs[1].ID || ed.Source != "intercept" {
		t.Fatalf("edited = %+v", ed.Request)
	}
}

// scripted is an upstream that answers each request with reply(request) and
// records every byte each connection sent. A 101 reply turns the connection
// into an echo.
type scripted struct {
	addr string

	mu    sync.Mutex
	conns []*bytes.Buffer
	done  chan struct{} // one value per finished connection
}

func newScripted(t *testing.T, reply func(req *rawhttp.Request) string) *scripted {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	s := &scripted{addr: ln.Addr().String(), done: make(chan struct{}, 16)}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			in := &bytes.Buffer{}
			s.mu.Lock()
			s.conns = append(s.conns, in)
			s.mu.Unlock()

			go s.serve(c, in, reply)
		}
	}()

	return s
}

func (s *scripted) serve(c net.Conn, in *bytes.Buffer, reply func(req *rawhttp.Request) string) {
	defer func() { _ = c.Close(); s.done <- struct{}{} }()

	_ = c.SetDeadline(time.Now().Add(10 * time.Second))

	var mu sync.Mutex // in is read by the test only after done
	br := bufio.NewReader(io.TeeReader(c, writerFunc(func(p []byte) { mu.Lock(); in.Write(p); mu.Unlock() })))

	for {
		req, err := rawhttp.ReadRequest(br, rawhttp.Limits{})
		if err != nil {
			_, _ = io.Copy(io.Discard, br) // keep recording until the proxy closes

			return
		}

		_, _ = io.Copy(io.Discard, req.Body())

		out := reply(req)
		if _, err := io.WriteString(c, out); err != nil {
			return
		}

		if strings.HasPrefix(out, "HTTP/1.1 101 ") {
			_, _ = io.Copy(c, br)

			return
		}
	}
}

// received waits for n connections to end and returns what each sent.
func (s *scripted) received(t *testing.T, ctx context.Context, n int) []string {
	t.Helper()

	for range n {
		select {
		case <-s.done:
		case <-ctx.Done():
			t.Fatalf("waiting for upstream connections: %v", ctx.Err())
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var out []string
	for _, b := range s.conns {
		out = append(out, b.String())
	}

	return out
}

type writerFunc func(p []byte)

func (f writerFunc) Write(p []byte) (int, error) { f(p); return len(p), nil }

// readAll reads the client connection until the proxy closes it.
func readAll(t *testing.T, br *bufio.Reader) string {
	t.Helper()

	b, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("client connection not closed: %v (read %q)", err, b)
	}

	return string(b)
}

// A Forward whose Content-Length is shorter than its body sends every byte as given.
func TestInterceptEditedLengthSendsBytesAsGiven(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})
	up := newScripted(t, func(*rawhttp.Request) string { return testutil.RawReply })
	h.addr = up.addr

	_, br := h.send("POST http://%s/a HTTP/1.1\r\nHost: %s\r\nContent-Length: 4\r\nConnection: close\r\n\r\nbody")

	edited := "POST http://%s/b HTTP/1.1\r\nHost: x\r\nProxy-Connection: keep-alive\r\nContent-Length: 2\r\nConnection: close\r\n\r\nedited"
	h.nextHold(intercept.Request).reply <- intercept.Decision{Raw: []byte(strings.ReplaceAll(edited, "%s", up.addr)), Edited: true}

	if got := h.response(br, "POST"); got != testutil.RawReply {
		t.Fatalf("client got %q", got)
	}

	want := "POST /b HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\nConnection: close\r\n\r\nedited"
	if got := up.received(t, h.ctx, 1); len(got) != 1 || got[0] != want {
		t.Fatalf("upstream got %q, want [%q]", got, want)
	}
}

// A hook that turns GET into HEAD: the origin answers HEAD, and the GET
// client must not wait for the body the head announces.
func TestMethodEditToHeadClosesClient(t *testing.T) {
	toHead := func(raw []byte) []byte { return bytes.Replace(raw, []byte("GET "), []byte("HEAD "), 1) }
	headReply := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n"

	for name, respActive := range map[string]bool{"streaming": false, "hooked": true} {
		t.Run(name, func(t *testing.T) {
			rw := &fakeRewriter{active: map[string][2]bool{"proxy": {true, respActive}}, edit: toHead}
			h := newHookRig(t, nil, rw, rawhttp.Limits{})
			up := newScripted(t, func(req *rawhttp.Request) string {
				if req.Method() != "HEAD" {
					return "HTTP/1.1 500 Not HEAD\r\nContent-Length: 0\r\n\r\n"
				}

				return headReply
			})
			h.addr = up.addr

			_, br := h.send("GET http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n")

			want := headReply
			if respActive {
				want = string(addHeader([]byte(headReply), "X-Mr-proxy")) // the response rule ran
			}

			if got := readAll(t, br); got != want {
				t.Fatalf("client got %q, want %q", got, want)
			}
		})
	}
}

// Response rules see the request as the origin received it; a response-only
// edit keeps the client's request bytes in both records.
func TestResponseRulesSeeUpstreamRequest(t *testing.T) {
	rw := &fakeRewriter{active: map[string][2]bool{"proxy": {false, true}}}
	h := newHookRig(t, nil, rw, rawhttp.Limits{})

	sent := "GET http://%s/p HTTP/1.1\r\nHost: %s\r\nProxy-Connection: keep-alive\r\n\r\n"
	_, br := h.send(sent)
	h.response(br, "GET")

	wantUp := strings.Replace(strings.ReplaceAll(sent, "%s", h.upAddr()), "http://"+h.upAddr(), "", 1)
	wantUp = strings.Replace(wantUp, "Proxy-Connection: keep-alive\r\n", "", 1)

	rw.mu.Lock()
	reqs := slices.Clone(rw.respReqs)
	rw.mu.Unlock()

	if len(reqs) != 1 || reqs[0] != wantUp {
		t.Fatalf("response rule got request %q, want [%q]", reqs, wantUp)
	}

	recs := h.records(2, complete)
	wantReq := strings.ReplaceAll(sent, "%s", h.upAddr())
	if a, b := h.raw(recs[0].ID, history.PartRequest), h.raw(recs[1].ID, history.PartRequest); a != wantReq || b != wantReq {
		t.Fatalf("request bytes: original %q, edited %q, want both %q", a, b, wantReq)
	}
}

// Pipelined requests wait in order behind a held one and keep their bytes.
func TestInterceptPipelinedRequestsKeepOrder(t *testing.T) {
	h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})

	_, br := h.send("GET http://%s/1 HTTP/1.1\r\nHost: %s\r\n\r\nGET http://%s/2 HTTP/1.1\r\nHost: %s\r\n\r\n")

	for i := range 2 {
		call := h.nextHold(intercept.Request)
		if want := fmt.Sprintf("/%d", i+1); call.msg.Path() != want {
			t.Fatalf("hold %d path = %q, want %q", i, call.msg.Path(), want)
		}

		call.reply <- intercept.Decision{Raw: call.msg.Request().Raw}

		if got := h.response(br, "GET"); got != testutil.RawReply {
			t.Fatalf("response %d = %q", i, got)
		}
	}
}

// A held 101 handshake: the tunnel starts only on a Forward that still switches.
func TestInterceptUpgradeResponse(t *testing.T) {
	handshake := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: x\r\nConnection: Upgrade\r\n\r\n"
	req := "GET http://%s/ws HTTP/1.1\r\nHost: %s\r\nUpgrade: x\r\nConnection: Upgrade\r\n\r\n"

	tests := []struct {
		name   string
		dec    intercept.Decision
		want   string // what the client reads before the tunnel or the close
		tunnel bool
	}{
		{"forward", intercept.Decision{Raw: []byte(handshake)}, handshake, true},
		{"edited", intercept.Decision{Raw: []byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n"), Edited: true},
			"HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n", false},
		{"drop", intercept.Decision{DropStatusCode: 502}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHookRig(t, []intercept.Kind{intercept.Response}, nil, rawhttp.Limits{})
			up := newScripted(t, func(*rawhttp.Request) string { return handshake })
			h.addr = up.addr

			conn, br := h.send(req)

			call := h.nextHold(intercept.Response)
			if call.msg.Response() == nil || call.msg.Response().Code != 101 {
				t.Fatalf("held response = %+v", call.msg.Response())
			}

			call.reply <- tt.dec

			if !tt.tunnel {
				got := readAll(t, br)
				if tt.want != "" && got != tt.want {
					t.Fatalf("client got %q, want %q", got, tt.want)
				}

				if tt.want == "" && !strings.HasPrefix(got, "HTTP/1.1 502 Bad Gateway\r\n") {
					t.Fatalf("client got %q", got)
				}

				up.received(t, h.ctx, 1) // the switched upstream is closed

				return
			}

			if got := h.response(br, "GET"); got != handshake {
				t.Fatalf("client got %q", got)
			}

			if _, err := io.WriteString(conn, "ping\n"); err != nil {
				t.Fatal(err)
			}

			if line, err := br.ReadString('\n'); err != nil || line != "ping\n" {
				t.Fatalf("tunnel echo = %q, %v", line, err)
			}
		})
	}
}

// Drop pages: a HEAD client gets no body; a 1xx status is followed by a 502 page.
func TestInterceptDropPages(t *testing.T) {
	tests := []struct {
		name, method string
		code         int
		check        func(t *testing.T, got string)
	}{
		{"head", "HEAD", 404, func(t *testing.T, got string) {
			if !strings.HasPrefix(got, "HTTP/1.1 404 Not Found\r\n") || !strings.HasSuffix(got, "\r\n\r\n") || strings.Contains(got, "<") {
				t.Fatalf("client got %q", got)
			}
		}},
		{"interim", "GET", 100, func(t *testing.T, got string) {
			if !strings.HasPrefix(got, "HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 502 Bad Gateway\r\n") || !strings.Contains(got, "Ratatoskr") {
				t.Fatalf("client got %q", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHookRig(t, []intercept.Kind{intercept.Request}, nil, rawhttp.Limits{})

			_, br := h.send(tt.method + " http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n")
			h.nextHold(intercept.Request).reply <- intercept.Decision{DropStatusCode: tt.code}

			tt.check(t, readAll(t, br))
		})
	}
}
