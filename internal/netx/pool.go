// Package netx dials upstream servers for the proxy and keeps idle connections
// alive for reuse (docs/03-architecture.md §4).
package netx

import (
	"bufio"
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"sync"
	"time"
)

// Defaults from docs/03-architecture.md §4.
const (
	DefaultDialTimeout = 10 * time.Second
	DefaultTLSTimeout  = 10 * time.Second
	DefaultIdleTimeout = 90 * time.Second
	DefaultMaxIdle     = 16 // idle connections kept per key
)

// Key identifies connections that can serve the same requests.
type Key struct {
	Host string
	Port int
	TLS  bool
	// SNI is the TLS server name; empty means Host.
	SNI string
}

// Addr is "host:port" in dialable form (IPv6 hosts bracketed).
func (k Key) Addr() string { return net.JoinHostPort(k.Host, strconv.Itoa(k.Port)) }

// Conn is an upstream connection with the buffered reader responses are read
// through. Return it with [Pool.Put] or release it with Close.
type Conn struct {
	net.Conn

	// BR reads from the connection; it must be the only reader.
	BR  *bufio.Reader
	key Key
	// idle refreshes the read deadline of every read while set.
	idle *IdleConn
	// timer expires the connection while it sits idle in the pool.
	timer *time.Timer
}

// Pool dials and pools upstream connections. The zero value works with the
// defaults; set the exported fields before first use. Pooling uses timers, not
// goroutines.
type Pool struct {
	// DialContext opens the TCP connection; nil means a zero [net.Dialer].
	// Tests swap it for an in-memory network.
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	// TLS is the template for upstream TLS; ServerName and NextProtos are set
	// per connection.
	TLS         *tls.Config
	DialTimeout time.Duration
	TLSTimeout  time.Duration
	IdleTimeout time.Duration
	MaxIdle     int

	mu     sync.Mutex
	idle   map[Key][]*Conn
	closed bool
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}

	return v
}

// Get returns a pooled connection for key, or dials a new one. reused reports
// which: a reused connection may have been closed by the server meanwhile, so a
// caller that has not sent a body may retry on a fresh one.
func (p *Pool) Get(ctx context.Context, key Key) (c *Conn, reused bool, err error) {
	if c = p.take(key); c != nil {
		return c, true, nil
	}

	c, err = p.dial(ctx, key)

	return c, false, err
}

// Fresh dials a new connection, bypassing the pool.
func (p *Pool) Fresh(ctx context.Context, key Key) (*Conn, error) { return p.dial(ctx, key) }

func (p *Pool) take(key Key) *Conn {
	p.mu.Lock()
	defer p.mu.Unlock()

	for conns := p.idle[key]; len(conns) > 0; conns = p.idle[key] {
		c := conns[len(conns)-1] // most recently used is the likeliest alive
		p.idle[key] = conns[:len(conns)-1]

		if c.timer.Stop() {
			return c
		}
		// The timer already fired: expire is closing this one.
	}

	return nil
}

func (p *Pool) dial(ctx context.Context, key Key) (*Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, orDefault(p.DialTimeout, DefaultDialTimeout))
	defer cancel()

	dial := p.DialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}

	raw, err := dial(dctx, "tcp", key.Addr())
	if err != nil {
		return nil, err
	}

	if key.TLS {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if p.TLS != nil {
			cfg = p.TLS.Clone()
		}

		cfg.ServerName = orDefault(key.SNI, key.Host)
		cfg.NextProtos = []string{"http/1.1"}

		tctx, tcancel := context.WithTimeout(ctx, orDefault(p.TLSTimeout, DefaultTLSTimeout))
		defer tcancel()

		tc := tls.Client(raw, cfg)
		if err := tc.HandshakeContext(tctx); err != nil {
			_ = raw.Close()

			return nil, err
		}

		return newConn(tc, key), nil
	}

	return newConn(raw, key), nil
}

func newConn(nc net.Conn, key Key) *Conn {
	ic := &IdleConn{Conn: nc}

	return &Conn{Conn: nc, BR: bufio.NewReader(ic), key: key, idle: ic}
}

// SetIdle makes every read wait at most d for data (0 turns it off), so a peer
// that stalls mid-body cannot hold a goroutine forever.
func (c *Conn) SetIdle(d time.Duration) {
	c.idle.Idle = d

	if d == 0 {
		_ = c.SetReadDeadline(time.Time{})
	}
}

// IdleConn refreshes the read deadline before every Read while Idle > 0.
type IdleConn struct {
	net.Conn

	Idle time.Duration
}

func (c *IdleConn) Read(p []byte) (int, error) {
	if c.Idle > 0 {
		_ = c.SetReadDeadline(time.Now().Add(c.Idle))
	}

	return c.Conn.Read(p)
}

// Put returns a healthy connection for reuse. The caller must have read the
// previous response completely; unread bytes make the connection unusable, so
// Put closes it instead.
func (p *Pool) Put(c *Conn) {
	if c.BR.Buffered() > 0 {
		_ = c.Close()

		return
	}

	c.SetIdle(0)

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || len(p.idle[c.key]) >= orDefault(p.MaxIdle, DefaultMaxIdle) {
		_ = c.Close()

		return
	}

	if p.idle == nil {
		p.idle = map[Key][]*Conn{}
	}

	c.timer = time.AfterFunc(orDefault(p.IdleTimeout, DefaultIdleTimeout), func() { p.expire(c) })
	p.idle[c.key] = append(p.idle[c.key], c)
}

func (p *Pool) expire(c *Conn) {
	p.mu.Lock()
	conns := p.idle[c.key]

	for i, x := range conns {
		if x == c {
			p.idle[c.key] = append(conns[:i:i], conns[i+1:]...)

			break
		}
	}
	p.mu.Unlock()

	_ = c.Close()
}

// Close closes every idle connection and stops pooling. Connections handed out
// earlier are closed by Put.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()

	for _, conns := range idle {
		for _, c := range conns {
			if c.timer.Stop() {
				_ = c.Close()
			}
		}
	}
}
