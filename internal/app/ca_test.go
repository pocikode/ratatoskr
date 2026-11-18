package app

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"testing"

	"github.com/pocikode/ratatoskr/internal/config"
)

// TestCADownloadsAtOrigin covers F-NET-2's http://ratatoskr.localhost/ca.crt
// through a real listener. The certificate is public (it holds no key material),
// so it must be served without --auth-token credentials, and the SPA catch-all
// must not shadow it.
func TestCADownloadsAtOrigin(t *testing.T) {
	t.Parallel()

	a, err := New(t.Context(), Options{Config: config.Config{
		Listen:    []string{"127.0.0.1:0"},
		DataDir:   dataDir(t),
		AuthToken: "s3cret",
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = a.Close() })

	ln := listen(t)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	serveErr := make(chan error, 1)
	go func() { serveErr <- a.Serve(ctx, ln) }()

	base := "http://" + ln.Addr().String()

	pemBody := getOK(t, base+"/ca.crt")

	block, _ := pem.Decode(pemBody)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("GET /ca.crt is not a CERTIFICATE PEM block: %q", pemBody)
	}

	fromPEM, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse /ca.crt: %v", err)
	}

	if !fromPEM.IsCA {
		t.Error("/ca.crt is not a CA certificate")
	}

	derBody := getOK(t, base+"/ca.der")

	fromDER, err := x509.ParseCertificate(derBody)
	if err != nil {
		t.Fatalf("GET /ca.der is not a DER certificate: %v", err)
	}

	if !bytes.Equal(fromDER.Raw, fromPEM.Raw) {
		t.Error("/ca.crt and /ca.der serve different certificates")
	}

	cancel()

	if err := <-serveErr; err != nil {
		t.Fatalf("Serve() = %v, want nil", err)
	}
}

// getOK performs a GET without credentials and returns a 200 body.
func getOK(t *testing.T, url string) []byte {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200 (body %q)", url, resp.StatusCode, body)
	}

	return body
}
