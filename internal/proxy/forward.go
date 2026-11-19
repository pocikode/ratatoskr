package proxy

import (
	"context"
	"errors"
	"html"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/netx"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// forward sends req to dst, relays the answer and records the exchange. It
// reports whether the client connection can serve another request. inTunnel
// means the bytes are forwarded exactly as received; outside a tunnel an
// absolute-form target is rewritten to origin-form for the origin server.
func (c *session) forward(req *rawhttp.Request, dst destination, inTunnel bool) bool {
	entry := history.Entry{
		Host: dst.host, Port: dst.port, TLS: dst.tls,
		Method: req.Method(), Target: req.Target(), CreatedAt: time.Now(),
	}

	var id int64
	attempted := false
	pending := func() {
		if attempted || c.s.deps.Capture == nil {
			return
		}
		attempted = true
		entry.Req = sideOf(&req.Message)
		entry.Truncated = req.Truncated()
		id = c.s.deps.Capture.NewPending(c.ctx, entry)
	}
	if req.Framing() == rawhttp.FramingNone {
		pending()
	}

	var out io.WriterTo = req

	if !inTunnel {
		if _, rest, ok := cutScheme(req.Target()); ok {
			_, origin := splitAuthority(rest)
			out = req.Builder().SetTarget(origin).DelHeader("Proxy-Connection")
		}
	}

	if v, ok := req.Header("Expect"); ok && strings.EqualFold(strings.TrimSpace(v), "100-continue") {
		// The client holds its body back until we answer; the upstream's own
		// 100 is swallowed below.
		if _, err := io.WriteString(c.conn, "HTTP/1.1 100 Continue\r\n\r\n"); err != nil {
			if id != 0 {
				c.s.deps.Capture.Complete(c.ctx, id, nil)
			}
			return false
		}

		c.continued = true
		defer func() { c.continued = false }()
	}

	x := &edits{sent: req, inTunnel: inTunnel}

	if c.s.hookRequest() {
		data, full, err := c.s.buffer(&req.Message)

		switch {
		case err != nil:
			pending()

			return c.fail(id, dst, err)
		case !full:
			// Over the cap: sent unheld and unchanged, the consumed bytes first.
			head, _, err := c.s.upstreamBytes(req.Head(), inTunnel)
			if err != nil {
				pending()

				return c.fail(id, dst, err)
			}

			out = writeAll{b: append(head, data[len(req.Head()):]...), r: req.Body()}
		default:
			pending()

			var dropped bool
			if out, dropped, err = c.requestHooks(x, id, &entry, data); dropped {
				return false
			}

			if errors.Is(err, errHoldEnded) {
				c.s.hc.Complete(c.ctx, id, nil) // the record stays without a response (F-INT-1)

				return false
			}

			if err != nil {
				return c.fail(id, dst, err)
			}
		}
	}

	resp, up, err := c.roundTrip(x.sent, out, dst, pending)
	if err != nil {
		pending()
		return c.fail(id, dst, err)
	}

	answer := &history.Response{
		Code: resp.Status(), RoundTrip: time.Since(entry.CreatedAt), CreatedAt: time.Now(),
	}

	if c.s.hookResponse(id, req.Truncated()) {
		return c.respondHooked(x, id, &entry, req, resp, up, answer)
	}

	owner := c.editOwner(id, x.changed)

	if resp.Status() == 101 {
		c.finish(id, answer, resp)
		c.storeEdited(owner, x, id, &entry, answer)
		return c.upgrade(resp.Head(), up)
	}

	_, copyErr := resp.WriteTo(c.conn)

	c.finish(id, answer, resp)
	c.storeEdited(owner, x, id, &entry, answer)

	// An incomplete body closes up either way. The origin answered the method
	// it got (exchange reads with x.sent); a HEAD edit leaves the client's
	// framing wrong, so its connection ends here.
	return c.release(copyErr, req, x.sent, resp, up) && !x.headChanged(req)
}

// idempotent reports the RFC 9110 safe methods: a repeat is harmless.
func idempotent(method string) bool {
	switch method {
	case "GET", "HEAD", "OPTIONS", "TRACE":
		return true
	}

	return false
}

// roundTrip writes the request and reads the final response head. A pooled
// connection the server closed meanwhile is retried once on a fresh one, but
// only for safe methods without a body, which can be sent twice.
func (c *session) roundTrip(req *rawhttp.Request, out io.WriterTo, dst destination, pending func()) (*rawhttp.Response, *netx.Conn, error) {
	up, reused, err := c.s.pool.Get(c.ctx, dst.key())
	if err != nil {
		return nil, nil, err
	}

	resp, err := c.exchange(req, out, up, pending)
	if err != nil && reused && req.Framing() == rawhttp.FramingNone && idempotent(req.Method()) {
		_ = up.Close()

		if up, err = c.s.pool.Fresh(c.ctx, dst.key()); err != nil {
			return nil, nil, err
		}

		resp, err = c.exchange(req, out, up, pending)
	}

	if err != nil {
		_ = up.Close()

		return nil, nil, err
	}

	return resp, up, nil
}

func (c *session) exchange(req *rawhttp.Request, out io.WriterTo, up *netx.Conn, pending func()) (*rawhttp.Response, error) {
	// Shutdown must unblock reads and writes on the upstream too.
	stop := context.AfterFunc(c.ctx, func() { _ = up.Close() })
	defer stop()

	// Per-write idle deadline: slow uploads may outlast HeaderTimeout, stalled ones may not.
	_, err := out.WriteTo(&netx.IdleConn{Conn: up, Idle: c.s.deps.IdleTimeout})
	_ = up.SetWriteDeadline(time.Time{}) // tunnels and pooled reuse must not inherit it

	if err != nil {
		return nil, err
	}

	pending()

	_ = up.SetReadDeadline(time.Now().Add(c.s.deps.HeaderTimeout))

	for {
		resp, err := rawhttp.ReadResponse(up.BR, req.Method(), c.s.deps.Limits)
		if err != nil {
			return nil, err
		}

		if resp.Status()/100 != 1 || resp.Status() == 101 {
			up.SetIdle(c.s.deps.IdleTimeout) // the body is read next: a stalled upstream must not hold us

			return resp, nil
		}

		if resp.Status() == 100 && c.continued {
			continue
		}

		if _, err := resp.WriteTo(c.conn); err != nil { // other interim heads pass through
			return nil, err
		}
	}
}

// upgrade relays a 101 answer: head goes to the client, then both sides
// are copied raw until either closes. WebSocket frames are recorded by a
// later task; the handshake is stored like any request.
func (c *session) upgrade(head []byte, up *netx.Conn) bool {
	defer up.Close()

	up.SetIdle(0)

	if _, err := c.conn.Write(head); err != nil {
		return false
	}

	c.idle.Idle = 0
	_ = c.conn.SetDeadline(time.Time{})

	stop := context.AfterFunc(c.ctx, func() { _ = up.Close() })
	defer stop()

	done := make(chan struct{}, 2)

	go func() { _, _ = io.Copy(up, c.br); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c.conn, up.BR); done <- struct{}{} }()

	<-done // first side to finish ends the relay
	_ = up.Close()
	_ = c.conn.Close()
	<-done

	return false
}

