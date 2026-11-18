package ca_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/ca"
)

// TestLeafVerifies is the acceptance-criteria table: every kind of host a
// browser or a client can put in SNI gets a leaf that chains to the root and
// covers that host.
func TestLeafVerifies(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	rootPool := roots(svc)

	tests := []struct {
		name string
		// host is what the proxy asks for.
		host string
		// wantSAN is the DNS name or IP literal expected in the leaf.
		wantSAN string
		// verifyAs is the name the client checks the certificate against.
		verifyAs string
		wantIP   bool
	}{
		{name: "plain", host: "example.com", wantSAN: "example.com", verifyAs: "example.com"},
		{name: "subdomain", host: "api.sub.example.com", wantSAN: "api.sub.example.com", verifyAs: "api.sub.example.com"},
		{name: "with port", host: "example.com:443", wantSAN: "example.com", verifyAs: "example.com"},
		{name: "uppercase and trailing dot", host: "Example.COM.", wantSAN: "example.com", verifyAs: "example.com"},
		{name: "wildcard-looking", host: "*.wild.example", wantSAN: "*.wild.example", verifyAs: "anything.wild.example"},
		{name: "idn punycode", host: "xn--mnchen-3ya.example", wantSAN: "xn--mnchen-3ya.example", verifyAs: "xn--mnchen-3ya.example"},
		{name: "ipv4", host: "192.0.2.10", wantSAN: "192.0.2.10", verifyAs: "192.0.2.10", wantIP: true},
		{name: "ipv4 with port", host: "192.0.2.10:8080", wantSAN: "192.0.2.10", verifyAs: "192.0.2.10", wantIP: true},
		{name: "ipv6", host: "2001:db8::1", wantSAN: "2001:db8::1", verifyAs: "2001:db8::1", wantIP: true},
		{name: "ipv6 bracketed", host: "[2001:db8::1]", wantSAN: "2001:db8::1", verifyAs: "2001:db8::1", wantIP: true},
		{name: "ipv6 bracketed with port", host: "[2001:db8::1]:8443", wantSAN: "2001:db8::1", verifyAs: "2001:db8::1", wantIP: true},
		{name: "ipv6 loopback", host: "::1", wantSAN: "::1", verifyAs: "::1", wantIP: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			leaf, err := svc.Certificate(tc.host)
			if err != nil {
				t.Fatalf("Certificate(%q): %v", tc.host, err)
			}

			verifyChain(t, leaf, rootPool)

			// VerifyHostname covers both DNS SANs and IP SANs.
			if err := leaf.Leaf.VerifyHostname(tc.verifyAs); err != nil {
				t.Fatalf("leaf does not cover %q: %v", tc.verifyAs, err)
			}

			if tc.wantIP {
				ip := net.ParseIP(tc.wantSAN)
				if !slices.ContainsFunc(leaf.Leaf.IPAddresses, ip.Equal) {
					t.Errorf("IPAddresses = %v, want %s", leaf.Leaf.IPAddresses, tc.wantSAN)
				}

				if len(leaf.Leaf.DNSNames) != 0 {
					t.Errorf("DNSNames = %v, want none for an IP", leaf.Leaf.DNSNames)
				}
			} else {
				if !slices.Equal(leaf.Leaf.DNSNames, []string{tc.wantSAN}) {
					t.Errorf("DNSNames = %v, want [%s]", leaf.Leaf.DNSNames, tc.wantSAN)
				}

				if len(leaf.Leaf.IPAddresses) != 0 {
					t.Errorf("IPAddresses = %v, want none for a DNS name", leaf.Leaf.IPAddresses)
				}
			}

			// A leaf is an ECDSA P-256 end-entity certificate valid for a year.
			pub, ok := leaf.Leaf.PublicKey.(*ecdsa.PublicKey)
			if !ok {
				t.Fatalf("leaf key is %T, want ECDSA", leaf.Leaf.PublicKey)
			}

			if pub.Curve != elliptic.P256() {
				t.Errorf("leaf curve = %v, want P-256", pub.Curve)
			}

			if leaf.Leaf.IsCA {
				t.Error("leaf is a CA certificate")
			}

			if _, err := leaf.Leaf.Verify(x509.VerifyOptions{
				Roots:     rootPool,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}); err != nil {
				t.Fatalf("leaf lacks server-auth usage: %v", err)
			}

			validity := leaf.Leaf.NotAfter.Sub(leaf.Leaf.NotBefore)
			if validity < 364*24*time.Hour || validity > 366*24*time.Hour {
				t.Errorf("leaf validity = %v, want ~365 days", validity)
			}

			// The chain carries the root so a client that trusts it needs no extra
			// fetch.
			if len(leaf.Certificate) != 2 {
				t.Fatalf("chain length = %d, want leaf + root", len(leaf.Certificate))
			}

			rootDER, _ := pem.Decode(svc.CertPEM())
			if rootDER == nil || string(rootDER.Bytes) != string(leaf.Certificate[1]) {
				t.Error("chain does not end with the root CA")
			}
		})
	}
}

