package middleware

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TokenCookie is the cookie the login page (T-024) sets to the auth token.
const TokenCookie = "ratatoskr_token" //nolint:gosec // T-007: a cookie name, not a credential

// Auth is token authentication for the UI/API (F-WSP-4, docs/03-architecture.md §8).
// The zero value (no token) lets every request through.
type Auth struct {
	// Token is the --auth-token value; empty disables auth.
	Token string
	// AllowGuests lets clients connecting from a loopback address in without a token.
	AllowGuests bool
	// Logger receives failed-login warnings. Nil uses [slog.Default].
	Logger *slog.Logger
	// loginLimiter rates the session endpoints; nil uses [defaultLoginLimiter].
	loginLimiter *loginLimiter
}

// Required reports whether a token is configured.
func (a Auth) Required() bool { return a.Token != "" }

// CheckToken compares tok with the configured token in constant time.
func (a Auth) CheckToken(tok string) bool {
	return a.Required() && subtle.ConstantTimeCompare([]byte(tok), []byte(a.Token)) == 1
}

type authKey struct{}

// Authenticated reports whether the request behind ctx passed [Auth.Handler] or
// [Auth.WebSocket] with credentials: it presented the token, is a permitted
// guest, or auth is off. Only [Auth.WebSocket] lets a request through without
// them, so the token can still arrive in connection_init.
func Authenticated(ctx context.Context) bool {
	ok, _ := ctx.Value(authKey{}).(bool)

	return ok
}

// Handler rejects every unauthenticated request with 401. The session
// endpoints are served here instead, without credentials (T-024).
func (a Auth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.serveSession(w, r) {
			return
		}

		if !a.allows(r) {
			unauthorized(w)

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, true)))
	})
}

// WebSocket is [Auth.Handler] for the GraphQL WebSocket endpoint only: an
// unauthenticated WebSocket upgrade is passed on with [Authenticated] false, so
// the GraphQL layer can check the token of connection_init. Anything else
// without credentials gets 401.
func (a Auth) WebSocket(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := a.allows(r)
		if !ok && !isWebSocketUpgrade(r) {
			unauthorized(w)

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, ok)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="ratatoskr"`)
	writeError(w, http.StatusUnauthorized, "authentication required")
}

func (a Auth) allows(r *http.Request) bool {
	if !a.Required() {
		return true
	}

	if a.AllowGuests && isLoopbackRemote(r.RemoteAddr) {
		return true
	}

	if a.CheckToken(bearerToken(r)) {
		return true
	}

	if c, err := r.Cookie(TokenCookie); err == nil && a.CheckToken(c.Value) {
		return true
	}

	return false
}

// bearerToken returns the credentials of an "Authorization: Bearer <token>"
// header. The scheme is case-insensitive (RFC 9110 §11.1).
func bearerToken(r *http.Request) string {
	scheme, tok, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}

	return strings.TrimSpace(tok)
}

func isWebSocketUpgrade(r *http.Request) bool {
	return r.Method == http.MethodGet &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}
