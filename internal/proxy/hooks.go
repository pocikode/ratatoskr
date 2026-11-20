package proxy

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/intercept"
	"github.com/pocikode/ratatoskr/internal/netx"
	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/store"
)

// Match & Replace sources the proxy runs (F-MR-1).
const (
	sourceProxy     = "proxy"
	sourceIntercept = "intercept"
)

// Interceptor holds fully buffered messages for manual review (F-INT-1).
// *intercept.Service implements it.
type Interceptor interface {
	// WouldHold reports whether buffering this side may lead to a hold.
	WouldHold(kind intercept.Kind) bool
	HoldRequest(ctx context.Context, db *store.DB, msg query.Message) (intercept.Decision, error)
	HoldResponse(ctx context.Context, db *store.DB, msg query.Message) (intercept.Decision, error)
}

// Rewriter applies Match & Replace rules (F-MR-1).
type Rewriter interface {
	// Active reports whether an enabled rule exists for source and side (decides full buffering).
	Active(source string, response bool) bool
	// Request returns the rewritten request; the same slice when nothing changed.
	Request(ctx context.Context, source string, raw []byte) ([]byte, error)
	// Response returns the rewritten response; req is the request as sent upstream.
	Response(ctx context.Context, source string, req, resp []byte) ([]byte, error)
}

// HoldCapture is the part of capture that holds and edited pairs need.
// *history.Capture implements it.
type HoldCapture interface {
	Capture
	// Paused reports that no record would be created, so nothing can be held.
	Paused() bool
	Written(ctx context.Context, id int64) error
	PendingDB(id int64) *store.DB
	CompleteDropped(ctx context.Context, id int64, resp *history.Response)
	Add(ctx context.Context, e history.Entry)
}

// hookRequest reports whether the request side must be buffered. A hold needs
// a history record, so paused capture leaves only a rule to buffer for; a pause
// that starts later still yields id 0 and an unheld request.
func (s *Server) hookRequest() bool {
	return s.deps.Rewriter != nil && s.deps.Rewriter.Active(sourceProxy, false) ||
		s.icpt != nil && !s.hc.Paused() && s.icpt.WouldHold(intercept.Request)
}

// hookResponse reports whether the response side must be buffered. A request
// cut at the capture cap gets no response rule (F-MR-1), so only a hold can
// buffer its response; a hold needs a history record (id != 0).
func (s *Server) hookResponse(id int64, reqTruncated bool) bool {
	if id != 0 && s.icpt != nil && s.icpt.WouldHold(intercept.Response) {
		return true
	}

	rule := s.deps.Rewriter != nil && s.deps.Rewriter.Active(sourceProxy, true)
	if rule && reqTruncated {
		s.log.Info("proxy: request over the capture cap; response rules skipped", "component", "proxy", "source", sourceProxy)
	}

	return rule && !reqTruncated
}

func (s *Server) captureCap() int64 {
	if s.deps.Limits.MaxBodyBytes == 0 {
		return rawhttp.DefaultLimits.MaxBodyBytes
	}

	return s.deps.Limits.MaxBodyBytes
}

// buffer reads the body of m. It returns the whole message and full=true when
// the body fits the capture cap; otherwise it returns the head plus the body
// bytes consumed so far, which the caller must still send before the rest of
// m.Body(). A read error returns what arrived.
func (s *Server) buffer(m *rawhttp.Message) (data []byte, full bool, err error) {
	limit := s.captureCap()

	switch {
	case m.Framing() == rawhttp.FramingNone:
		return m.Raw(), true, nil
	case limit < 0 || m.ContentLength() > limit:
		s.log.Info("proxy: body over the capture cap passes through unheld and unchanged", "component", "proxy", "cap", limit)

		return m.Head(), false, nil
	}

	body := m.Body()

	if _, err := io.CopyN(io.Discard, body, limit); err != nil {
		if errors.Is(err, io.EOF) {
			return m.Raw(), true, nil
		}

		return m.Raw(), false, err
	}

	var one [1]byte

	switch n, err := io.ReadFull(body, one[:]); {
	case n == 1:
		s.log.Info("proxy: body over the capture cap passes through unheld and unchanged", "component", "proxy", "cap", limit)

		return append(m.Raw(), one[0]), false, nil
	case errors.Is(err, io.EOF):
		return m.Raw(), true, nil
	default:
		return m.Raw(), false, err
	}
}

