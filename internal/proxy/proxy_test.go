package proxy_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/tools/txtar"

	"github.com/pocikode/ratatoskr/internal/proxy"
	"github.com/pocikode/ratatoskr/internal/testutil"
)

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()

	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	return resp.StatusCode, string(b)
}

func upstream(t *testing.T, tls bool) *httptest.Server {
	t.Helper()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "hello %s %s", r.Method, r.URL.RequestURI())
	})

	var s *httptest.Server
	if tls {
		s = httptest.NewTLSServer(h)
	} else {
		s = httptest.NewServer(h)
	}

	t.Cleanup(s.Close)

	return s
}

func TestHTTPAndHTTPSMatchDirect(t *testing.T) {
	t.Parallel()

	r := startProxy(t)

	for _, tls := range []bool{false, true} {
		t.Run(fmt.Sprintf("tls=%v", tls), func(t *testing.T) {
			s := upstream(t, tls)

			code, via := get(t, r.client(t), s.URL+"/a/b?x=1")
			if code != 200 || via != "hello GET /a/b?x=1" {
				t.Fatalf("via proxy: %d %q", code, via)
			}

			_, direct := get(t, s.Client(), s.URL+"/a/b?x=1")
			if via != direct {
				t.Fatalf("proxy body %q != direct %q", via, direct)
			}
		})
	}

	got := r.rec.all()
	if len(got) != 2 || got[0].TLS || !got[1].TLS || got[1].Method != http.MethodGet || got[1].Resp == nil || got[1].Resp.Code != 200 {
		t.Fatalf("entries = %+v", got)
	}
}

func TestUIRouting(t *testing.T) {
	t.Parallel()

	r := startProxy(t)
	c := r.client(t)

	// origin-form to the listener itself, no proxy involved
	if code, body := get(t, http.DefaultClient, "http://"+r.addr+"/x"); code != 200 || body != "ui:/x" {
		t.Fatalf("direct UI: %d %q", code, body)
	}

	// F-NET-2: the CA download through the proxy
	if code, body := get(t, c, "http://ratatoskr.localhost/ca.crt"); code != 200 || body != "ui:/ca.crt" {
		t.Fatalf("UI via proxy: %d %q", code, body)
	}

	if n := len(r.rec.all()); n != 0 {
		t.Fatalf("UI traffic was recorded: %d entries", n)
	}
}

// A keep-alive client that fetched the CA must still be proxied afterwards.
func TestUIDownloadDoesNotCaptureConnection(t *testing.T) {
	t.Parallel()

	r := startProxy(t)
	up := upstream(t, false)
	c := r.client(t)

	if code, _ := get(t, c, "http://ratatoskr.localhost/ca.crt"); code != 200 {
		t.Fatalf("UI via proxy: %d", code)
	}

	if code, body := get(t, c, up.URL+"/x"); code != 200 || body != "hello GET /x" {
		t.Fatalf("after UI: %d %q", code, body)
	}

	if n := len(r.rec.all()); n != 1 {
		t.Fatalf("recorded %d entries, want 1", n)
	}
}

func TestOriginFormForeignHostIsMisdirected(t *testing.T) {
	t.Parallel()

	r := startProxy(t)
	up := upstream(t, false)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+r.addr+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Host = strings.TrimPrefix(up.URL, "http://")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusMisdirectedRequest || len(r.rec.all()) != 0 {
		t.Fatalf("status %d, entries %d", resp.StatusCode, len(r.rec.all()))
	}
}

func TestUpstreamErrorIs502AndRecorded(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	dead := ln.Addr().String()
	_ = ln.Close() // nothing listens there now

	r := startProxy(t)

	code, body := get(t, r.client(t), "http://"+dead+"/")
	if code != 502 || !strings.Contains(body, dead) {
		t.Fatalf("got %d %q", code, body)
	}

	got := r.rec.all()
	if len(got) != 1 || got[0].Resp == nil || got[0].Resp.Code != 502 || !bytes.Contains(got[0].Resp.Raw, []byte(dead)) {
		t.Fatalf("entries = %+v", got)
	}
}

func TestPooledConnectionReused(t *testing.T) {
	t.Parallel()

	r := startProxy(t)

	var conns int

	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	s.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			conns++
		}
	}
	s.Start()
	t.Cleanup(s.Close)

	c := r.client(t)
	for range 5 {
		get(t, c, s.URL)
	}

	if conns != 1 {
		t.Fatalf("upstream saw %d connections, want 1", conns)
	}
}

