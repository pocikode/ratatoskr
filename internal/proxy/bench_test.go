package proxy_test

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// BenchmarkProxyGET measures one keep-alive GET through the proxy with no
// Intercept or Match & Replace configured.
func BenchmarkProxyGET(b *testing.B) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	b.Cleanup(up.Close)
	r := startProxy(b)

	conn, err := net.Dial("tcp", r.addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	host := up.Listener.Addr().String()
	req := []byte("GET http://" + host + "/bench HTTP/1.1\r\nHost: " + host + "\r\n\r\n")
	br := bufio.NewReader(conn)

	for b.Loop() {
		if _, err := conn.Write(req); err != nil {
			b.Fatal(err)
		}

		resp, err := rawhttp.ReadResponse(br, "GET", rawhttp.Limits{})
		if err != nil {
			b.Fatal(err)
		}

		if _, err := io.Copy(io.Discard, resp.Body()); err != nil {
			b.Fatal(err)
		}
	}
}