// writeAll is an io.WriterTo for bytes that are already in memory, followed
// by an optional reader. Writing the bytes again (a retry) is allowed when r
// is nil.
type writeAll struct {
	b []byte
	r io.Reader
}

func (w writeAll) WriteTo(dst io.Writer) (int64, error) {
	n, err := dst.Write(w.b)
	if err != nil || w.r == nil {
		return int64(n), err
	}

	m, err := io.Copy(dst, w.r)

	return int64(n) + m, err
}

// parseRequest re-reads a whole request held in memory.
func (s *Server) parseRequest(raw []byte) (*rawhttp.Request, error) {
	return rawhttp.ReadRequest(bufio.NewReader(bytes.NewReader(raw)), rawhttp.Limits{MaxHeadBytes: s.maxHead(), MaxBodyBytes: -1})
}

// parseResponse re-reads a response held in memory. whole reports whether raw
// is exactly one complete message: only then can the client connection be
// reused after it.
func (s *Server) parseResponse(raw []byte, method string) (r *rawhttp.Response, whole bool, err error) {
	rd := bytes.NewReader(raw)
	br := bufio.NewReader(rd)

	r, err = rawhttp.ReadResponse(br, method, rawhttp.Limits{MaxHeadBytes: s.maxHead(), MaxBodyBytes: -1})
	if err != nil {
		return nil, false, err
	}

	_, err = io.Copy(io.Discard, r.Body())

	return r, err == nil && rd.Len()+br.Buffered() == 0, nil
}

// upstreamBytes returns raw (a whole request, or a head alone) as it goes to
// the origin server, and its parse: outside a tunnel an absolute-form target
// becomes origin-form. Only the head is rebuilt; the bytes after it go out as
// given, whatever length the head declares.
func (s *Server) upstreamBytes(raw []byte, inTunnel bool) ([]byte, *rawhttp.Request, error) {
	rd := bytes.NewReader(raw)
	br := bufio.NewReader(rd)

	r, err := rawhttp.ReadRequest(br, rawhttp.Limits{MaxHeadBytes: s.maxHead(), MaxBodyBytes: -1})
	if err != nil {
		return nil, nil, err
	}

	_, rest, ok := cutScheme(r.Target())
	if inTunnel || !ok {
		return raw, r, nil
	}

	_, origin := splitAuthority(rest)
	suffix := raw[len(raw)-rd.Len()-br.Buffered():]

	h, err := s.parseRequest(r.Head()) // a head alone: the builder copies no body
	if err != nil {
		return nil, nil, err
	}

	b := append(h.Builder().SetTarget(origin).DelHeader("Proxy-Connection").Bytes(), suffix...)

	u, err := s.parseRequest(b)
	if err != nil {
		return nil, nil, err
	}

	return b, u, nil
}

