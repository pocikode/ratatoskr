package sender

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// MaxRedirects is how many redirects Send follows before it returns the next
// 3xx as the final response.
const MaxRedirects = 10

// Target is where a request goes: the connection, not the Host header.
type Target struct {
	Host string
	Port int
	TLS  bool
	// SNI overrides the TLS server name. Empty means Host when it is a name;
	// an IP host sends no SNI.
	SNI string
}

// Options tune a send.
type Options struct {
	// Timeout bounds the whole send, dial to last response byte, redirects
	// included. Zero means no timeout.
	Timeout time.Duration
	// FollowRedirects follows up to MaxRedirects redirects.
	FollowRedirects bool
}

// Request is one send: the exact bytes and where they go.
type Request struct {
	Target
	Raw     []byte
	Options Options
}

// Hop is one exchange on its own connection. Response is nil when the
// exchange failed; otherwise its body has been read and Raw is complete, or
// cut at the capture cap when Truncated reports so.
type Hop struct {
	Target   Target
	Raw      []byte
	Response *rawhttp.Response
	Elapsed  time.Duration
}

// Result holds every hop in order; the last one is the final exchange.
type Result struct {
	Hops []Hop
}

// Kind classifies a send failure.
type Kind string

// Failure kinds.
const (
	KindDial      Kind = "dial"
	KindTLS       Kind = "tls"
	KindTimeout   Kind = "timeout"
	KindCancelled Kind = "cancelled"
	KindProtocol  Kind = "protocol"
)

// Error is a failed exchange.
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string { return "sender: " + string(e.Kind) + ": " + e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// dial opens the TCP connection; tests swap it for an in-memory pipe.
var dial = func(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "tcp", addr)
}

// Send writes req.Raw unchanged to req.Target on a fresh connection and reads
// the response. With FollowRedirects it re-sends to each Location and returns
// every hop. On error the failed hop is the last one in the result.
func Send(ctx context.Context, req Request) (Result, error) {
	if req.Options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Options.Timeout)
		defer cancel()
	}

	var res Result
	target, raw := req.Target, req.Raw
	for {
		start := time.Now()
		resp, err := exchange(ctx, target, raw)
		res.Hops = append(res.Hops, Hop{Target: target, Raw: raw, Response: resp, Elapsed: time.Since(start)})
		if err != nil || !req.Options.FollowRedirects || len(res.Hops) > MaxRedirects {
			return res, err
		}
		var ok bool
		if target, raw, ok = redirect(target, raw, resp); !ok {
			return res, nil
		}
	}
}

func exchange(ctx context.Context, t Target, raw []byte) (*rawhttp.Response, error) {
	conn, err := dial(ctx, net.JoinHostPort(t.Host, strconv.Itoa(t.Port)))
	if err != nil {
		return nil, classify(ctx, KindDial, err)
	}
	defer conn.Close()
	// Closing the connection is what unblocks a pending read or write.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	if t.TLS {
		tc := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // T-101: Replay must reach self-signed and broken targets (F-REP-2)
			ServerName:         cmp.Or(t.SNI, t.Host),
			NextProtos:         []string{"http/1.1"},
		})
		if err := tc.HandshakeContext(ctx); err != nil {
			return nil, classify(ctx, KindTLS, err)
		}
		conn = tc
	}

	if _, err := conn.Write(raw); err != nil {
		return nil, classify(ctx, KindProtocol, err)
	}
	method, _, _ := bytes.Cut(raw, []byte(" "))
	br := bufio.NewReader(conn)
	var resp *rawhttp.Response
	for {
		resp, err = rawhttp.ReadResponse(br, string(method), rawhttp.DefaultLimits)
		if err != nil {
			return nil, classify(ctx, KindProtocol, err)
		}
		if resp.Status()/100 != 1 || resp.Status() == 101 {
			break
		}
	}
	// One byte past the cap marks the response truncated; the deferred Close drops the rest.
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body(), rawhttp.DefaultLimits.MaxBodyBytes+1)); err != nil {
		return nil, classify(ctx, KindProtocol, err)
	}
	return resp, nil
}

func classify(ctx context.Context, kind Kind, err error) error {
	switch ctx.Err() {
	case context.DeadlineExceeded:
		kind = KindTimeout
	case context.Canceled:
		kind = KindCancelled
	}
	return &Error{Kind: kind, Err: err}
}

func parseRequest(raw []byte) *rawhttp.Request {
	req, err := rawhttp.ReadRequest(bufio.NewReader(bytes.NewReader(raw)), rawhttp.Limits{})
	if err != nil {
		return nil
	}
	return req
}

// redirect builds the next hop for a 3xx response, or reports false when
// there is none to follow.
func redirect(t Target, raw []byte, resp *rawhttp.Response) (Target, []byte, bool) {
	status := resp.Status()
	switch status {
	case 301, 302, 303, 307, 308:
	default:
		return t, nil, false
	}
	loc, _ := resp.Header("Location")
	req := parseRequest(raw)
	if loc == "" || req == nil {
		return t, nil, false
	}

	scheme := "http"
	if t.TLS {
		scheme = "https"
	}
	base := &url.URL{Scheme: scheme, Host: net.JoinHostPort(t.Host, strconv.Itoa(t.Port)), Path: "/"}
	cur, err := base.Parse(req.Target())
	if err != nil {
		return t, nil, false
	}
	next, err := cur.Parse(loc)
	if err != nil || next.Scheme != "http" && next.Scheme != "https" || next.Hostname() == "" {
		return t, nil, false
	}

	nt := Target{Host: next.Hostname(), TLS: next.Scheme == "https", Port: 80}
	if nt.TLS {
		nt.Port = 443
	}
	if p := next.Port(); p != "" {
		if nt.Port, err = strconv.Atoi(p); err != nil {
			return t, nil, false
		}
	}

	b := req.Builder().SetTarget(next.RequestURI())
	if nt.Host == t.Host {
		nt.SNI = t.SNI
	}
	if nt.Host != t.Host || nt.Port != t.Port {
		b.SetHeader("Host", next.Host)
	}
	method := req.Method()
	if status == 303 && method != "HEAD" || (status == 301 || status == 302) && method == "POST" {
		// Builder streams req.Body(); draining it writes no body without adding Content-Length (SetBody would add one).
		_, _ = io.Copy(io.Discard, req.Body())
		b.SetMethod("GET").DelHeader("Content-Length").DelHeader("Content-Type").DelHeader("Transfer-Encoding")
	}
	return nt, b.Bytes(), true
}
