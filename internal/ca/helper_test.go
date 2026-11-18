package ca_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/ca"
	"github.com/pocikode/ratatoskr/internal/store"
)

// env is one data dir (secret.key, instance.db) the CA lives in.
type env struct {
	dir  string
	root *os.Root
	inst *store.DB
}

func newEnv(t testing.TB) *env {
	t.Helper()

	dir := t.TempDir()

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	inst, err := store.OpenInstance(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = inst.Close() })

	return &env{dir: dir, root: root, inst: inst}
}

// open returns a service over the env, generating the root CA on first use.
func (e *env) open(t testing.TB, opts ...func(*ca.Deps)) *ca.Service {
	t.Helper()

	deps := ca.Deps{
		Instance: e.inst,
		Root:     e.root,
		Logger:   slog.New(slog.DiscardHandler),
	}

	for _, opt := range opts {
		opt(&deps)
	}

	svc, err := ca.Open(t.Context(), deps)
	if err != nil {
		t.Fatal(err)
	}

	return svc
}

// roots returns a pool with the service's current root only.
func roots(svc *ca.Service) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(svc.CertPEM())

	return pool
}

// verifyChain checks that leaf chains to a root in pool (the host is checked
// separately).
func verifyChain(t testing.TB, leaf *tls.Certificate, pool *x509.CertPool) {
	t.Helper()

	if _, err := leaf.Leaf.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("leaf does not verify against the root: %v", err)
	}
}

// verifyFails is verifyChain's negative: the leaf must not chain to pool.
func verifyFails(t testing.TB, leaf *tls.Certificate, pool *x509.CertPool) {
	t.Helper()

	if _, err := leaf.Leaf.Verify(x509.VerifyOptions{Roots: pool}); err == nil {
		t.Fatal("leaf verified against a root it was not signed by")
	}
}

// pemBytes renders DER as a CERTIFICATE PEM block.
func pemBytes(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// keyPEMBytes renders a private key as an unencrypted PKCS#8 PEM block.
func keyPEMBytes(t testing.TB, key any) []byte {
	t.Helper()

	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// leafPEM returns the leaf certificate (not the chain) the service issues for
// host, as PEM: an import candidate that is not a CA.
func leafPEM(t testing.TB, svc *ca.Service, host string) []byte {
	t.Helper()

	cert, err := svc.Certificate(host)
	if err != nil {
		t.Fatal(err)
	}

	return pemBytes(cert.Certificate[0])
}

// leafKeyPEM returns the private key that belongs to leafPEM.
func leafKeyPEM(t testing.TB, svc *ca.Service, host string) []byte {
	t.Helper()

	cert, err := svc.Certificate(host)
	if err != nil {
		t.Fatal(err)
	}

	return keyPEMBytes(t, cert.PrivateKey)
}

// expiredCA returns a self-signed CA that expired yesterday, with its key.
func expiredCA(t testing.TB) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Expired Test CA"},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return pemBytes(der), keyPEMBytes(t, key)
}

// noCertSignCA returns a self-signed CA that is marked as a CA but may not sign
// certificates: browsers reject its leaves, so Import must reject it too.
func noCertSignCA(t testing.TB) (certPEM, keyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Not A Signing Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return pemBytes(der), keyPEMBytes(t, key)
}

// ed25519CA returns a self-signed Ed25519 CA: Go can sign with it, browsers and
// OS trust stores reject every certificate that chains to it.
func ed25519CA(t testing.TB) (certPEM, keyPEM []byte) {
	t.Helper()

	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "Ed25519 Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}

	return pemBytes(der), keyPEMBytes(t, key)
}

// tcpPipe is an in-memory connection pair for a real TLS handshake.
func tcpPipe(t testing.TB) (client, server net.Conn) {
	t.Helper()

	client, server = net.Pipe()

	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	return client, server
}
