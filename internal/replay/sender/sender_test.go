package sender

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/testutil"
)

// listen serves each connection with handle on 127.0.0.1 and closes it after.
func listen(t *testing.T, handle func(net.Conn)) Target {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer c.Close()
				handle(c)
			})
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	return Target{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
}

// router answers each request with reply(req) and records the raw requests.
type router struct {
	mu   sync.Mutex
	reqs []string
}

func (r *router) serve(reply func(*rawhttp.Request) string) func(net.Conn) {
	return func(c net.Conn) {
		req, err := rawhttp.ReadRequest(bufio.NewReader(c), rawhttp.Limits{})
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, req.Body())
		r.mu.Lock()
		r.reqs = append(r.reqs, string(req.Raw()))
		r.mu.Unlock()
		_, _ = io.WriteString(c, reply(req))
	}
}

func (r *router) requests() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.reqs)
}

func TestSendVerbatim(t *testing.T) {
	t.Parallel()
	// F-REP-2 AC: a wrong Content-Length with the update off goes out byte for byte.
	const raw = "GET / HTTP/1.1\r\nhost: x\r\nContent-Length: 99\r\n\r\nabc"
	got := make(chan []byte, 1)
	target := listen(t, func(c net.Conn) {
		buf := make([]byte, len(raw))
		_, _ = io.ReadFull(c, buf)
		got <- buf
		_, _ = io.WriteString(c, testutil.RawReply)
	})

	res, err := Send(t.Context(), Request{Target: target, Raw: []byte(raw)})
	if err != nil {
		t.Fatal(err)
	}
	if b := <-got; string(b) != raw {
		t.Errorf("server got %q, want %q", b, raw)
	}
	if len(res.Hops) != 1 || string(res.Hops[0].Response.Raw()) != testutil.RawReply {
		t.Errorf("hops = %+v", res.Hops)
	}
}

func TestSendTLS(t *testing.T) {
	t.Parallel()
	srv := testutil.NewTLSRawServer(t)
	const raw = "GET / HTTP/1.1\r\nHost: x\r\n\r\n"
	tests := []struct {
		name, host, sni, want string
	}{
		{name: "override", host: "127.0.0.1", sni: "override.test", want: "override.test"},
		{name: "host name", host: "localhost", want: "localhost"},
		{name: "ip host sends none", host: "127.0.0.1", want: ""},
	}
	for _, tt := range tests {
		res, err := Send(t.Context(), Request{
			Host: tt.host, Port: srv.Port, TLS: true, SNI: tt.sni,
			Raw: []byte(raw),
		})
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := string(res.Hops[0].Response.Raw()); got != testutil.RawReply {
			t.Errorf("%s: response %q", tt.name, got)
		}
		hellos := srv.Hellos()
		h := hellos[len(hellos)-1]
		if h.ServerName != tt.want || !slices.Equal(h.ALPN, []string{"http/1.1"}) {
			t.Errorf("%s: hello = %+v, want SNI %q and ALPN [http/1.1]", tt.name, h, tt.want)
		}
	}
	if n := len(srv.Requests()); n != len(tests) {
		t.Errorf("server got %d requests, want %d", n, len(tests))
	}
}

func TestSendErrorKinds(t *testing.T) {
	t.Parallel()
	closed := listen(t, func(net.Conn) {})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := Target{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}
	_ = ln.Close()
	garbage := listen(t, func(c net.Conn) {
		_, _ = rawhttp.ReadRequest(bufio.NewReader(c), rawhttp.Limits{})
		_, _ = io.WriteString(c, "nonsense\r\n\r\n")
	})
	short := listen(t, func(c net.Conn) {
		_, _ = rawhttp.ReadRequest(bufio.NewReader(c), rawhttp.Limits{})
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc")
	})
	tlsOnPlain := closed
	tlsOnPlain.TLS = true

	tests := []struct {
		name   string
		target Target
		want   Kind
	}{
		{"dial", refused, KindDial},
		{"tls", tlsOnPlain, KindTLS},
		{"bad status line", garbage, KindProtocol},
		{"short body", short, KindProtocol},
	}
	for _, tt := range tests {
		res, err := Send(t.Context(), Request{Target: tt.target, Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
		if e, ok := errors.AsType[*Error](err); !ok || e.Kind != tt.want {
			t.Errorf("%s: err = %v, want kind %s", tt.name, err, tt.want)
		}
		if len(res.Hops) != 1 || res.Hops[0].Response != nil {
			t.Errorf("%s: hops = %+v, want the failed hop without response", tt.name, res.Hops)
		}
	}
}

// pipeDial makes Send talk to serve over an in-memory pipe so synctest can
// drive time and see every goroutine. Tests using it must not run in parallel.
func pipeDial(t *testing.T, serve func(net.Conn)) {
	old := dial
	t.Cleanup(func() { dial = old })
	dial = func(context.Context, string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			serve(server)
		}()
		return client, nil
	}
}

func TestSendTimeoutMidBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		closed := make(chan error, 1)
		pipeDial(t, func(c net.Conn) {
			br := bufio.NewReader(c)
			_, _ = rawhttp.ReadRequest(br, rawhttp.Limits{})
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc")
			_, err := br.ReadByte() // stalls until the sender closes
			closed <- err
		})

		start := time.Now()
		_, err := Send(t.Context(), Request{Raw: []byte("GET / HTTP/1.1\r\n\r\n"), Options: Options{Timeout: 5 * time.Second}})
		if e, ok := errors.AsType[*Error](err); !ok || e.Kind != KindTimeout {
			t.Fatalf("err = %v, want kind timeout", err)
		}
		if d := time.Since(start); d != 5*time.Second {
			t.Errorf("returned after %v, want 5s", d)
		}
		if err := <-closed; err == nil {
			t.Error("connection still open after timeout")
		}
	})
}

func TestSendCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		got, closed := make(chan struct{}), make(chan error, 1)
		pipeDial(t, func(c net.Conn) {
			br := bufio.NewReader(c)
			_, _ = rawhttp.ReadRequest(br, rawhttp.Limits{})
			close(got)
			_, err := br.ReadByte()
			closed <- err
		})

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := Send(ctx, Request{Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
			done <- err
		}()
		<-got
		cancel()
		err := <-done
		if e, ok := errors.AsType[*Error](err); !ok || e.Kind != KindCancelled {
			t.Fatalf("err = %v, want kind cancelled", err)
		}
		if err := <-closed; err == nil {
			t.Error("connection still open after cancel")
		}
		// synctest.Test fails if any goroutine from the bubble is still blocked.
	})
}

func TestSendTruncatesOverCap(t *testing.T) {
	t.Parallel()
	limit := rawhttp.DefaultLimits.MaxBodyBytes
	writeErr := make(chan error, 1)
	target := listen(t, func(c net.Conn) {
		_, _ = rawhttp.ReadRequest(bufio.NewReader(c), rawhttp.Limits{})
		_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", 4*limit)
		_, err := io.Copy(c, io.LimitReader(zeros{}, 4*limit))
		writeErr <- err
	})

	res, err := Send(t.Context(), Request{Target: target, Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	resp := res.Hops[0].Response
	if !resp.Truncated() || int64(len(resp.Raw())-len(resp.Head())) != limit {
		t.Errorf("truncated = %v, body = %d bytes, want true and %d", resp.Truncated(), len(resp.Raw())-len(resp.Head()), limit)
	}
	if err := <-writeErr; err == nil {
		t.Error("server wrote the whole body: connection was not closed at the cap")
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestSendRedirectsOffByDefault(t *testing.T) {
	t.Parallel()
	var r router
	target := listen(t, r.serve(func(*rawhttp.Request) string {
		return "HTTP/1.1 302 Found\r\nLocation: /next\r\nContent-Length: 0\r\n\r\n"
	}))
	res, err := Send(t.Context(), Request{Target: target, Raw: []byte("GET / HTTP/1.1\r\n\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hops) != 1 || res.Hops[0].Response.Status() != 302 {
		t.Errorf("hops = %d, want the 302 alone", len(res.Hops))
	}
}

func TestSendRedirectLimit(t *testing.T) {
	t.Parallel()
	var r router
	target := listen(t, r.serve(func(req *rawhttp.Request) string {
		n, _ := strconv.Atoi(strings.TrimPrefix(req.Target(), "/"))
		return fmt.Sprintf("HTTP/1.1 302 Found\r\nLocation: /%d\r\nContent-Length: 0\r\n\r\n", n+1)
	}))
	res, err := Send(t.Context(), Request{
		Target:  target,
		Raw:     []byte("GET /0 HTTP/1.1\r\n\r\n"),
		Options: Options{FollowRedirects: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hops) != MaxRedirects+1 {
		t.Fatalf("hops = %d, want %d", len(res.Hops), MaxRedirects+1)
	}
	for i, h := range res.Hops {
		if want := fmt.Sprintf("GET /%d HTTP/1.1\r\n\r\n", i); string(h.Raw) != want || h.Response.Status() != 302 {
			t.Errorf("hop %d = %q (%d), want %q", i, h.Raw, h.Response.Status(), want)
		}
	}
	if n := len(r.requests()); n != MaxRedirects+1 {
		t.Errorf("server got %d requests, want %d", n, MaxRedirects+1)
	}
}

func TestSendRedirectRewrites(t *testing.T) {
	t.Parallel()
	const post = "POST /a/b HTTP/1.1\r\nHost: origin\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nX-Keep: 1\r\n\r\nabc"
	tests := []struct {
		name, req, status, location, want string
	}{
		{
			name: "303 after POST", req: post, status: "303", location: "/next",
			want: "GET /next HTTP/1.1\r\nHost: origin\r\nX-Keep: 1\r\n\r\n",
		},
		{
			name: "302 after POST", req: post, status: "302", location: "/next",
			want: "GET /next HTTP/1.1\r\nHost: origin\r\nX-Keep: 1\r\n\r\n",
		},
		{
			name: "307 keeps method and body", req: post, status: "307", location: "/next?q=1",
			want: "POST /next?q=1 HTTP/1.1\r\nHost: origin\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nX-Keep: 1\r\n\r\nabc",
		},
		{
			name: "308 keeps method and body", req: post, status: "308", location: "/next",
			want: "POST /next HTTP/1.1\r\nHost: origin\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nX-Keep: 1\r\n\r\nabc",
		},
		{
			name: "relative location", req: post, status: "307", location: "c",
			want: "POST /a/c HTTP/1.1\r\nHost: origin\r\nContent-Type: text/plain\r\nContent-Length: 3\r\nX-Keep: 1\r\n\r\nabc",
		},
		{
			name: "303 keeps HEAD", req: "HEAD /a HTTP/1.1\r\nHost: origin\r\n\r\n", status: "303", location: "/next",
			want: "HEAD /next HTTP/1.1\r\nHost: origin\r\n\r\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var r router
			target := listen(t, r.serve(func(req *rawhttp.Request) string {
				if req.Target() == "/a/b" || req.Target() == "/a" {
					return "HTTP/1.1 " + tt.status + " Moved\r\nLocation: " + tt.location + "\r\nContent-Length: 0\r\n\r\n"
				}
				return testutil.RawReply
			}))
			res, err := Send(t.Context(), Request{Target: target, Raw: []byte(tt.req), Options: Options{FollowRedirects: true}})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Hops) != 2 {
				t.Fatalf("hops = %d, want 2", len(res.Hops))
			}
			if got := string(res.Hops[1].Raw); got != tt.want {
				t.Errorf("hop 2 = %q, want %q", got, tt.want)
			}
			if got := r.requests(); len(got) != 2 || got[0] != tt.req || got[1] != tt.want {
				t.Errorf("server got %q", got)
			}
			if res.Hops[1].Target != target || res.Hops[1].Response.Status() != 200 {
				t.Errorf("hop 2 target %+v status %d", res.Hops[1].Target, res.Hops[1].Response.Status())
			}
		})
	}
}

func TestSendRedirectCrossHost(t *testing.T) {
	t.Parallel()
	tlsSrv := testutil.NewTLSRawServer(t)
	location := fmt.Sprintf("https://localhost:%d/there", tlsSrv.Port)
	var r router
	origin := listen(t, r.serve(func(*rawhttp.Request) string {
		return "HTTP/1.1 302 Found\r\nLocation: " + location + "\r\nContent-Length: 0\r\n\r\n"
	}))
	origin.SNI = "ignored.test" // plain HTTP; must not leak to the new host

	res, err := Send(t.Context(), Request{
		Target:  origin,
		Raw:     []byte("GET /here HTTP/1.1\r\nHost: origin\r\n\r\n"),
		Options: Options{FollowRedirects: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hops) != 2 {
		t.Fatalf("hops = %d, want 2", len(res.Hops))
	}
	hop := res.Hops[1]
	if want := (Target{Host: "localhost", Port: tlsSrv.Port, TLS: true}); hop.Target != want {
		t.Errorf("target = %+v, want %+v", hop.Target, want)
	}
	want := fmt.Sprintf("GET /there HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", tlsSrv.Port)
	if got := tlsSrv.Requests(); len(got) != 1 || string(got[0]) != want {
		t.Errorf("TLS server got %q, want %q", got, want)
	}
	if h := tlsSrv.Hellos(); len(h) != 1 || h[0].ServerName != "localhost" {
		t.Errorf("hellos = %+v, want SNI localhost", h)
	}
	if string(hop.Response.Raw()) != testutil.RawReply {
		t.Errorf("final response %q", hop.Response.Raw())
	}
}
