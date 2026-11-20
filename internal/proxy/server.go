// Package proxy is the intercepting HTTP/1.x proxy: it multiplexes proxy and
// UI traffic on one listener, terminates TLS for CONNECT with certificates from
// internal/ca, forwards through internal/rawhttp and internal/netx, and hands
// every exchange to the capture sink (docs/03-architecture.md §4).
//
// Traffic never touches net/http's client or server (ADR-0003); the UI handler
// is the one exception, and only serves connections addressed to Ratatoskr
// itself.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/netx"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

const (
	// UIHost is the name that always reaches the UI/API, also through the proxy
	// (F-NET-1, F-NET-2).
	UIHost = "ratatoskr.localhost"

	defaultIdleTimeout    = 2 * time.Minute
	handshakeTimeout      = 10 * time.Second
	defaultHeaderTimeout  = 60 * time.Second
	maxConns              = 10000
	maxHeaderValues       = 512
	uiShutdownGrace       = 5 * time.Second
	connectEstablished    = "HTTP/1.1 200 Connection Established\r\n\r\n"
	tlsHandshakeRecordTag = 0x16
)

// Capture records requests and their responses. *history.Capture implements it.
type Capture interface {
	NewPending(ctx context.Context, e history.Entry) int64
	Complete(ctx context.Context, id int64, resp *history.Response)
}

// CA supplies the MITM certificates. *ca.Service implements it.
type CA interface {
	TLSConfig() *tls.Config
	Certificate(host string) (*tls.Certificate, error)
}

// Deps are the inputs to [New].
type Deps struct {
	// CA is required for CONNECT interception.
	CA CA
	// Pool dials upstreams. Nil means a zero [netx.Pool].
	Pool *netx.Pool
	// Capture records exchanges. Nil records nothing. Holds and edited pairs
	// need it to implement [HoldCapture].
	Capture Capture
	// Interceptor holds messages for review. Nil disables Intercept.
	Interceptor Interceptor
	// Rewriter applies Match & Replace. Nil disables it.
	Rewriter Rewriter
	// UI serves requests addressed to Ratatoskr. Nil answers 404.
	UI     http.Handler
	Logger *slog.Logger
	// Limits bound what the HTTP reader buffers.
	Limits rawhttp.Limits
	// ShutdownGrace is how long in-flight UI requests may take once Serve's
	// context ends. Zero means 5 s.
	ShutdownGrace time.Duration
	// HeaderTimeout is how long an upstream may take to answer with a head.
	HeaderTimeout time.Duration
	// IdleTimeout bounds each read and write on a connection that makes no
	// progress. Zero means 2 min.
	IdleTimeout time.Duration
}

// Server is the proxy. Create it with [New] and run it with [Server.Serve].
type Server struct {
	deps Deps
	pool *netx.Pool
	log  *slog.Logger
	// hc is Capture when it supports holds and edited pairs.
	hc HoldCapture
	// icpt is Interceptor when hc is set.
	icpt Interceptor

	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

// New creates a Server.
func New(deps Deps) *Server {
	s := &Server{deps: deps, pool: deps.Pool, log: deps.Logger, conns: map[net.Conn]struct{}{}}
	if s.pool == nil {
		s.pool = &netx.Pool{}
	}

	if s.log == nil {
		s.log = slog.Default()
	}

	if s.deps.ShutdownGrace == 0 {
		s.deps.ShutdownGrace = uiShutdownGrace
	}

	if s.deps.IdleTimeout == 0 {
		s.deps.IdleTimeout = defaultIdleTimeout
	}

	if s.deps.HeaderTimeout == 0 {
		s.deps.HeaderTimeout = defaultHeaderTimeout
	}

	s.hc, _ = deps.Capture.(HoldCapture)

	switch {
	case deps.Interceptor == nil:
	case s.hc == nil:
		s.log.Warn("proxy: capture cannot confirm pending records; Intercept holds are off", "component", "proxy")
	default:
		s.icpt = deps.Interceptor
	}

	return s
}

// listenerInfo is what the origin-form check needs to know about the listener.
type listenerInfo struct {
	port int
}

// Serve accepts on ln until ctx ends, then closes every connection and returns
// nil. It returns an error only when accepting fails. Serve owns ln.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	info := listenerInfo{port: port}

	ui := &chanListener{ch: make(chan net.Conn), done: make(chan struct{}), addr: ln.Addr()}
	handler := s.deps.UI

	if handler == nil {
		handler = http.NotFoundHandler()
	}

	uiSrv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: handshakeTimeout,
		IdleTimeout:       s.deps.IdleTimeout,
		MaxHeaderBytes:    s.maxHead(),
		// Same hardening the UI server had when it owned the listener.
		MaxHeaderValueCount: maxHeaderValues,
		ErrorLog:            slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}

	var wg, uiWG sync.WaitGroup

	uiWG.Go(func() { _ = uiSrv.Serve(ui) }) // returns once uiSrv is shut down below

	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer stop()

	err := s.acceptLoop(ctx, ln, info, ui, &wg)

	_ = ln.Close()

	s.closeAll()
	wg.Wait()

	// In-flight UI requests get the grace period to finish; ctx is already
	// cancelled here, so the deadline is its own.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.deps.ShutdownGrace)
	defer cancel()

	shutErr := uiSrv.Shutdown(sctx)
	if shutErr != nil {
		_ = uiSrv.Close()
	}

	_ = ui.Close()

	uiWG.Wait()

	if shutErr != nil {
		return fmt.Errorf("proxy: shutdown: %w", shutErr)
	}

	if ctx.Err() != nil {
		return nil
	}

	return err
}

