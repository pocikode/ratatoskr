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
	Written(ctx context.Context, id int64) error
	PendingDB(id int64) *store.DB
	CompleteDropped(ctx context.Context, id int64, resp *history.Response)
	Add(ctx context.Context, e history.Entry)
}

// hookRequest reports whether the request side must be buffered.
func (s *Server) hookRequest() bool {
	return s.deps.Rewriter != nil && s.deps.Rewriter.Active(sourceProxy, false) ||
		s.icpt != nil && s.icpt.WouldHold(intercept.Request)
}

// hookResponse reports whether the response side must be buffered.
func (s *Server) hookResponse() bool {
	return s.deps.Rewriter != nil && s.deps.Rewriter.Active(sourceProxy, true) ||
		s.icpt != nil && s.icpt.WouldHold(intercept.Response)
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

// parseResponse re-reads a whole response held in memory.
func (s *Server) parseResponse(raw []byte, method string) (*rawhttp.Response, error) {
	return rawhttp.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), method, rawhttp.Limits{MaxHeadBytes: s.maxHead(), MaxBodyBytes: -1})
}

// upstreamBytes returns raw (a whole request, or a head alone) as it goes to
// the origin server: outside a tunnel an absolute-form target becomes
// origin-form. A head alone yields the rewritten head, as no body follows it.
func (s *Server) upstreamBytes(raw []byte, inTunnel bool) ([]byte, *rawhttp.Request, error) {
	r, err := s.parseRequest(raw)
	if err != nil {
		return nil, nil, err
	}

	if !inTunnel {
		if _, rest, ok := cutScheme(r.Target()); ok {
			_, origin := splitAuthority(rest)
			b := r.Builder().SetTarget(origin).DelHeader("Proxy-Connection").Bytes()

			return b, r, nil
		}
	}

	return raw, r, nil
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

	go func() {
		defer close(done)

		// Pipelined bytes end the watch without a signal; only a read error means the client is gone.
		if _, err := c.br.Peek(1); err != nil && !stopping.Load() {
			cancel()
		}
	}()

	return func() {
		stopping.Store(true)
		_ = c.conn.SetReadDeadline(time.Now()) // unblocks the Peek; a deadline error is not sticky
		<-done
		_ = c.conn.SetReadDeadline(time.Time{})
		c.idle.Idle = idle
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
func (c *session) rewrite(source string, response bool, req, raw []byte) []byte {
	rw := c.s.deps.Rewriter
	if rw == nil || !rw.Active(source, response) {
		return raw
	}

	var (
		out []byte
		err error
	)

	if response {
		out, err = rw.Response(c.ctx, source, req, raw)
	} else {
		out, err = rw.Request(c.ctx, source, raw)
	}

	if err != nil {
		c.s.log.Warn("proxy: match & replace failed; message unchanged", "component", "proxy", "source", source, "error", err)

		return raw
	}

	return out
}

// dropPage is Ratatoskr's answer to a dropped message.
func dropPage(code int) []byte {
	status := strconv.Itoa(code) + " " + http.StatusText(code)
	body := ""

	if code >= 200 && code != http.StatusNoContent && code != http.StatusNotModified {
		body = "<!doctype html><title>" + status + "</title><h1>" + status +
			"</h1><p>Ratatoskr dropped this message in Intercept.</p>\n"
	}

	return []byte("HTTP/1.1 " + strings.TrimSpace(status) + "\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n" + body)
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
	// sent is the request as written upstream (before the origin-form
	// rewrite): the client's request unless a hook changed it.
	sent *rawhttp.Request
	// sentRaw holds sent's bytes when they differ from the client's.
	sentRaw []byte
	// intercepted is set when a Forward in Intercept changed bytes.
	intercepted bool
}

// requestHooks runs M&R and Intercept on a fully buffered request raw and
// returns what to write upstream. dropped means the client was answered.
func (c *session) requestHooks(x *edits, id int64, entry *history.Entry, raw []byte, inTunnel bool) (out io.WriterTo, dropped bool, err error) {
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
			_, _ = c.conn.Write(dropPage(dec.DropStatusCode))
			c.s.hc.CompleteDropped(c.ctx, id, nil)

			return nil, true, nil
		}

		if dec.Edited && !bytes.Equal(dec.Raw, sent) {
			sent, x.intercepted = c.rewrite(sourceIntercept, false, nil, dec.Raw), true
		}
	}

	// Edits may change the framing: the bytes are re-read before they go out.
	b, r, err := c.s.upstreamBytes(sent, inTunnel)
	if err != nil {
		return nil, false, err
	}

	x.sent = r
	if !bytes.Equal(sent, raw) {
		x.sentRaw = sent
	}

	return writeAll{b: b}, false, nil
}

