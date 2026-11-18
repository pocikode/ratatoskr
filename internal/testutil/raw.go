// Package testutil holds servers and helpers shared by integration tests.
package testutil

import (
	"bufio"
	"io"
	"net"
	"sync"
	"testing"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// RawReply is what a [RawServer] answers every request with.
const RawReply = "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok"

// RawServer is a plain TCP server that records each request exactly as it
// arrived and answers [RawReply]. It frames requests with rawhttp, so it also
// accepts the malformed-but-forwardable messages the proxy must pass on.
type RawServer struct {
	Host string
	Port int

	mu   sync.Mutex
	reqs [][]byte
}

// NewRawServer starts a RawServer on loopback; it stops with the test.
func NewRawServer(t testing.TB) *RawServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := ln.Addr().(*net.TCPAddr)
	s := &RawServer{Host: "127.0.0.1", Port: addr.Port}

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			wg.Go(func() { s.serve(c) })

			t.Cleanup(func() { _ = c.Close() })
		}
	})

	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	return s
}

func (s *RawServer) serve(c net.Conn) {
	defer c.Close()

	br := bufio.NewReader(c)

	for {
		req, err := rawhttp.ReadRequest(br, rawhttp.Limits{})
		if err != nil {
			return
		}

		if _, err := io.Copy(io.Discard, req.Body()); err != nil {
			return
		}

		s.mu.Lock()
		s.reqs = append(s.reqs, req.Raw())
		s.mu.Unlock()

		if _, err := io.WriteString(c, RawReply); err != nil {
			return
		}
	}
}

// Requests returns the raw requests received so far.
func (s *RawServer) Requests() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([][]byte(nil), s.reqs...)
}
