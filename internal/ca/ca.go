// Package ca owns the interception certificate authority: the root CA stored in
// instance.db, the per-host leaf certificates the proxy presents, and
// import/export/regenerate for the Settings → Certificate page
// (docs/01-feature-spec.md F-NET-2, docs/03-architecture.md §5.1, §8).
//
// The root private key is sealed with XChaCha20-Poly1305 under secret.key
// before it reaches the database, so a copy of instance.db alone cannot be used
// to intercept anyone (docs/03-architecture.md §8).
package ca

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"

	"github.com/pocikode/ratatoskr/internal/secret"
	"github.com/pocikode/ratatoskr/internal/store"
)

const (
	// cacheSize is the leaf cache size: 1024 hosts (F-NET-2).
	cacheSize = 1024
	// keyAAD binds a sealed root key to this use, so a blob copied from another
	// secret column does not open here.
	keyAAD = "ratatoskr/ca/root-key/v1"
	// maxIssuerRetries bounds how often a leaf is re-signed when the root CA is
	// replaced mid-issuance.
	maxIssuerRetries = 3
)

var (
	// ErrNoServerName is returned when a certificate is requested for an empty
	// host: the TLS client sent no SNI and the caller had no CONNECT fallback.
	ErrNoServerName = errors.New("ca: missing server name")
	// ErrInvalidHost is returned for a host that cannot go into a certificate.
	ErrInvalidHost = errors.New("ca: invalid host")
	// ErrInvalidCA is returned by [Service.Import] for material that is not a
	// usable CA certificate and key pair.
	ErrInvalidCA = errors.New("ca: invalid CA")
	// ErrRootChanged is returned when the root CA was replaced repeatedly while a
	// leaf was being issued. The handshake fails and the client retries; it never
	// receives a leaf that does not chain to the served root.
	ErrRootChanged = errors.New("ca: root CA changed while issuing a certificate")
)

// Deps are the inputs to [Open].
type Deps struct {
	// Instance is the open instance.db (its ca row holds the root CA).
	Instance *store.DB
	// Root is the data dir; secret.key lives in it.
	Root *os.Root
	// Logger defaults to [slog.Default].
	Logger *slog.Logger
	// Now defaults to [time.Now].
	Now func() time.Time
}

// Service issues leaf certificates from the root CA and manages that root.
// It is safe for concurrent use.
type Service struct {
	inst *store.DB
	seal *secret.Box
	log  *slog.Logger
	now  func() time.Time

	// cur is the current root CA. Issuance reads it once and caches leaves
	// under its generation, so a replacement cannot leak an old-root leaf into
	// a new-root cache.
	cur atomic.Pointer[issuer]
	// mu serializes Regenerate and Import.
	mu sync.Mutex

	// cache holds issued leaves. lru.Cache is safe for concurrent use, so the
	// hot handshake path takes only its internal lock.
	cache *lru.Cache[string, *tls.Certificate]
	group singleflight.Group
}

// Open loads the root CA from instance.db, generating and storing one on first
// start. ctx only carries values: a cancelled ctx does not abort the open.
func Open(ctx context.Context, deps Deps) (*Service, error) {
	if deps.Instance == nil || deps.Root == nil {
		return nil, errors.New("ca: Deps.Instance and Deps.Root are required")
	}

	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	now := deps.Now
	if now == nil {
		now = time.Now
	}

	seal, loose, err := secret.Open(deps.Root)
	if err != nil {
		return nil, err
	}

	if loose {
		logger.Warn("secret.key was readable by other users; permissions restored to 0600",
			"file", "secret.key")
	}

	cache, err := lru.New[string, *tls.Certificate](cacheSize)
	if err != nil {
		return nil, fmt.Errorf("ca: create leaf cache: %w", err)
	}

	s := &Service{
		inst:  deps.Instance,
		seal:  seal,
		log:   logger,
		now:   now,
		cache: cache,
	}

	openCtx := context.WithoutCancel(ctx)

	root, err := s.loadOrCreate(openCtx)
	if err != nil {
		return nil, err
	}

	if err := errExpired(root.cert, now()); err != nil {
		logger.Warn("root CA is not usable; regenerate it from Settings → Certificate", "error", err)
	}

	s.cur.Store(&issuer{cert: root.cert, key: root.key, pem: root.pem, gen: 1})

	return s, nil
}