// respondHooked delivers a response that M&R or Intercept may change and
// reports whether the client connection stays usable.
func (c *session) respondHooked(x *edits, id int64, entry *history.Entry, req *rawhttp.Request,
	resp *rawhttp.Response, up *netx.Conn, answer *history.Response,
) bool {
	data, full, err := c.s.buffer(&resp.Message)
	if err != nil || !full {
		var rest io.Reader
		if err == nil {
			rest = resp.Body()
		}

		_, copyErr := writeAll{b: data, r: rest}.WriteTo(c.conn)
		owner := c.editOwner(id, x.sentRaw != nil)
		c.finish(id, answer, resp)
		c.storeEdited(owner, x, id, entry, answer)

		return c.release(cmp.Or(err, copyErr), req, x.sent, resp, up)
	}

	// The response is in memory: the upstream is free before any hold.
	_ = c.release(nil, x.sent, x.sent, resp, up)

	reqRaw := x.sentRaw
	if reqRaw == nil {
		reqRaw = req.Raw()
	}

	delivered := c.rewrite(sourceProxy, true, reqRaw, data)

	if id != 0 && c.s.icpt != nil && c.s.icpt.WouldHold(intercept.Response) {
		if r, err := c.s.parseResponse(delivered, x.sent.Method()); err != nil {
			c.s.log.Info("proxy: rewritten response does not parse; not holding", "component", "proxy", "error", err)
		} else {
			msg := &heldMessage{
				id: id, entry: *entry, method: x.sent.Method(), target: x.sent.Target(),
				req:     querySide(reqRaw, &x.sent.Message),
				resp:    &query.Response{Side: querySide(delivered, &r.Message), Code: r.Status(), RoundTrip: answer.RoundTrip},
				created: entry.CreatedAt,
			}

			dec, err := c.hold(c.ctx, id, intercept.Response, msg, delivered)

			switch {
			case err != nil:
				c.finish(id, answer, resp)

				return false
			case dec.DropStatusCode != 0:
				_, _ = c.conn.Write(dropPage(dec.DropStatusCode))
				answer.Side, answer.Truncated = sideOf(&resp.Message), resp.Truncated()
				c.s.hc.CompleteDropped(c.ctx, id, answer)

				return false
			case dec.Edited && !bytes.Equal(dec.Raw, delivered):
				delivered, x.intercepted = c.rewrite(sourceIntercept, true, reqRaw, dec.Raw), true
			}
		}
	}

	_, copyErr := c.conn.Write(delivered)

	changed := !bytes.Equal(delivered, data)
	owner := c.editOwner(id, x.sentRaw != nil || changed)
	c.finish(id, answer, resp)

	final := &resp.Message
	edited := answer

	if changed {
		if r, err := c.s.parseResponse(delivered, x.sent.Method()); err == nil {
			final = &r.Message
			edited = &history.Response{
				Side: historySide(delivered, &r.Message), Code: r.Status(),
				RoundTrip: answer.RoundTrip, CreatedAt: answer.CreatedAt,
			}
		} else {
			// Unparsable bytes still reached the client; store them as they went.
			final = nil
			edited = &history.Response{Raw: delivered, BodyOffset: len(delivered), Code: answer.Code,
				RoundTrip: answer.RoundTrip, CreatedAt: answer.CreatedAt}
		}
	}

	c.storeEdited(owner, x, id, entry, edited)

	return copyErr == nil && final != nil && !req.Close() && !final.Close() && final.Framing() != rawhttp.FramingClose
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

	if x.sentRaw != nil {
		e.Method, e.Target = x.sent.Method(), x.sent.Target()
		e.Req, e.Truncated = historySide(x.sentRaw, &x.sent.Message), false
	}

	c.s.hc.Add(c.ctx, e)
}
