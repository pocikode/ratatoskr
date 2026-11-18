package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Hosts rejects requests that do not address this instance (docs/03-architecture.md §8).
//
// DNS rebinding cannot be caught by [CrossOrigin] or the WebSocket origin check,
// because the attacker's page has its own origin as Host. A page on a rebinding
// domain therefore gets 421 here, before CSRF or auth runs.
//
// allowed holds the configured listen address and the names Ratatoskr answers to
// besides "localhost" and loopback IP literals (ports are ignored). A nil or empty
// allowed disables the check: a listener bound to a non-loopback address is
// protected by --auth-token instead.
func Hosts(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		return next
	}

	names := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		names[hostName(name)] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, names) {
			writeError(w, http.StatusMisdirectedRequest, "unexpected Host header")

			return
		}

		next.ServeHTTP(w, r)
	})
}

// hostAllowed reports whether hostport names Ratatoskr: localhost, a loopback IP
// literal or one of names.
func hostAllowed(hostport string, names map[string]struct{}) bool {
	host := hostName(hostport)

	if _, ok := names[host]; ok {
		return true
	}

	if host == "localhost" {
		return true
	}

	ip, err := netip.ParseAddr(host)

	// Unmap makes ::ffff:127.0.0.1 a loopback address too.
	return err == nil && ip.Unmap().IsLoopback()
}

// hostName returns the lower-cased host of hostport with any port and IPv6
// brackets removed. A value without a port is returned as it is.
func hostName(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}

	return strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"))
}