// loadOrCreate returns the stored root CA, or generates and stores a new one.
func (s *Service) loadOrCreate(ctx context.Context) (*rootCA, error) {
	certPEM, keyEnc, err := loadRow(ctx, s.inst)
	switch {
	case errors.Is(err, errNoCA):
		return s.create(ctx)
	case err != nil:
		return nil, err
	}

	keyDER, err := s.seal.Open(keyEnc, []byte(keyAAD))
	if err != nil {
		return nil, fmt.Errorf("ca: cannot open the stored root CA key: secret.key no longer matches instance.db; "+
			"restore the original secret.key, or remove the ca row from instance.db to generate a new root CA "+
			"(every client must then re-trust the new one): %w", err)
	}

	root, err := parseRoot(certPEM, keyDER)
	if err != nil {
		return nil, fmt.Errorf("ca: load stored root: %w", err)
	}

	return root, nil
}

// create generates a root CA and stores it.
func (s *Service) create(ctx context.Context) (*rootCA, error) {
	root, err := generateRoot(s.now())
	if err != nil {
		return nil, err
	}

	if err := s.save(ctx, root); err != nil {
		return nil, err
	}

	s.log.Info("generated root CA", "subject", root.cert.Subject.CommonName,
		"not_after", root.cert.NotAfter.UTC().Format(time.RFC3339))

	return root, nil
}

// save seals root's key and writes the ca row.
func (s *Service) save(ctx context.Context, root *rootCA) error {
	keyDER, err := marshalKeyDER(root.key)
	if err != nil {
		return err
	}

	sealed := s.seal.Seal(keyDER, []byte(keyAAD))

	if err := saveRow(ctx, s.inst, root.pem, sealed); err != nil {
		return err
	}

	return nil
}

// Certificate returns the leaf for host, issuing it on first use. host may carry
// a port and brackets ([::1]:443); it is normalised first. Concurrent calls for
// the same host share one issuance.
func (s *Service) Certificate(host string) (*tls.Certificate, error) {
	name, err := normalizeHost(host)
	if err != nil {
		return nil, err
	}

	// A Regenerate or Import between the snapshot and the signature would hand
	// out a leaf the client cannot verify against the served root; retry with the
	// new root instead.
	for range maxIssuerRetries {
		iss := s.cur.Load()

		key := cacheKey(iss.gen, name)
		if cert, ok := s.cached(key); ok {
			return cert, nil
		}

		cert, err := s.issueOnce(key, iss, name)
		if err != nil {
			return nil, err
		}

		if s.cur.Load() == iss {
			return cert, nil
		}
	}

	return nil, fmt.Errorf("%w after %d attempts", ErrRootChanged, maxIssuerRetries)
}

// issueOnce issues name with iss, collapsing concurrent callers into one
// signature and caching the result under key.
func (s *Service) issueOnce(key string, iss *issuer, name string) (*tls.Certificate, error) {
	issued, err, _ := s.group.Do(key, func() (any, error) {
		if cert, ok := s.cached(key); ok {
			return cert, nil
		}

		cert, err := s.issue(iss, name)
		if err != nil {
			return nil, err
		}

		// A root replaced while we were signing makes this leaf unusable; do not
		// cache it under the new generation. A concurrent Purge can only drop
		// this entry, which the next request re-issues.
		if s.cur.Load() == iss {
			s.cache.Add(key, cert)
		}

		return cert, nil
	})
	if err != nil {
		return nil, err
	}

	cert, ok := issued.(*tls.Certificate)
	if !ok {
		return nil, fmt.Errorf("ca: unexpected cached certificate type %T", issued)
	}

	return cert, nil
}