// TestLeafCommonNameIsTrimmed keeps the CN inside the X.509 64-character limit
// for very long hosts.
func TestLeafCommonNameIsTrimmed(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	host := strings.Repeat("abcdefghijklmnopqrstuvwxyz0123456789.", 6) + "example"

	leaf, err := svc.Certificate(host)
	if err != nil {
		t.Fatal(err)
	}

	if len(leaf.Leaf.Subject.CommonName) > 64 {
		t.Errorf("CommonName length = %d, want <= 64", len(leaf.Leaf.Subject.CommonName))
	}
}

func TestCertificateRejectsInvalidHosts(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	hosts := []string{
		"",
		"   ",
		"exa mple.com",
		"example..com",
		".",
		"*.",
		"*.*.example.com",
		"ex*ample.com",
		"münchen.example", // raw IDN: SNI carries punycode, never UTF-8
		"[::1",
		"[::1]x",
		"fe80::1%eth0",
		"a..b:443",
	}

	for _, host := range hosts {
		t.Run(host, func(t *testing.T) {
			t.Parallel()

			if _, err := svc.Certificate(host); !errors.Is(err, ca.ErrInvalidHost) && !errors.Is(err, ca.ErrNoServerName) {
				t.Fatalf("Certificate(%q) error = %v, want ErrInvalidHost/ErrNoServerName", host, err)
			}
		})
	}
}

// TestConcurrentIssuance asks for the same host from many goroutines: the cache
// and singleflight must collapse them into one certificate (run with -race).
func TestConcurrentIssuance(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	const workers = 64

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		leafs = make(map[string]int)
	)

	for range workers {
		wg.Go(func() {
			cert, err := svc.Certificate("concurrent.example")
			if err != nil {
				t.Errorf("Certificate: %v", err)

				return
			}

			mu.Lock()
			leafs[string(cert.Certificate[0])]++
			mu.Unlock()
		})
	}

	wg.Wait()

	if len(leafs) != 1 {
		t.Fatalf("issued %d distinct certificates for one host, want 1", len(leafs))
	}

	for raw, count := range leafs {
		if count != workers {
			t.Errorf("certificate issued %d/%d times", count, workers)
		}

		if _, err := x509.ParseCertificate([]byte(raw)); err != nil {
			t.Errorf("bad certificate: %v", err)
		}
	}
}

// TestRegenerateDuringIssuance hammers the cache while the root changes: it
// must stay race-free and never hand out a leaf signed by a replaced root
// (run with -race).
func TestRegenerateDuringIssuance(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 25 {
				cert, err := svc.Certificate("race.example")
				if errors.Is(err, ca.ErrRootChanged) {
					// A handshake here simply fails and the client retries.
					continue
				}

				if err != nil {
					t.Errorf("Certificate: %v", err)

					return
				}

				if cert == nil {
					t.Error("nil certificate")
				}
			}
		})
	}

	wg.Go(func() {
		for range 5 {
			if err := svc.Regenerate(t.Context()); err != nil {
				t.Errorf("Regenerate: %v", err)

				return
			}
		}
	})

	wg.Wait()

	// Whatever the interleaving, the certificate the service now hands out
	// chains to the root it advertises.
	cert, err := svc.Certificate("race.example")
	if err != nil {
		t.Fatal(err)
	}

	verifyChain(t, cert, roots(svc))
}

func TestCachedCertificateIsReused(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	first, err := svc.Certificate("cached.example")
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Certificate("cached.example:443")
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("second lookup missed the cache (different *tls.Certificate)")
	}
}

func TestTLSConfigMinVersion(t *testing.T) {
	t.Parallel()

	e := newEnv(t)
	svc := e.open(t)

	cfg := svc.TLSConfig()

	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2", cfg.MinVersion)
	}

	if !slices.Equal(cfg.NextProtos, []string{"http/1.1"}) {
		t.Errorf("NextProtos = %v, want [http/1.1]", cfg.NextProtos)
	}
}

// BenchmarkCertificateCached is the acceptance criterion: a cached leaf lookup
// stays under a millisecond (the handshake path calls it once per connection).
func BenchmarkCertificateCached(b *testing.B) {
	e := newEnv(b)
	svc := e.open(b)

	if _, err := svc.Certificate("bench.example"); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		cert, err := svc.Certificate("bench.example")
		if err != nil {
			b.Fatal(err)
		}

		if cert == nil {
			b.Fatal("nil certificate")
		}
	}
}