// watchClient calls cancel when the client closes its connection while a
// message is held, so the item leaves the queue (F-INT-1). stop must be
// called before the connection is read again.
func (c *session) watchClient(cancel context.CancelFunc) (stop func()) {
	idle := c.idle.Idle
	c.idle.Idle = 0 // a long hold is not an idle client
	_ = c.conn.SetReadDeadline(time.Time{})

	var stopping atomic.Bool

	done := make(chan struct{})

	// over holds pipelined bytes beyond a full c.br, in order; only the
	// watcher touches it until done is closed.
	var over []byte

	gone := func() {
		if !stopping.Load() {
			cancel()
		}
	}

	go func() {
		defer close(done)

		// Pipelined bytes stay in c.br for the next read; only a read error means
		// the client is gone.
		for n := c.br.Buffered() + 1; n <= c.br.Size(); n = c.br.Buffered() + 1 {
			if _, err := c.br.Peek(n); err != nil {
				gone()

				return
			}
		}

		// c.br is full: keep reading behind it, bounded by the head limit. A
		// client that pipelines more during a hold is treated as gone.
		buf := make([]byte, c.br.Size())
		for {
			n, err := c.br.Read(buf)
			over = append(over, buf[:n]...)

			if err != nil || len(over) > c.s.maxHead() {
				gone()

				return
			}
		}
	}()

	return func() {
		stopping.Store(true)
		_ = c.conn.SetReadDeadline(time.Now()) // unblocks the read; a deadline error is not sticky
		<-done
		_ = c.conn.SetReadDeadline(time.Time{})
		c.idle.Idle = idle

		if len(over) > 0 { // put the pipelined bytes back in front, in order
			rest, _ := c.br.Peek(c.br.Buffered())
			c.src = io.MultiReader(bytes.NewReader(append(over, rest...)), c.src)
			c.br.Reset(c.src)
		}
	}
}

// errHoldEnded means the client went away (or the proxy stopped) while its
// message waited in Intercept.
var errHoldEnded = errors.New("proxy: hold ended")

// hold waits until the record is readable, then holds msg, whose held side is
// raw. A record that cannot be confirmed passes raw through unheld.
func (c *session) hold(ctx context.Context, id int64, kind intercept.Kind, msg query.Message, raw []byte) (intercept.Decision, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stop := c.watchClient(cancel)
	defer stop()

	if err := c.s.hc.Written(ctx, id); err != nil {
		if ctx.Err() != nil {
			return intercept.Decision{}, fmt.Errorf("%w: %w", errHoldEnded, err)
		}

		c.s.log.Info("proxy: record not readable; not holding", "component", "proxy", "request_id", id, "error", err)

		return intercept.Decision{Raw: raw}, nil
	}

	var dec intercept.Decision

	var err error
	if kind == intercept.Request {
		dec, err = c.s.icpt.HoldRequest(ctx, c.s.hc.PendingDB(id), msg)
	} else {
		dec, err = c.s.icpt.HoldResponse(ctx, c.s.hc.PendingDB(id), msg)
	}

	if err != nil {
		return intercept.Decision{}, fmt.Errorf("%w: %w", errHoldEnded, err)
	}

	return dec, nil
}

// rewrite runs the Match & Replace rules of source on raw. A failing rule
// set leaves the message as it was.
func (c *session) rewrite(source string, response bool, req func() []byte, raw []byte) []byte {
	rw := c.s.deps.Rewriter
	if rw == nil || !rw.Active(source, response) {
		return raw
	}

	var (
		out []byte
		err error
	)

	if response {
		out, err = rw.Response(c.ctx, source, req(), raw)
	} else {
		out, err = rw.Request(c.ctx, source, raw)
	}

	if err != nil {
		c.s.log.Warn("proxy: match & replace failed; message unchanged", "component", "proxy", "source", source, "error", err)

		return raw
	}

	return out
}

// dropPage is Ratatoskr's answer to a dropped message; method is the
// client's. An interim (1xx) status is no final answer, so a 502 page follows
// it (lead, 2026-11-19, T-105 review). A HEAD client gets no body.
func dropPage(code int, method string) []byte {
	var out []byte

	if code < 200 {
		out = []byte("HTTP/1.1 " + statusText(code) + "\r\n\r\n")
		code = http.StatusBadGateway
	}

	status := statusText(code)
	body := ""

	if code != http.StatusNoContent && code != http.StatusNotModified {
		body = "<!doctype html><title>" + status + "</title><h1>" + status +
			"</h1><p>Ratatoskr dropped this message in Intercept.</p>\n"
	}

	out = append(out, "HTTP/1.1 "+status+"\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: "+
		strconv.Itoa(len(body))+"\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n"...)

	if isHead(method) { // the fields a GET would get, without the content (RFC 9110 §9.3.2)
		return out
	}

	return append(out, body...)
}

