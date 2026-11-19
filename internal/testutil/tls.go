package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync"
	"testing"
	"time"
)

// TLSHello is what a client offered in its TLS ClientHello.
type TLSHello struct {
	ServerName string
	ALPN       []string
}

// TLSRawServer is a [RawServer] behind TLS with a self-signed certificate for
// localhost and 127.0.0.1. It also records each ClientHello.
type TLSRawServer struct {
	*RawServer

	mu     sync.Mutex
	hellos []TLSHello
}

// NewTLSRawServer starts a TLSRawServer on loopback; it stops with the test.
func NewTLSRawServer(t testing.TB) *TLSRawServer {
	t.Helper()

	cert := selfSigned(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s := &TLSRawServer{RawServer: &RawServer{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port}}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"http/1.1"},
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			s.mu.Lock()
			s.hellos = append(s.hellos, TLSHello{ServerName: h.ServerName, ALPN: append([]string(nil), h.SupportedProtos...)})
			s.mu.Unlock()
			return nil, nil
		},
	}
	tln := tls.NewListener(ln, cfg)

	var wg sync.WaitGroup

	wg.Go(func() {
		for {
			c, err := tln.Accept()
			if err != nil {
				return
			}

			wg.Go(func() { s.serve(c) })

			t.Cleanup(func() { _ = c.Close() })
		}
	})

	t.Cleanup(func() {
		_ = tln.Close()
		wg.Wait()
	})

	return s
}

// Hellos returns the ClientHellos received so far.
func (s *TLSRawServer) Hellos() []TLSHello {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]TLSHello(nil), s.hellos...)
}

func selfSigned(t testing.TB) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tmpl := &x509.Certificate{
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}
