package ca_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/ca"
)

func TestOpenGeneratesRoot(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	block, _ := pem.Decode(svc.CertPEM())
	if block == nil {
		t.Fatal("CertPEM is not a PEM block")
	}

	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if !root.IsCA || !root.BasicConstraintsValid {
		t.Error("root is not a CA")
	}

	if root.Subject.CommonName != "Ratatoskr CA" {
		t.Errorf("CommonName = %q, want %q", root.Subject.CommonName, "Ratatoskr CA")
	}

	if _, ok := root.PublicKey.(*ecdsa.PublicKey); !ok {
		t.Fatalf("root key is %T, want ECDSA (F-NET-2)", root.PublicKey)
	}

	if curve := root.PublicKey.(*ecdsa.PublicKey).Curve; curve != elliptic.P256() {
		t.Errorf("root curve = %v, want P-256", curve)
	}

	if got := root.NotAfter.Sub(root.NotBefore); got < 9*365*24*time.Hour {
		t.Errorf("root validity = %v, want ~10 years", got)
	}

	if !bytes.Equal(svc.CertDER(), root.Raw) {
		t.Error("CertDER does not match CertPEM")
	}

	if svc.Generation() != 1 {
		t.Errorf("Generation() = %d, want 1", svc.Generation())
	}
}

func TestOpenReusesStoredRoot(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	first := e.open(t)
	second := e.open(t)

	if !bytes.Equal(first.CertPEM(), second.CertPEM()) {
		t.Fatal("reopening the data dir generated a second root CA")
	}

	if first.Generation() != second.Generation() {
		t.Errorf("generations differ: %d vs %d", first.Generation(), second.Generation())
	}
}

func TestSecretKeyFile(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.open(t)

	path := filepath.Join(e.dir, "secret.key")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secret.key: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secret.key mode = %o, want 600", perm)
	}

	if info.Size() != 32 {
		t.Errorf("secret.key size = %d, want 32", info.Size())
	}
}

func TestSecretKeyPermissionsAreTightened(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	if err := os.WriteFile(filepath.Join(e.dir, "secret.key"), make([]byte, 32), 0o644); err != nil {
		t.Fatal(err)
	}

	e.open(t)

	info, err := os.Stat(filepath.Join(e.dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}

	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("secret.key mode = %o, want it tightened to 600", perm)
	}
}

