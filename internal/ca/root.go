package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

const (
	// rootCommonName and rootOrgName identify our CA in the trust store
	// (docs/01-feature-spec.md F-NET-2). No Caido branding (AGENTS.md rule 3).
	rootCommonName = "Ratatoskr CA"
	rootOrgName    = "Ratatoskr"
	// rootValidity is how long a generated root CA is trusted for.
	rootValidity = 10 * 365 * 24 * time.Hour
	// backdate keeps a fresh certificate valid for clients whose clock runs
	// slightly behind (a common cause of "certificate not yet valid").
	backdate = time.Hour
	// maxSerialBits keeps generated serial numbers positive and short enough
	// for every client.
	maxSerialBits = 128
)

// rootCA is one root certificate with the private key that signs leaves.
type rootCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  []byte
}

// generateRoot creates a new ECDSA P-256 root CA valid from now.
func generateRoot(now time.Time) (*rootCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ca: generate root key: %w", err)
	}

	serial, err := newSerial()
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   rootCommonName,
			Organization: []string{rootOrgName},
		},
		NotBefore:             now.Add(-backdate),
		NotAfter:              now.Add(rootValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		SubjectKeyId:          keyID(&key.PublicKey),
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("ca: create root certificate: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("ca: parse new root certificate: %w", err)
	}

	return &rootCA{cert: cert, key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}, nil
}

// parseRoot turns a stored or imported PEM certificate and PKCS#8 (or SEC1,
// or PKCS#1) private key into a root CA. It rejects anything that cannot sign
// leaves: a non-CA certificate or a key that does not match the certificate.
func parseRoot(certPEM, keyDER []byte) (*rootCA, error) {
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, err
	}

	if !cert.IsCA || !cert.BasicConstraintsValid {
		return nil, fmt.Errorf("%w: certificate is not a CA", ErrInvalidCA)
	}

	// x509.Verify ignores KeyUsage, but browsers and OS trust stores do not: a
	// root without KeyUsageCertSign issues leaves every client rejects.
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, fmt.Errorf("%w: certificate cannot sign certificates (no KeyUsageCertSign)", ErrInvalidCA)
	}

	key, err := parseKeyDER(keyDER)
	if err != nil {
		return nil, err
	}

	if err := checkKeyType(key); err != nil {
		return nil, err
	}

	if !publicKeysEqual(cert, key) {
		return nil, fmt.Errorf("%w: private key does not match the certificate", ErrInvalidCA)
	}

	// Re-encode so CertPEM always returns a single clean block.
	clean := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})

	return &rootCA{cert: cert, key: key, pem: clean}, nil
}

// parseCertPEM decodes the first CERTIFICATE block.
func parseCertPEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%w: no CERTIFICATE PEM block", ErrInvalidCA)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse certificate: %w", ErrInvalidCA, err)
	}

	return cert, nil
}

// parseKeyPEM decodes a private key in PKCS#8, SEC1 (EC) or PKCS#1 (RSA) form.
func parseKeyPEM(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("%w: no private key PEM block", ErrInvalidCA)
	}

	return parseKeyDER(block.Bytes)
}

// parseKeyDER parses a private key in PKCS#8, SEC1 or PKCS#1 form.
func parseKeyDER(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%w: unsupported private key type %T", ErrInvalidCA, key)
		}

		return signer, nil
	}

	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}

	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}

	return nil, fmt.Errorf("%w: unrecognised private key format", ErrInvalidCA)
}

// marshalKeyDER returns the PKCS#8 encoding of key, which every supported key
// type round-trips through.
func marshalKeyDER(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("ca: marshal private key: %w", err)
	}

	return der, nil
}

// marshalKeyPEM renders key as an unencrypted PKCS#8 PEM block.
func marshalKeyPEM(key crypto.Signer) ([]byte, error) {
	der, err := marshalKeyDER(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// publicKeysEqual reports whether key is the private half of cert's public key.
func publicKeysEqual(cert *x509.Certificate, key crypto.Signer) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }

	if eq, ok := cert.PublicKey.(equaler); ok {
		return eq.Equal(key.Public())
	}

	return false
}

// keyID is the RFC 5280 subject key identifier: the SHA-256 of the DER-encoded
// public key, truncated to 20 bytes.
func keyID(pub crypto.PublicKey) []byte {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil
	}

	sum := sha256.Sum256(der)

	return sum[:20]
}

// newSerial returns a random positive 128-bit serial number.
func newSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), maxSerialBits)

	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("ca: generate serial number: %w", err)
	}

	// rand.Int may return zero; serial numbers must be positive.
	return serial.Add(serial, big.NewInt(1)), nil
}

// issuer is an immutable snapshot of the root CA. Leaves signed with it are
// cached under its generation, so replacing the root cannot serve a stale leaf.
type issuer struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  []byte
	gen  uint64
}

// errExpired reports whether cert cannot sign anything valid yet, or any more.
func errExpired(cert *x509.Certificate, now time.Time) error {
	switch {
	case now.After(cert.NotAfter):
		return fmt.Errorf("%w: CA expired on %s", ErrInvalidCA, cert.NotAfter.UTC().Format(time.RFC3339))
	case now.Before(cert.NotBefore):
		return fmt.Errorf("%w: CA is not valid before %s", ErrInvalidCA, cert.NotBefore.UTC().Format(time.RFC3339))
	}

	return nil
}

// checkKeyType reports whether browsers accept certificates signed with key.
// Go can sign with Ed25519, but no major browser or OS trust store verifies
// Ed25519 certificate signatures, so such a root would produce leaves that fail
// on every client.
func checkKeyType(key crypto.Signer) error {
	switch key.Public().(type) {
	case *ecdsa.PublicKey, *rsa.PublicKey:
		return nil
	case ed25519.PublicKey:
		return fmt.Errorf("%w: Ed25519 CA keys are not accepted by browsers; use ECDSA P-256 or RSA", ErrInvalidCA)
	}

	return fmt.Errorf("%w: unsupported private key type %T", ErrInvalidCA, key.Public())
}