func (s *Server) maxHead() int {
	if s.deps.Limits.MaxHeadBytes > 0 {
		return s.deps.Limits.MaxHeadBytes
	}

	return rawhttp.DefaultLimits.MaxHeadBytes
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener, info listenerInfo, ui *chanListener, wg *sync.WaitGroup) error {
	backoff := time.Duration(0)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return err
			}

			// Temporary failure (EMFILE and friends): back off, do not spin.
			backoff = min(max(backoff*2, 5*time.Millisecond), time.Second)
			s.log.Warn("proxy: accept", "component", "proxy", "error", err, "retry_in", backoff)

			select {
			case <-time.After(backoff):
				continue
			case <-ctx.Done():
				return err
			}
		}

		backoff = 0

		if !s.track(conn) {
			_ = conn.Close()

			continue
		}

		wg.Go(func() { s.serveConn(ctx, conn, info, ui) })
	}
}

func (s *Server) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.conns) >= maxConns {
		return false
	}

	s.conns[c] = struct{}{}

	return true
}

func (s *Server) untrack(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
}

func (s *Server) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for c := range s.conns {
		_ = c.Close()
	}
}

// session is one client connection's context.
type session struct {
	s    *Server
	ctx  context.Context
	info listenerInfo
	ui   *chanListener
	conn net.Conn
	br   *bufio.Reader
	// src is what br reads from; a hold may put pipelined bytes in front of it.
	src io.Reader
	// idle refreshes the client read deadline on every read.
	idle *netx.IdleConn
	// tun is the CONNECT destination when the connection is a tunnel.
	tun *destination
	// continued is set while a 100 Continue was already sent for the request.
	continued bool
	// handedOff is set once the UI server owns the connection.
	handedOff bool
}

// destination is where a request goes.
type destination struct {
	host string
	port int
	tls  bool
	// sni is the name the client asked for in its TLS handshake.
	sni string
}

func (d destination) key() netx.Key {
	return netx.Key{Host: d.host, Port: d.port, TLS: d.tls, SNI: d.sni}
}

func (s *Server) serveConn(ctx context.Context, conn net.Conn, info listenerInfo, ui *chanListener) {
	defer s.untrack(conn)

	ic := &netx.IdleConn{Conn: conn, Idle: s.deps.IdleTimeout}
	sess := &session{s: s, ctx: ctx, info: info, ui: ui, conn: ic, br: bufio.NewReader(ic), src: ic, idle: ic}

	sess.requests()

	if !sess.handedOff {
		_ = conn.Close()
	}
}

// requests serves the request loop of one connection until it ends.
func (c *session) requests() {
	for {
		req, err := rawhttp.ReadRequest(c.br, c.s.deps.Limits)
		if err != nil {
			c.rejectBadRequest(err)

			return
		}

		if !c.handle(req) {
			return
		}
	}
}