func statusText(code int) string {
	return strings.TrimSpace(strconv.Itoa(code) + " " + http.StatusText(code))
}

// heldMessage is the query view of an exchange in flight (docs/04 §5).
type heldMessage struct {
	id      int64
	entry   history.Entry
	method  string
	target  string
	req     query.Side
	resp    *query.Response
	created time.Time
}

func (m *heldMessage) ID() int64      { return m.id }
func (m *heldMessage) Source() string { return cmp.Or(m.entry.Source, sourceProxy) }
func (m *heldMessage) Host() string   { return m.entry.Host }
func (m *heldMessage) Port() int      { return m.entry.Port }
func (m *heldMessage) TLS() bool      { return m.entry.TLS }
func (m *heldMessage) Method() string { return m.method }

func (m *heldMessage) Path() string {
	p, _ := splitTarget(m.target)

	return p
}

func (m *heldMessage) Query() string {
	_, q := splitTarget(m.target)

	return q
}

func (m *heldMessage) Ext() string               { return path.Ext(m.Path()) }
func (m *heldMessage) CreatedAt() time.Time      { return m.created }
func (m *heldMessage) Color() string             { return "" }
func (m *heldMessage) Note() string              { return "" }
func (m *heldMessage) Edited() bool              { return false }
func (m *heldMessage) Request() query.Side       { return m.req }
func (m *heldMessage) Response() *query.Response { return m.resp }

// splitTarget derives path and query the way history stores them.
func splitTarget(target string) (p, q string) {
	target, _, _ = strings.Cut(target, "#")
	p, q, _ = strings.Cut(target, "?")

	if _, rest, ok := strings.Cut(p, "://"); ok {
		_, p2, _ := strings.Cut(rest, "/")
		p = "/" + p2
	}

	return p, q
}

// querySide is the query view of raw, whose head m parsed.
func querySide(raw []byte, m *rawhttp.Message) query.Side {
	s := query.Side{Raw: raw, BodyOffset: len(m.Head())}

	for _, h := range m.Headers() {
		s.Headers = append(s.Headers, query.Header{Name: h.Name, Value: h.Value})
	}

	return s
}

// historySide is the stored form of raw, whose head m parsed.
func historySide(raw []byte, m *rawhttp.Message) history.Side {
	s := history.Side{Raw: raw, BodyOffset: len(m.Head())}

	for _, h := range m.Headers() {
		s.Headers = append(s.Headers, history.Field{Name: h.Name, Value: h.Value})
	}

	return s
}

// edits is what the hooks did to one exchange.
type edits struct {
	// sent is the request as the origin server sees it: the client's request
	// unless a hook buffered it.
	sent *rawhttp.Request
	// upRaw holds sent's bytes once known.
	upRaw []byte
	// changed is set when a hook changed the request bytes.
	changed bool
	// intercepted is set when a Forward in Intercept changed bytes.
	intercepted bool
	inTunnel    bool
}

// headChanged reports whether a hook made the request HEAD or took HEAD away:
// the response then has a body the client does not expect, or lacks one it
// waits for, so the client connection must close after it.
func (x *edits) headChanged(client *rawhttp.Request) bool {
	return isHead(x.sent.Method()) != isHead(client.Method())
}

// isHead matches HEAD as rawhttp frames responses: case-insensitively.
func isHead(method string) bool {
	return strings.EqualFold(method, "HEAD")
}

// upstreamRaw returns the request bytes as the origin server received them.
func (c *session) upstreamRaw(x *edits, client *rawhttp.Request) []byte {
	if x.upRaw == nil {
		x.upRaw = client.Raw()
		if b, _, err := c.s.upstreamBytes(client.Raw(), x.inTunnel); err == nil {
			x.upRaw = b
		}
	}

	return x.upRaw
}

