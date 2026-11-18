package ca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const (
	// leafValidity is how long an issued leaf is trusted for (F-NET-2).
	leafValidity = 365 * 24 * time.Hour
	// maxCommonName is the X.509 upper bound for a printable CommonName.
	maxCommonName = 64
	// maxHostLen is the DNS name length limit (RFC 1035).
	maxHostLen = 253
	// maxLabelLen is the DNS label length limit.
	maxLabelLen = 63
)

// errRootExpires reports that the root cannot cover a leaf any more.
var errRootExpires = errors.New("ca: root CA expires too soon to issue a leaf")

// issue signs a leaf certificate for host with a fresh ECDSA P-256 key of its
// own. host is either an IP literal or a DNS name (a leading "*." label stays a
// wildcard).
func (s *Service) issue(iss *issuer, host string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ca: generate leaf key for %q: %w", host, err)
	}

	serial, err := newSerial()
	if err != nil {
		return nil, err
	}

	now := s.now()

	notBefore := now.Add(-backdate)
	if notBefore.Before(iss.cert.NotBefore) {
		notBefore = iss.cert.NotBefore
	}

	notAfter := now.Add(leafValidity)
	if notAfter.After(iss.cert.NotAfter) {
		notAfter = iss.cert.NotAfter
	}

	if !notAfter.After(notBefore) {
		return nil, fmt.Errorf("%w (expires %s)", errRootExpires, iss.cert.NotAfter.UTC().Format(time.RFC3339))
	}

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName(host)},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		SubjectKeyId:          keyID(&key.PublicKey),
		AuthorityKeyId:        iss.cert.SubjectKeyId,
	}

	if ip, isIP, _ := parseHostIP(host); isIP {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, iss.cert, &key.PublicKey, iss.key)
	if err != nil {
		return nil, fmt.Errorf("ca: sign leaf for %q: %w", host, err)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("ca: parse leaf for %q: %w", host, err)
	}

	return &tls.Certificate{
		// The chain carries the root too: clients that trust it directly still
		// get a complete path, and clients that skip it are unaffected.
		Certificate: [][]byte{der, iss.cert.Raw},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}

// commonName truncates host to the X.509 CommonName limit.
func commonName(host string) string {
	if len(host) <= maxCommonName {
		return host
	}

	return host[:maxCommonName]
}

// normalizeHost strips a port and brackets from host, lowercases it and rejects
// names we cannot put in a certificate. SNI never carries a port, but a CONNECT
// fallback host does.
func normalizeHost(host string) (string, error) {
	h := strings.TrimSpace(host)
	if h == "" {
		return "", ErrNoServerName
	}

	// [::1]:443, [::1] and ::1 all mean the same address.
	if strings.HasPrefix(h, "[") {
		end := strings.IndexByte(h, ']')
		if end < 0 {
			return "", fmt.Errorf("%w: unbalanced brackets in %q", ErrInvalidHost, host)
		}

		rest := h[end+1:]
		if rest != "" && !isPortSuffix(rest) {
			return "", fmt.Errorf("%w: %q", ErrInvalidHost, host)
		}

		h = h[1:end]
	} else if strings.Count(h, ":") == 1 {
		if name, _, err := net.SplitHostPort(h); err == nil {
			h = name
		}
	}

	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if h == "" {
		return "", fmt.Errorf("%w: empty host in %q", ErrInvalidHost, host)
	}

	_, isIP, err := parseHostIP(h)
	if err != nil {
		return "", err
	}

	if isIP {
		return h, nil
	}

	if err := validateDNSName(h); err != nil {
		return "", err
	}

	return h, nil
}

// isPortSuffix reports whether s is ":<digits>", so "[::1]:443" keeps its port
// and "[::1]x" is rejected.
func isPortSuffix(s string) bool {
	if !strings.HasPrefix(s, ":") || len(s) == 1 {
		return false
	}

	_, err := strconv.ParseUint(s[1:], 10, 16)

	return err == nil
}

// parseHostIP parses host as an IP literal. ip is the 16-byte form the
// certificate needs; isIP is false for DNS names.
func parseHostIP(host string) (ip net.IP, isIP bool, err error) {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// A name that merely looks numeric ("1.2.3.4.5", "999.1.1.1") is still
		// a DNS name, but a colon or empty label is not.
		if strings.ContainsAny(host, ":[]") {
			return nil, false, fmt.Errorf("%w: %q", ErrInvalidHost, host)
		}

		return nil, false, nil
	}

	if addr.Zone() != "" {
		return nil, false, fmt.Errorf("%w: zone in %q", ErrInvalidHost, host)
	}

	return net.IP(addr.AsSlice()), true, nil
}

// validateDNSName accepts the names that can appear in a certificate SAN: ASCII
// letters, digits, hyphens, underscores, dots, and a single leading "*." label.
func validateDNSName(host string) error {
	if len(host) > maxHostLen {
		return fmt.Errorf("%w: %q is longer than %d bytes", ErrInvalidHost, host, maxHostLen)
	}

	labels := strings.Split(host, ".")
	for i, label := range labels {
		wildcard := i == 0 && label == "*" && len(labels) > 1
		if wildcard {
			continue
		}

		if label == "" || len(label) > maxLabelLen {
			return fmt.Errorf("%w: bad label in %q", ErrInvalidHost, host)
		}

		for _, r := range label {
			ok := r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
			if !ok {
				// Punycode ("xn--…") is ASCII, so a non-ASCII rune here means the
				// caller sent a raw IDN, which SNI never does (RFC 6066 §3).
				return fmt.Errorf("%w: unexpected character %q in %q", ErrInvalidHost, r, host)
			}
		}
	}

	return nil
}
