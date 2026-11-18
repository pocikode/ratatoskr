package netx_test

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pocikode/ratatoskr/internal/netx"
)

// serve accepts and holds connections until the test ends.
func serve(t *testing.T) netx.Key {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			t.Cleanup(func() { _ = c.Close() })
		}
	}()

	return netx.Key{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
}

func TestPoolReuse(t *testing.T) {
	t.Parallel()

	key := serve(t)
	p := &netx.Pool{}
	t.Cleanup(p.Close)

	c, reused, err := p.Get(t.Context(), key)
	if err != nil || reused {
		t.Fatalf("first Get: reused=%v err=%v", reused, err)
	}

	p.Put(c)

	c2, reused, err := p.Get(t.Context(), key)
	if err != nil || !reused || c2 != c {
		t.Fatalf("second Get: reused=%v same=%v err=%v", reused, c2 == c, err)
	}

	_ = c2.Close()
}

func TestPoolIdleExpiry(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		p := &netx.Pool{
			IdleTimeout: time.Minute,
			DialContext: func(context.Context, string, string) (net.Conn, error) {
				c, s := net.Pipe()
				t.Cleanup(func() { _ = s.Close() })

				return c, nil
			},
		}
		key := netx.Key{Host: "h", Port: 1}

		c, _, err := p.Get(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}

		p.Put(c)
		time.Sleep(2 * time.Minute) // fake clock

		c2, reused, err := p.Get(t.Context(), key)
		if err != nil || reused {
			t.Fatalf("Get after expiry: reused=%v err=%v", reused, err)
		}

		_ = c2.Close()
		p.Close()
	})
}
