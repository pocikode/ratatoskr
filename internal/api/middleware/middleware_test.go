package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocikode/ratatoskr/internal/api/middleware"
)

// okHandler answers 204 and reports whether the request was authenticated.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if middleware.Authenticated(r.Context()) {
		w.Header().Set("X-Authenticated", "1")
	}

	w.WriteHeader(http.StatusNoContent)
})

func TestRequireAPIRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		header      string
		want        int
	}{
		{"valid", "application/json", "1", http.StatusNoContent},
		{"valid with charset", "application/json; charset=utf-8", "1", http.StatusNoContent},
		{"missing header", "application/json", "", http.StatusForbidden},
		{"wrong header value", "application/json", "true", http.StatusForbidden},
		{"form post", "application/x-www-form-urlencoded", "1", http.StatusUnsupportedMediaType},
		{"text plain", "text/plain", "1", http.StatusUnsupportedMediaType},
		{"no content type", "", "1", http.StatusUnsupportedMediaType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader("{}"))
			if tt.contentType != "" {
				r.Header.Set("Content-Type", tt.contentType)
			}

			if tt.header != "" {
				r.Header.Set(middleware.APIHeader, tt.header)
			}

			rec := httptest.NewRecorder()
			middleware.RequireAPIRequest(okHandler).ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestCrossOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		headers map[string]string
		want    int
	}{
		{"same origin", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusNoContent},
		{"cross site post", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"foreign origin", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"non-browser client", http.MethodPost, nil, http.StatusNoContent},
		{"cross site get is safe", http.MethodGet, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(tt.method, "http://127.0.0.1:8080/api/graphql", nil)
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			middleware.CrossOrigin(okHandler).ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestAuth(t *testing.T) {
	t.Parallel()

	const token = "s3cret"

	tests := []struct {
		name    string
		auth    middleware.Auth
		remote  string
		headers map[string]string
		cookie  string
		// ws selects Auth.WebSocket instead of Auth.Handler.
		ws       bool
		want     int
		wantAuth bool
	}{
		{name: "auth off", want: http.StatusNoContent, wantAuth: true},
		{name: "no token", auth: middleware.Auth{Token: token}, want: http.StatusUnauthorized},
		{
			name: "bearer", auth: middleware.Auth{Token: token},
			headers: map[string]string{"Authorization": "Bearer " + token}, want: http.StatusNoContent, wantAuth: true,
		},
		{
			name: "wrong bearer", auth: middleware.Auth{Token: token},
			headers: map[string]string{"Authorization": "Bearer nope"}, want: http.StatusUnauthorized,
		},
		{name: "cookie", auth: middleware.Auth{Token: token}, cookie: token, want: http.StatusNoContent, wantAuth: true},
		{name: "wrong cookie", auth: middleware.Auth{Token: token}, cookie: "s3cre", want: http.StatusUnauthorized},
		{
			name: "guest on loopback", auth: middleware.Auth{Token: token, AllowGuests: true},
			remote: "127.0.0.1:5555", want: http.StatusNoContent, wantAuth: true,
		},
		{
			name: "guest on ipv6 loopback", auth: middleware.Auth{Token: token, AllowGuests: true},
			remote: "[::1]:5555", want: http.StatusNoContent, wantAuth: true,
		},
		{
			name: "guest from the network", auth: middleware.Auth{Token: token, AllowGuests: true},
			remote: "192.168.1.20:5555", want: http.StatusUnauthorized,
		},
		{
			name: "guests not allowed", auth: middleware.Auth{Token: token},
			remote: "127.0.0.1:5555", want: http.StatusUnauthorized,
		},
		{
			name: "bearer scheme is case-insensitive", auth: middleware.Auth{Token: token},
			headers: map[string]string{"Authorization": "bearer " + token}, want: http.StatusNoContent, wantAuth: true,
		},
		{
			name: "token without scheme", auth: middleware.Auth{Token: token},
			headers: map[string]string{"Authorization": token}, want: http.StatusUnauthorized,
		},
		{
			// An Upgrade header must not open other routes (T-007 review).
			name: "websocket upgrade on a strict route", auth: middleware.Auth{Token: token},
			headers: map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"}, want: http.StatusUnauthorized,
		},
		{
			// The token may still arrive in connection_init.
			name: "websocket without credentials", auth: middleware.Auth{Token: token}, ws: true,
			headers: map[string]string{"Upgrade": "websocket", "Connection": "Upgrade"}, want: http.StatusNoContent,
		},
		{
			name: "websocket with bearer", auth: middleware.Auth{Token: token}, ws: true,
			headers: map[string]string{
				"Upgrade": "websocket", "Connection": "Upgrade", "Authorization": "Bearer " + token,
			},
			want: http.StatusNoContent, wantAuth: true,
		},
		{
			name: "websocket route without upgrade", auth: middleware.Auth{Token: token}, ws: true,
			want: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/api/graphql/ws", nil)
			r.RemoteAddr = "192.0.2.1:1234"

			if tt.remote != "" {
				r.RemoteAddr = tt.remote
			}

			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}

			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: middleware.TokenCookie, Value: tt.cookie})
			}

			rec := httptest.NewRecorder()
			h := tt.auth.Handler
			if tt.ws {
				h = tt.auth.WebSocket
			}

			h(okHandler).ServeHTTP(rec, r)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}

			if got := rec.Header().Get("X-Authenticated") == "1"; got != tt.wantAuth {
				t.Errorf("Authenticated = %v, want %v", got, tt.wantAuth)
			}

			if tt.want == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("401 without WWW-Authenticate")
			}
		})
	}
}