// fail answers an upstream error with a 502 page from Ratatoskr and stores the
// exchange with that page as its response, so the error text is in history.
func (c *session) fail(id int64, dst destination, err error) bool {
	c.s.log.Debug("proxy: upstream error", "component", "proxy", "host", dst.host, "port", dst.port, "error", err)

	text := "Ratatoskr could not complete the request to " + dst.key().Addr() + ": " + err.Error()
	body := "<!doctype html><title>502 Bad Gateway</title><h1>502 Bad Gateway</h1><p>" + html.EscapeString(text) + "</p>\n"
	head := "HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n"

	_, _ = io.WriteString(c.conn, head+body)

	if id == 0 {
		return false
	}

	c.s.deps.Capture.Complete(c.ctx, id, &history.Response{
		Raw: []byte(head + body), BodyOffset: len(head),
		Headers: []history.Field{
			{Name: "Content-Type", Value: "text/html; charset=utf-8"},
			{Name: "Content-Length", Value: strconv.Itoa(len(body))},
			{Name: "Cache-Control", Value: "no-store"},
			{Name: "Connection", Value: "close"},
		},
		Code: 502, CreatedAt: time.Now(),
	})

	return false
}

// finish stores a completed exchange.
func (c *session) finish(id int64, answer *history.Response, resp *rawhttp.Response) {
	if id != 0 {
		answer.Side = sideOf(&resp.Message)
		answer.Truncated = resp.Truncated()
		c.s.deps.Capture.Complete(c.ctx, id, answer)
	}
}

func sideOf(m *rawhttp.Message) history.Side {
	s := history.Side{Raw: m.Raw(), BodyOffset: len(m.Head())}

	for _, h := range m.Headers() {
		s.Headers = append(s.Headers, history.Field{Name: h.Name, Value: h.Value})
	}

	return s
}
