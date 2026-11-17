package middleware

import (
	"context"
	"crypto/subtle"
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
}

// Required reports whether a token is configured.
func (a Auth) Required() bool { return a.Token != "" }

// CheckToken compares tok with the configured token in constant time.
func (a Auth) CheckToken(tok string) bool {
	return a.Required() && subtle.ConstantTimeCompare([]byte(tok), []byte(a.Token)) == 1
}

type authKey struct{}

// Authenticated reports whether the request behind ctx passed [Auth.Handler]:
// it presented the token, is a permitted guest, or auth is off. WebSocket
// upgrades without credentials reach the GraphQL layer unauthenticated so the
// token can still arrive in connection_init.
func Authenticated(ctx context.Context) bool {
	ok, _ := ctx.Value(authKey{}).(bool)

	return ok
}

// Handler authenticates every request. Unauthenticated requests get 401, except
// WebSocket upgrades, which are passed on with [Authenticated] false.
func (a Auth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := a.allows(r)
		if !ok && !isWebSocketUpgrade(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="ratatoskr"`)
			writeError(w, http.StatusUnauthorized, "authentication required")

			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), authKey{}, ok)))
	})
}

func (a Auth) allows(r *http.Request) bool {
	if !a.Required() {
		return true
	}

	if a.AllowGuests && isLoopbackRemote(r.RemoteAddr) {
		return true
	}

	if tok, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); found && a.CheckToken(tok) {
		return true
	}

	if c, err := r.Cookie(TokenCookie); err == nil && a.CheckToken(c.Value) {
		return true
	}

	return false
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