// TestCorpusByteExact sends every wire request of the rawhttp golden corpus
// through a CONNECT tunnel to a raw TCP server and requires the exact bytes.
func TestCorpusByteExact(t *testing.T) {
	t.Parallel()

	archive, err := txtar.ParseFile("../rawhttp/testdata/requests.txtar")
	if err != nil {
		t.Fatal(err)
	}

	r := startProxy(t)
	sent := 0

	for _, f := range archive.Files {
		name, field, _ := strings.Cut(f.Name, "/")
		if field != "wire" {
			continue
		}

		wire, err := strconv.Unquote(strings.TrimSpace(string(f.Data)))
		if err != nil || !forwardable(archive, name) {
			continue
		}

		srv := testutil.NewRawServer(t)
		conn, br := connectThrough(t, r.addr, srv.Host, srv.Port)

		if _, err := io.WriteString(conn, wire); err != nil {
			t.Fatal(err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		if _, err := br.ReadString('k'); err != nil { // reply ends in "ok"
			t.Fatalf("%s: no reply: %v", name, err)
		}

		_ = conn.Close()

		reqs := srv.Requests()
		if len(reqs) != 1 || string(reqs[0]) != wire {
			t.Errorf("%s: upstream got %q, want %q", name, reqs, wire)
		}

		sent++
	}

	if sent < 20 {
		t.Fatalf("only %d corpus requests exercised", sent)
	}
}

// forwardable keeps the cases whose bytes all belong to the one message: those
// with an expectation override ("raw", "rest") or a parse error are skipped.
func forwardable(a *txtar.Archive, name string) bool {
	for _, f := range a.Files {
		n, field, _ := strings.Cut(f.Name, "/")
		if n == name && (field == "raw" || field == "rest" || field == "error") {
			return false
		}
	}

	return true
}

func connectThrough(t *testing.T, proxyAddr, host string, port int) (net.Conn, *bufio.Reader) {
	t.Helper()

	conn, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	target := net.JoinHostPort(host, strconv.Itoa(port))
	_, _ = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)

	br := bufio.NewReader(conn)

	status, err := br.ReadString('\n')
	if err != nil || !strings.Contains(status, "200") {
		t.Fatalf("CONNECT: %q %v", status, err)
	}

	if _, err := br.ReadString('\n'); err != nil { // blank line
		t.Fatal(err)
	}

	return conn, br
}

// Servers close idle keep-alive connections early; a POST after a pause must
// not be sent on the dead one.
func TestPostAfterUpstreamIdleClose(t *testing.T) {
	t.Parallel()

	r := startProxy(t)

	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	s.Config.IdleTimeout = 50 * time.Millisecond
	s.Start()
	t.Cleanup(s.Close)

	c := r.client(t)

	for i := range 2 {
		resp, err := c.Post(s.URL, "text/plain", strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}

		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %d: status %d", i, resp.StatusCode)
		}

		time.Sleep(300 * time.Millisecond) //nolint:forbidigo // T-011: real upstream idle timer, no synctest over real sockets
	}
}

// An absolute target naming the listener with a zero-padded port is the UI,
// not a request to forward to ourselves.
func TestSelfTargetWithPaddedPortIsUI(t *testing.T) {
	t.Parallel()

	r := startProxy(t)
	_, port, _ := net.SplitHostPort(r.addr)

	conn, err := net.Dial("tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	_, _ = fmt.Fprintf(conn, "GET http://127.0.0.1:%s/ HTTP/1.1\r\nHost: 127.0.0.1:0%s\r\nConnection: close\r\n\r\n", port, port)

	b, _ := io.ReadAll(conn)
	if !bytes.HasSuffix(b, []byte("ui:/")) || len(r.rec.all()) != 0 {
		t.Fatalf("reply %q, entries %d", b, len(r.rec.all()))
	}
}

// An upstream that sends a head and then stalls must not hold the session.
func TestStalledUpstreamBodyTimesOut(t *testing.T) {
	t.Parallel()

	r := startProxy(t, func(d *proxy.Deps) { d.IdleTimeout = 100 * time.Millisecond })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}

		t.Cleanup(func() { _ = c.Close() })

		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nabc")
	}()

	conn, err := net.Dial("tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	_, _ = fmt.Fprintf(conn, "GET http://%s/ HTTP/1.1\r\nHost: %[1]s\r\n\r\n", ln.Addr())

	if _, err := io.ReadAll(conn); err != nil { // EOF means the proxy closed on its own
		t.Fatalf("proxy did not close the stalled session: %v", err)
	}
}
