package static_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/pocikode/ratatoskr/internal/api/static"
)

func newFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>app</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
		"favicon.svg":   {Data: []byte("<svg/>")},
	}
}

func get(t *testing.T, h http.Handler, method, target string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, string(body), res.Header
}

func TestHandler(t *testing.T) {
	h := static.NewHandler(newFS())
	const index = "<!doctype html><title>app</title>"

	tests := []struct {
		name, method, target string
		status               int
		body                 string // exact body when non-empty
	}{
		{"root serves index", "GET", "/", 200, index},
		{"explicit index.html", "GET", "/index.html", 200, index},
		{"client route falls back", "GET", "/replay/session/42", 200, index},
		{"static file", "GET", "/favicon.svg", 200, "<svg/>"},
		{"hashed asset", "GET", "/assets/app.js", 200, "console.log(1)"},
		{"missing asset is 404", "GET", "/assets/missing.js", 404, ""},
		{"api path is not swallowed", "GET", "/api/graphql", 404, ""},
		{"api root is not swallowed", "GET", "/api", 404, ""},
		{"traversal is cleaned", "GET", "/api/../x", 200, index},
		{"HEAD ok", "HEAD", "/", 200, ""},
		{"POST rejected", "POST", "/", 405, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, body, _ := get(t, h, tc.method, tc.target)
			if status != tc.status {
				t.Fatalf("status = %d, want %d", status, tc.status)
			}
			if tc.body != "" && body != tc.body {
				t.Fatalf("body = %q, want %q", body, tc.body)
			}
		})
	}
}

func TestCacheHeaders(t *testing.T) {
	h := static.NewHandler(newFS())
	if _, _, hdr := get(t, h, "GET", "/"); hdr.Get("Cache-Control") != "no-cache" {
		t.Errorf("index Cache-Control = %q, want no-cache", hdr.Get("Cache-Control"))
	}
	if _, _, hdr := get(t, h, "GET", "/assets/app.js"); hdr.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("asset Cache-Control = %q", hdr.Get("Cache-Control"))
	}
}

func TestUnbuiltUI(t *testing.T) {
	status, _, _ := get(t, static.NewHandler(fstest.MapFS{}), "GET", "/")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", status)
	}
}

func TestEmbeddedHandlerRoutes(t *testing.T) {
	// Whether or not web/dist has been built, /api must still 404 and the handler must not panic.
	status, _, _ := get(t, static.Handler(), "GET", "/api/x")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}
