package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocikode/ratatoskr/internal/api/middleware"
)

func TestHosts(t *testing.T) {
	t.Parallel()

	// The allowlist app.New builds for a loopback listener.
	listen := []string{"127.0.0.1:8080", "ratatoskr.localhost"}

	tests := []struct {
		name    string
		allowed []string
		host    string
		want    int
	}{
		{name: "check off without a loopback listener", host: "evil.example:8080", want: http.StatusNoContent},
		{name: "localhost", allowed: listen, host: "localhost:8080", want: http.StatusNoContent},
		{name: "localhost without a port", allowed: listen, host: "localhost", want: http.StatusNoContent},
		{name: "localhost is case-insensitive", allowed: listen, host: "LOCALHOST:8080", want: http.StatusNoContent},
		{name: "ipv4 loopback", allowed: listen, host: "127.0.0.1:8080", want: http.StatusNoContent},
		{name: "ipv4 loopback range", allowed: listen, host: "127.0.0.2:8080", want: http.StatusNoContent},
		{name: "ipv6 loopback", allowed: listen, host: "[::1]:8080", want: http.StatusNoContent},
		{name: "mapped ipv4 loopback", allowed: listen, host: "[::ffff:127.0.0.1]:8080", want: http.StatusNoContent},
		{name: "configured listen host", allowed: listen, host: "127.0.0.1:9090", want: http.StatusNoContent},
		{name: "tool hostname", allowed: listen, host: "ratatoskr.localhost:8080", want: http.StatusNoContent},
		{
			// A hostname listen address is allowed verbatim.
			name: "listen hostname", allowed: []string{"dev.example:8080"}, host: "dev.example:8080",
			want: http.StatusNoContent,
		},
		{name: "rebinding domain", allowed: listen, host: "evil.example:8080", want: http.StatusMisdirectedRequest},
		{name: "loopback prefix is not loopback", allowed: listen, host: "127.0.0.1.evil.example:8080",
			want: http.StatusMisdirectedRequest},
		{name: "localhost suffix is not localhost", allowed: listen, host: "localhost.evil.example:8080",
			want: http.StatusMisdirectedRequest},
		{name: "missing host", allowed: listen, host: "", want: http.StatusMisdirectedRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/api/graphql", nil)
			r.Host = tt.host

			rec := httptest.NewRecorder()
			middleware.Hosts(tt.allowed, okHandler).ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Errorf("Host %q: status = %d, want %d", tt.host, rec.Code, tt.want)
			}
		})
	}
}
