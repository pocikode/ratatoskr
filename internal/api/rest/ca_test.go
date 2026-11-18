package rest_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocikode/ratatoskr/internal/api/rest"
)

// stubCerts is a CA with fixed bytes, so the routes can be tested without a real
// certificate authority.
type stubCerts struct {
	pem []byte
	der []byte
}

func (s stubCerts) CertPEM() []byte { return s.pem }
func (s stubCerts) CertDER() []byte { return s.der }

func TestCADownload(t *testing.T) {
	t.Parallel()

	certs := stubCerts{
		pem: []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		der: []byte{0x30, 0x82, 0x01, 0x00},
	}

	srv := httptest.NewServer(rest.NewRouter(rest.Deps{CA: certs}))
	t.Cleanup(srv.Close)

	tests := []struct {
		path        string
		contentType string
		filename    string
		body        []byte
	}{
		{path: "/api/rest/ca.crt", contentType: "application/x-pem-file", filename: "ratatoskr-ca.crt", body: certs.pem},
		{path: "/api/rest/ca.der", contentType: "application/pkix-cert", filename: "ratatoskr-ca.der", body: certs.der},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			resp, err := http.Get(srv.URL + tc.path) //nolint:noctx // test request
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			if got := resp.Header.Get("Content-Type"); got != tc.contentType {
				t.Errorf("Content-Type = %q, want %q", got, tc.contentType)
			}

			if got, want := resp.Header.Get("Content-Disposition"), `attachment; filename="`+tc.filename+`"`; got != want {
				t.Errorf("Content-Disposition = %q, want %q", got, want)
			}

			// A regenerated CA must not be masked by a cached download.
			if got := resp.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}

			if string(body) != string(tc.body) {
				t.Errorf("body = %q, want %q", body, tc.body)
			}
		})
	}
}

func TestCAHead(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(rest.NewRouter(rest.Deps{CA: stubCerts{pem: []byte("pem"), der: []byte("der")}}))
	t.Cleanup(srv.Close)

	resp, err := http.Head(srv.URL + "/api/rest/ca.crt") //nolint:noctx // test request
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if got := resp.Header.Get("Content-Length"); got != "3" {
		t.Errorf("Content-Length = %q, want 3", got)
	}
}

func TestCAPostNotAllowed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(rest.NewRouter(rest.Deps{CA: stubCerts{}}))
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/rest/ca.crt", "text/plain", http.NoBody) //nolint:noctx // test request
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// The GET patterns do not match, and the catch-all router answers 404, so a
	// write cannot reach the download.
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 404 or 405", resp.StatusCode)
	}
}

func TestCARoutesAbsentWithoutService(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(rest.NewRouter(rest.Deps{}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/rest/ca.crt") //nolint:noctx // test request
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestCAHandlerAtOrigin covers the bare-origin download of F-NET-2
// (http://ratatoskr.localhost/ca.crt).
func TestCAHandlerAtOrigin(t *testing.T) {
	t.Parallel()

	certs := stubCerts{pem: []byte("pem"), der: []byte("der")}
	files := rest.CAHandler(certs)

	mux := http.NewServeMux()
	mux.Handle("GET /ca.crt", files)
	mux.Handle("GET /ca.der", files)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, tc := range []struct{ path, want string }{
		{path: "/ca.crt", want: "pem"},
		{path: "/ca.der", want: "der"},
	} {
		resp, err := http.Get(srv.URL + tc.path) //nolint:noctx // test request
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.path, resp.StatusCode)
		}

		if string(body) != tc.want {
			t.Errorf("%s: body = %q, want %q", tc.path, body, tc.want)
		}
	}
}