// requestHooks runs M&R and Intercept on a fully buffered request raw and
// returns what to write upstream. dropped means the client was answered.
func (c *session) requestHooks(x *edits, id int64, entry *history.Entry, raw []byte) (out io.WriterTo, dropped bool, err error) {
	sent := c.rewrite(sourceProxy, false, nil, raw)

	if id != 0 && c.s.icpt != nil && c.s.icpt.WouldHold(intercept.Request) {
		r, err := c.s.parseRequest(sent)
		if err != nil {
			return nil, false, err
		}

		msg := &heldMessage{
			id: id, entry: *entry, method: r.Method(), target: r.Target(),
			req: querySide(sent, &r.Message), created: entry.CreatedAt,
		}

		dec, err := c.hold(c.ctx, id, intercept.Request, msg, sent)
		if err != nil {
			return nil, false, err
		}

		if dec.DropStatusCode != 0 {
			_, _ = c.conn.Write(dropPage(dec.DropStatusCode, entry.Method))
			c.s.hc.CompleteDropped(c.ctx, id, nil)

			return nil, true, nil
		}

		if dec.Edited && !bytes.Equal(dec.Raw, sent) {
			sent, x.intercepted = c.rewrite(sourceIntercept, false, nil, dec.Raw), true
		}
	}

	// Edits may change the framing: the bytes are re-read before they go out.
	b, r, err := c.s.upstreamBytes(sent, x.inTunnel)
	if err != nil {
		return nil, false, err
	}

	x.sent, x.upRaw, x.changed = r, b, !bytes.Equal(sent, raw)

	return writeAll{b: b}, false, nil
}