// rejectBadRequest answers a head the reader refused. A closed or idle
// connection is not worth an answer.
func (c *session) rejectBadRequest(err error) {
	status := ""

	switch {
	case errors.Is(err, rawhttp.ErrHeadTooLarge):
		status = "431 Request Header Fields Too Large"
	case errors.Is(err, rawhttp.ErrMalformed), errors.Is(err, rawhttp.ErrFraming):
		status = "400 Bad Request"
	default:
		return
	}

	_, _ = io.WriteString(c.conn, "HTTP/1.1 "+status+"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
}

// handle routes one request and reports whether the connection stays usable.
func (c *session) handle(req *rawhttp.Request) bool {
	if c.tun != nil {
		return c.forward(req, *c.tun, true)
	}

	target := req.Target()

	if strings.EqualFold(req.Method(), "CONNECT") {
		return c.connect(req)
	}

	if scheme, rest, ok := cutScheme(target); ok {
		authority, path := splitAuthority(rest)
		defPort, isTLS := 80, scheme == "https"

		if isTLS {
			defPort = 443
		}

		host, port, ok := parseAuthority(authority, defPort)
		if !ok {
			c.writeSimple("400 Bad Request", "bad request target")

			return false
		}

		// A target naming the listener itself must not be forwarded back to it.
		if !isTLS && c.isUIHost(authority) || strings.EqualFold(host, UIHost) {
			return c.serveUIAbsolute(req, path)
		}

		return c.forward(req, destination{host: host, port: port, tls: isTLS}, false)
	}

	hostHeader, _ := req.Header("Host")

	if c.isUIHost(hostHeader) {
		return c.handOffToUI(req.Head())
	}

	// An origin-form request for somebody else would be invisible proxying,
	// which is an opt-in listener option (F-NET-6, not built yet).
	c.writeSimple("421 Misdirected Request", "this is the Ratatoskr proxy listener; configure your client to use it as a proxy")

	return false
}

// isUIHost reports whether an origin-form Host header addresses Ratatoskr.
func (c *session) isUIHost(hostport string) bool {
	host, port, ok := parseAuthority(hostport, 80)
	if !ok {
		return false
	}

	if strings.EqualFold(host, UIHost) {
		return true
	}

	// Host = listener address: localhost, loopback or the listener's own IP on our port.
	if port != c.info.port {
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}

	// Compare with the address the client connected to: the listen address may be 0.0.0.0.
	local, lerr := netip.ParseAddrPort(c.conn.LocalAddr().String())

	return ip.IsLoopback() || lerr == nil && ip.Unmap() == local.Addr().Unmap()
}

// serveUIAbsolute serves `GET http://ratatoskr.localhost/ca.crt`-style requests
// that arrive through the proxy: the target is rewritten to origin-form and the
// request is replayed into the UI server. Only bodyless methods are supported.
func (c *session) serveUIAbsolute(req *rawhttp.Request, path string) bool {
	if req.Framing() != rawhttp.FramingNone || (req.Method() != http.MethodGet && req.Method() != http.MethodHead) {
		c.writeSimple("405 Method Not Allowed", "only GET and HEAD reach the UI through the proxy")

		return false
	}

	b := req.Builder().SetTarget(path).DelHeader("Proxy-Connection").
		SetHeader("Connection", "close") // the UI server owns the connection now; later requests must be proxied

	return c.handOffToUI(b.Bytes())
}

// handOffToUI gives the connection to the UI server, replaying head (the
// request already read) followed by whatever is still buffered.
func (c *session) handOffToUI(head []byte) bool {
	c.idle.Idle = 0
	_ = c.conn.SetDeadline(time.Time{})

	rc := &replayConn{Conn: c.conn, r: io.MultiReader(bytes.NewReader(head), c.br)}

	// Untrack first: once sent, the UI server may already be serving the
	// connection, and a shutdown closeAll must not close it under that request.
	// If the hand-off fails, serveConn closes it (handedOff stays false).
	c.s.untrack(c.idle.Conn)

	select {
	case c.ui.ch <- rc:
		c.handedOff = true
	case <-c.ui.done:
	case <-c.ctx.Done():
	}

	return false
}

func (c *session) writeSimple(status, text string) {
	_, _ = io.WriteString(c.conn, "HTTP/1.1 "+status+"\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: "+
		strconv.Itoa(len(text))+"\r\nConnection: close\r\n\r\n"+text)
}

// connect answers a CONNECT: it intercepts TLS when the client starts a
// handshake and otherwise tunnels plain HTTP, then serves the requests inside.
func (c *session) connect(req *rawhttp.Request) bool {
	host, port, ok := parseAuthority(req.Target(), 443)
	if !ok {
		c.writeSimple("400 Bad Request", "bad CONNECT target")

		return false
	}

	if c.s.deps.CA == nil {
		c.writeSimple("501 Not Implemented", "TLS interception is not configured")

		return false
	}

	if _, err := io.WriteString(c.conn, connectEstablished); err != nil {
		return false
	}

	c.idle.Idle = handshakeTimeout

	first, err := c.br.Peek(1)
	if err != nil {
		return false
	}

	if first[0] != tlsHandshakeRecordTag {
		c.idle.Idle = c.s.deps.IdleTimeout
		c.tun = &destination{host: host, port: port}

		return true // plain HTTP inside the tunnel: keep looping
	}

	tc := tls.Server(&replayConn{Conn: c.conn, r: c.br}, c.mitmConfig(host))

	// One deadline for the whole handshake, reads and writes alike.
	c.idle.Idle = 0
	_ = tc.SetDeadline(time.Now().Add(handshakeTimeout))

	if err := tc.HandshakeContext(c.ctx); err != nil {
		c.s.log.Debug("proxy: tls handshake", "component", "proxy", "host", host, "error", err)

		return false
	}

	_ = tc.SetDeadline(time.Time{})
	c.idle.Idle = c.s.deps.IdleTimeout

	c.conn, c.br, c.src = tc, bufio.NewReader(tc), tc
	c.tun = &destination{host: host, port: port, tls: true, sni: tc.ConnectionState().ServerName}

	return true
}

// mitmConfig is the CA's server config with the CONNECT host as the fallback
// name for clients that send no SNI (F-NET-2).
func (c *session) mitmConfig(host string) *tls.Config {
	cfg := c.s.deps.CA.TLSConfig().Clone()
	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if hello.ServerName == "" {
			return c.s.deps.CA.Certificate(host)
		}

		return c.s.deps.CA.Certificate(hello.ServerName)
	}

	return cfg
}