func TestStoredKeyIsSealed(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.open(t)

	var certPEM, keyEnc []byte

	err := e.inst.View(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT cert_pem, key_pem_enc FROM ca WHERE id = 1`).
			Scan(&certPEM, &keyEnc)
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(certPEM) == 0 || len(keyEnc) == 0 {
		t.Fatal("ca row is empty")
	}

	// The sealed blob must not contain a PEM private key (docs/03-architecture.md §8).
	if bytes.Contains(keyEnc, []byte("PRIVATE KEY")) {
		t.Fatal("key_pem_enc holds a plaintext PEM key")
	}

	if block, _ := pem.Decode(keyEnc); block != nil {
		t.Fatal("key_pem_enc decodes as PEM")
	}
}

func TestOpenRequiresDeps(t *testing.T) {
	t.Parallel()

	if _, err := ca.Open(t.Context(), ca.Deps{}); err == nil {
		t.Fatal("Open without deps succeeded")
	}
}

func TestRegenerateReplacesRootAndDropsLeaves(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	oldRoot := roots(svc)
	oldPEM := svc.CertPEM()

	before, err := svc.Certificate("regenerate.example")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Regenerate(t.Context()); err != nil {
		t.Fatal(err)
	}

	if svc.Generation() != 2 {
		t.Errorf("Generation() = %d, want 2", svc.Generation())
	}

	if bytes.Equal(svc.CertPEM(), oldPEM) {
		t.Fatal("Regenerate kept the old root")
	}

	after, err := svc.Certificate("regenerate.example")
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(before.Leaf.Raw, after.Leaf.Raw) {
		t.Fatal("Regenerate did not invalidate the cached leaf")
	}

	newRoot := roots(svc)

	verifyChain(t, after, newRoot)
	verifyFails(t, before, newRoot)
	verifyChain(t, before, oldRoot)
}

func TestExportImport(t *testing.T) {
	t.Parallel()

	source := newEnv(t)
	src := source.open(t)

	certPEM, keyPEM, err := src.Export()
	if err != nil {
		t.Fatal(err)
	}

	if svcPEM := src.CertPEM(); !bytes.Equal(certPEM, svcPEM) {
		t.Error("Export certificate differs from CertPEM")
	}

	if block, _ := pem.Decode(keyPEM); block == nil || block.Type != "PRIVATE KEY" {
		t.Fatalf("exported key is not a PRIVATE KEY PEM block: %v", block)
	}

	target := newEnv(t)
	dst := target.open(t)

	if err := dst.Import(t.Context(), certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(dst.CertPEM(), certPEM) {
		t.Fatal("imported root certificate differs from the exported one")
	}

	if dst.Generation() != 2 {
		t.Errorf("Generation() = %d after import, want 2", dst.Generation())
	}

	// Leaves issued after an import chain to the imported root.
	leaf, err := dst.Certificate("imported.example")
	if err != nil {
		t.Fatal(err)
	}

	verifyChain(t, leaf, roots(dst))

	// The imported root survives a restart.
	reopened := target.open(t)
	if !bytes.Equal(reopened.CertPEM(), certPEM) {
		t.Fatal("imported root was not persisted")
	}
}

func TestImportRejectsBadMaterial(t *testing.T) {
	t.Parallel()

	source := newEnv(t)
	src := source.open(t)

	certPEM, keyPEM, err := src.Export()
	if err != nil {
		t.Fatal(err)
	}

	// A key from an unrelated CA: valid PEM, wrong certificate.
	_, foreignKey, err := newEnv(t).open(t).Export()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		certPEM []byte
		keyPEM  []byte
	}{
		{name: "no certificate", certPEM: []byte("not a certificate"), keyPEM: keyPEM},
		{name: "no key", certPEM: certPEM, keyPEM: []byte("not a key")},
		{name: "mismatched key", certPEM: certPEM, keyPEM: foreignKey},
		{name: "non-CA certificate", certPEM: leafPEM(t, src, "leaf.example"), keyPEM: leafKeyPEM(t, src, "leaf.example")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := newEnv(t)
			svc := e.open(t)

			err := svc.Import(t.Context(), tc.certPEM, tc.keyPEM)
			if !errors.Is(err, ca.ErrInvalidCA) {
				t.Fatalf("Import() error = %v, want ErrInvalidCA", err)
			}

			if svc.Generation() != 1 {
				t.Errorf("Generation() = %d after a failed import, want 1", svc.Generation())
			}
		})
	}
}

func TestImportRejectsExpiredCA(t *testing.T) {
	t.Parallel()

	certPEM, keyPEM := expiredCA(t)

	e := newEnv(t)
	svc := e.open(t)

	if err := svc.Import(t.Context(), certPEM, keyPEM); !errors.Is(err, ca.ErrInvalidCA) {
		t.Fatalf("Import() error = %v, want ErrInvalidCA", err)
	}
}

func TestExportRoundTripsThroughImport(t *testing.T) {
	t.Parallel()

	source := newEnv(t)
	src := source.open(t)

	certPEM, keyPEM, err := src.Export()
	if err != nil {
		t.Fatal(err)
	}

	// Import our own material back: same root, one more generation.
	if err := src.Import(t.Context(), certPEM, keyPEM); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(src.CertPEM(), certPEM) {
		t.Fatal("re-importing our own certificate changed it")
	}

	if src.Generation() != 2 {
		t.Errorf("Generation() = %d, want 2", src.Generation())
	}
}

// TestTLSConfigHandshake drives a real handshake through TLSConfig: SNI selects
// the leaf, the client verifies it against the root and ALPN picks HTTP/1.1.
func TestTLSConfigHandshake(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(svc.CertPEM())

	clientConn, serverConn := tcpPipe(t)

	server := tls.Server(serverConn, svc.TLSConfig())
	client := tls.Client(clientConn, &tls.Config{
		ServerName: "handshake.example",
		RootCAs:    roots,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
	})

	errCh := make(chan error, 1)

	go func() { errCh <- server.HandshakeContext(t.Context()) }()

	if err := client.HandshakeContext(t.Context()); err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server handshake: %v", err)
	}

	state := client.ConnectionState()
	if state.NegotiatedProtocol != "http/1.1" {
		t.Errorf("ALPN = %q, want http/1.1 (v1.0 speaks HTTP/1.1 only)", state.NegotiatedProtocol)
	}

	if len(state.VerifiedChains) == 0 {
		t.Fatal("client did not verify the leaf against the root")
	}

	if state.Version < tls.VersionTLS12 {
		t.Errorf("TLS version = %#x, want >= TLS 1.2", state.Version)
	}

	if got := state.ServerName; got != "handshake.example" {
		t.Errorf("SNI = %q", got)
	}
}

func TestGetCertificateNeedsSNI(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	if _, err := svc.GetCertificate(nil); !errors.Is(err, ca.ErrNoServerName) {
		t.Fatalf("GetCertificate(nil) error = %v, want ErrNoServerName", err)
	}

	if _, err := svc.GetCertificate(&tls.ClientHelloInfo{}); !errors.Is(err, ca.ErrNoServerName) {
		t.Fatalf("GetCertificate(no SNI) error = %v, want ErrNoServerName", err)
	}
}

func TestMissingSecretKeyFailsLoudly(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	e.open(t)

	// A different secret.key cannot open the stored root key; the operator must
	// regenerate rather than silently trust a fresh CA.
	if err := os.WriteFile(filepath.Join(e.dir, "secret.key"), bytes.Repeat([]byte{7}, 32), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ca.Open(t.Context(), ca.Deps{Instance: e.inst, Root: e.root}); err == nil {
		t.Fatal("Open with a foreign secret.key succeeded")
	}
}