// respondHooked delivers a response that M&R or Intercept may change and
// reports whether the client connection stays usable. A 101 handshake enters
// the tunnel only when the delivered response still switches protocols.
func (c *session) respondHooked(x *edits, id int64, entry *history.Entry, req *rawhttp.Request,
	resp *rawhttp.Response, up *netx.Conn, answer *history.Response,
) bool {
	data, full, err := c.s.buffer(&resp.Message)
	if err != nil || !full { // never for 101, which has no body
		var rest io.Reader
		if err == nil {
			rest = resp.Body()
		}

		_, copyErr := writeAll{b: data, r: rest}.WriteTo(c.conn)
		owner := c.editOwner(id, x.changed)
		c.finish(id, answer, resp)
		c.storeEdited(owner, x, id, entry, answer)

		return c.release(cmp.Or(err, copyErr), req, x.sent, resp, up) && !x.headChanged(req)
	}

	upgrade := resp.Status() == 101
	if upgrade {
		defer func() {
			if up != nil {
				_ = up.Close() // the upstream switched protocols: it is not reusable
			}
		}()
	} else {
		// The response is in memory: the upstream is free before any hold.
		_ = c.release(nil, x.sent, x.sent, resp, up)
	}

	// Response rules get the upstream request, built only when a rule runs.
	// A request cut at the capture cap is no rule input: its exchange gets no
	// response rule (F-MR-1; operator, 2026-11-19, T-105 review).
	rewrite := func(source string, raw []byte) []byte {
		if req.Truncated() {
			if rw := c.s.deps.Rewriter; rw != nil && rw.Active(source, true) {
				c.s.log.Info("proxy: request over the capture cap; response rules skipped", "component", "proxy", "source", source)
			}

			return raw
		}

		return c.rewrite(source, true, func() []byte { return c.upstreamRaw(x, req) }, raw)
	}

	delivered := rewrite(sourceProxy, data)

	if id != 0 && c.s.icpt != nil && c.s.icpt.WouldHold(intercept.Response) {
		if r, _, err := c.s.parseResponse(delivered, x.sent.Method()); err != nil {
			c.s.log.Info("proxy: rewritten response does not parse; not holding", "component", "proxy", "error", err)
		} else {
			heldReq, heldHead := entry.Req.Raw, req // the pending record's copy: no second one per hold
			if x.changed {
				heldReq, heldHead = x.upRaw, x.sent
			}

			msg := &heldMessage{
				id: id, entry: *entry, method: heldHead.Method(), target: heldHead.Target(),
				req:     querySide(heldReq, &heldHead.Message),
				resp:    &query.Response{Side: querySide(delivered, &r.Message), Code: r.Status(), RoundTrip: answer.RoundTrip},
				created: entry.CreatedAt,
			}

			dec, err := c.hold(c.ctx, id, intercept.Response, msg, delivered)

			switch {
			case err != nil:
				c.finish(id, answer, resp)

				return false
			case dec.DropStatusCode != 0:
				_, _ = c.conn.Write(dropPage(dec.DropStatusCode, req.Method()))
				answer.Side, answer.Truncated = sideOf(&resp.Message), resp.Truncated()
				c.s.hc.CompleteDropped(c.ctx, id, answer)

				return false
			case dec.Edited && !bytes.Equal(dec.Raw, delivered):
				delivered, x.intercepted = rewrite(sourceIntercept, dec.Raw), true
			}
		}
	}

	changed := !bytes.Equal(delivered, data)
	owner := c.editOwner(id, x.changed || changed)
	c.finish(id, answer, resp)

	final := &resp.Message
	edited := answer
	whole := true

	if changed {
		var r *rawhttp.Response
		if r, whole, err = c.s.parseResponse(delivered, x.sent.Method()); err == nil {
			final = &r.Message
			edited = &history.Response{
				Side: historySide(delivered, &r.Message), Code: r.Status(),
				RoundTrip: answer.RoundTrip, CreatedAt: answer.CreatedAt,
			}
		} else {
			// Unparsable bytes still reach the client; store them as they went.
			final = nil
			edited = &history.Response{Raw: delivered, BodyOffset: len(delivered), Code: answer.Code,
				RoundTrip: answer.RoundTrip, CreatedAt: answer.CreatedAt}
		}
	}

	c.storeEdited(owner, x, id, entry, edited)

	if upgrade && final != nil && edited.Code == 101 {
		u := up
		up = nil // the relay owns it now

		return c.upgrade(delivered, u)
	}

	_, copyErr := c.conn.Write(delivered)

	// An edit whose bytes do not end where its framing says (a short or
	// overlong body), or a lone informational head, leaves the client unable
	// to find the next (or its final) response.
	return copyErr == nil && !upgrade && final != nil && whole && edited.Code >= 200 && !req.Close() && !final.Close() &&
		final.Framing() != rawhttp.FramingClose && !x.headChanged(req)
}

// release returns up to the pool when it is reusable after the exchange and
// reports whether the client connection stays usable.
func (c *session) release(err error, client, sent *rawhttp.Request, resp *rawhttp.Response, up *netx.Conn) bool {
	if err != nil || client.Close() || sent.Close() || resp.Close() || resp.Framing() == rawhttp.FramingClose {
		_ = up.Close()

		return false
	}

	c.s.pool.Put(up)

	return true
}

// editOwner returns the project that owns record id when an edited record
// must be stored for it. Call it before the record is completed.
func (c *session) editOwner(id int64, edited bool) *store.DB {
	if !edited || id == 0 || c.s.hc == nil {
		return nil
	}

	return c.s.hc.PendingDB(id)
}

// storeEdited stores the edited record of a pair (F-HIST-4): the request as
// the server received it and resp as the client received it.
func (c *session) storeEdited(owner *store.DB, x *edits, id int64, entry *history.Entry, resp *history.Response) {
	if owner == nil {
		return
	}

	e := *entry
	e.ProjectDB, e.EditedFrom, e.Resp = owner, new(id), resp

	if x.intercepted {
		e.Source = sourceIntercept
	}

	if x.changed { // the request as the origin server received it
		e.Method, e.Target = x.sent.Method(), x.sent.Target()
		e.Req, e.Truncated = historySide(x.upRaw, &x.sent.Message), false
	}

	c.s.hc.Add(c.ctx, e)
}