// GetCertificate is a [tls.Config.GetCertificate] callback: it issues a leaf for
// the SNI of the handshake. An SNI-less handshake fails; the proxy retries with
// the CONNECT host through [Service.Certificate].
func (s *Service) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	if hello == nil || hello.ServerName == "" {
		return nil, ErrNoServerName
	}

	return s.Certificate(hello.ServerName)
}

// TLSConfig returns the server config for the MITM listener: our leaves, TLS 1.2
// minimum and HTTP/1.1 as the only ALPN protocol (F-NET-2).
func (s *Service) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"http/1.1"},
	}
}

// CertPEM returns the current root certificate in PEM form.
func (s *Service) CertPEM() []byte { return bytes.Clone(s.cur.Load().pem) }

// CertDER returns the current root certificate in DER form.
func (s *Service) CertDER() []byte { return bytes.Clone(s.cur.Load().cert.Raw) }

// Generation counts how many roots this service has served; it starts at 1 and
// grows on every [Service.Regenerate] and [Service.Import].
func (s *Service) Generation() uint64 { return s.cur.Load().gen }

func (s *Service) cached(key string) (*tls.Certificate, bool) {
	return s.cache.Get(key)
}

// Regenerate replaces the root CA with a fresh one and drops every cached leaf.
func (s *Service) Regenerate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := generateRoot(s.now())
	if err != nil {
		return err
	}

	if err := s.replace(ctx, root); err != nil {
		return err
	}

	s.log.Info("regenerated root CA", "not_after", root.cert.NotAfter.UTC().Format(time.RFC3339))

	return nil
}

// Import replaces the root CA with an operator-supplied PEM certificate and
// private key (PKCS#8, SEC1 or PKCS#1) and drops every cached leaf. The
// certificate must be a CA that can sign certificates and the key must be ECDSA
// or RSA (browsers do not accept Ed25519 certificate signatures); anything else
// is rejected with [ErrInvalidCA].
func (s *Service) Import(ctx context.Context, certPEM, keyPEM []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key, err := parseKeyPEM(keyPEM)
	if err != nil {
		return err
	}

	der, err := marshalKeyDER(key)
	if err != nil {
		return err
	}

	root, err := parseRoot(certPEM, der)
	if err != nil {
		return err
	}

	if err := errExpired(root.cert, s.now()); err != nil {
		return err
	}

	if err := s.replace(ctx, root); err != nil {
		return err
	}

	s.log.Info("imported root CA", "subject", root.cert.Subject.CommonName,
		"not_after", root.cert.NotAfter.UTC().Format(time.RFC3339))

	return nil
}

// Export returns the current root certificate and its private key in PEM form.
// The key is unencrypted: whoever holds it can intercept TLS for every client
// that trusts this CA, so it must stay as guarded as secret.key.
func (s *Service) Export() (certPEM, keyPEM []byte, err error) {
	iss := s.cur.Load()

	key, err := marshalKeyPEM(iss.key)
	if err != nil {
		return nil, nil, err
	}

	return bytes.Clone(iss.pem), key, nil
}

// replace stores root, makes it current under a new generation and purges the
// leaf cache. Callers hold s.mu.
func (s *Service) replace(ctx context.Context, root *rootCA) error {
	if err := s.save(ctx, root); err != nil {
		return err
	}

	old := s.cur.Load()
	s.cur.Store(&issuer{cert: root.cert, key: root.key, pem: root.pem, gen: old.gen + 1})

	s.cache.Purge()

	return nil
}

// cacheKey identifies a leaf: leaves signed by a replaced root can never be
// served under the new generation.
func cacheKey(gen uint64, host string) string {
	return strconv.FormatUint(gen, 10) + "|" + host
}