// replayConn reads from r (data already pulled out of the connection) while
// writes go to the connection.
type replayConn struct {
	net.Conn

	r io.Reader
}

func (c *replayConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// chanListener feeds connections into an http.Server.
type chanListener struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
	addr net.Addr
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })

	return nil
}

func (l *chanListener) Addr() net.Addr { return l.addr }

// cutScheme splits "http://rest" and "https://rest" (any case).
func cutScheme(target string) (scheme, rest string, ok bool) {
	i := strings.Index(target, "://")
	if i < 0 || i > 5 {
		return "", "", false
	}

	scheme = strings.ToLower(target[:i])
	if scheme != "http" && scheme != "https" {
		return "", "", false
	}

	return scheme, target[i+3:], true
}

// splitAuthority splits "host:port/path?q" into the authority and an
// origin-form target.
func splitAuthority(rest string) (authority, origin string) {
	i := strings.IndexAny(rest, "/?#")
	if i < 0 {
		return rest, "/"
	}

	authority, origin = rest[:i], rest[i:]
	if origin[0] != '/' {
		origin = "/" + origin
	}

	return authority, origin
}

// parseAuthority reads "[user@]host[:port]"; IPv6 literals keep working.
func parseAuthority(authority string, defPort int) (host string, port int, ok bool) {
	if _, after, found := strings.Cut(authority, "@"); found {
		authority = after
	}

	host, portStr, err := net.SplitHostPort(authority)
	if err != nil {
		host, portStr = strings.Trim(authority, "[]"), ""
	}

	if host == "" {
		return "", 0, false
	}

	if portStr == "" {
		return host, defPort, true
	}

	p, err := strconv.Atoi(portStr)
	if err != nil || p < 1 || p > 65535 {
		return "", 0, false
	}

	return host, p, true
}
