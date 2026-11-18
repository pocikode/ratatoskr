package rest

import (
	"net/http"
	"strconv"
)

// Certificates is what the CA download routes need; *ca.Service implements it.
type Certificates interface {
	// CertPEM returns the root certificate in PEM form.
	CertPEM() []byte
	// CertDER returns the root certificate in DER form.
	CertDER() []byte
}

const (
	// caPEMType is the registered media type for PEM files.
	caPEMType = "application/x-pem-file"
	// caDERType is the registered media type for a DER certificate.
	caDERType = "application/pkix-cert"
)

// registerCA mounts the root certificate downloads below base: F-NET-2 wants PEM
// for the browser/OS trust stores and DER for the Android and Windows wizards.
func registerCA(mux *http.ServeMux, base string, certs Certificates) {
	mux.HandleFunc("GET "+base+"/ca.crt", func(w http.ResponseWriter, _ *http.Request) {
		serveCert(w, "ratatoskr-ca.crt", caPEMType, certs.CertPEM())
	})
	mux.HandleFunc("GET "+base+"/ca.der", func(w http.ResponseWriter, _ *http.Request) {
		serveCert(w, "ratatoskr-ca.der", caDERType, certs.CertDER())
	})
}

// CAHandler serves /ca.crt (PEM) and /ca.der (DER) on the bare origin, where
// F-NET-2 lets a browser download the certificate
// (http://ratatoskr.localhost/ca.crt). The certificate is public: it holds no
// key material, so these routes need no credentials (docs/03-architecture.md §8
// only gates /api/**).
func CAHandler(certs Certificates) http.Handler {
	mux := http.NewServeMux()
	registerCA(mux, "", certs)

	return mux
}

// serveCert writes the certificate as a download. The body is small and already
// in memory, so Content-Length is set explicitly.
func serveCert(w http.ResponseWriter, name, contentType string, body []byte) {
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	// A regeneration replaces the certificate under the same URL; a cached copy
	// would install the old one.
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Length", strconv.Itoa(len(body)))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
