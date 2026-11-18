package middleware

import (
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// The session endpoints (T-024). They are the only paths served without
// credentials: a browser needs them to obtain the token cookie
// (docs/03-architecture.md §8).
const (
	loginPath  = "/api/rest/auth/login"
	logoutPath = "/api/rest/auth/logout"
)

// maxLoginBodyBytes caps the login body; a token fits easily.
const maxLoginBodyBytes = 4 << 10

const (
	loginLimit  = 5 // login attempts per IP and window (a success resets it)
	loginWindow = time.Minute
	maxLoginIPs = 1024 // bound on the limiter's map
)

// defaultLoginLimiter serves the [Auth] values that leave loginLimiter nil. One
// process runs one Auth, so sharing its failures is what we want.
var defaultLoginLimiter = newLoginLimiter(loginLimit, loginWindow)

// serveSession serves the session endpoints and reports whether it handled the
// request. They bypass [Auth.allows] so a browser without credentials can log
// in; nothing else does.
func (a Auth) serveSession(w http.ResponseWriter, r *http.Request) bool {
	var handle http.HandlerFunc

	switch r.URL.Path {
	case loginPath:
		handle = a.login
	case logoutPath:
		handle = a.logout
	default:
		return false
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "use POST")

		return true
	}

	// Same CSRF rules as POST /api/graphql: a cross-site page can send neither a
	// JSON content type nor the header without a preflight (docs/03 §8).
	RequireAPIRequest(handle).ServeHTTP(w, r)

	return true
}

// login sets the token cookie when body.Token is the configured token. The
// cookie value is the token itself: there is one token per process and it is
// compared in constant time, so a session table would only add state to leak.
func (a Auth) login(w http.ResponseWriter, r *http.Request) {
	// Nothing to guess without a token: skip the limiter and the log.
	if !a.Required() {
		unauthorized(w)

		return
	}

	var body struct {
		Token string `json:"token"`
	}

	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "unreadable request body")
		}

		return
	}

	if err := json.Unmarshal(data, &body); err != nil {
		writeError(w, http.StatusBadRequest, `body must be JSON: {"token": "…"}`)

		return
	}

	// Reserve the attempt atomically, after the body is in, so connections that
	// stall before sending it cannot all pass a check made up front.
	ip := remoteIP(r)
	if !a.limiter().attempt(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, retry later")

		return
	}

	if !a.CheckToken(body.Token) {
		a.logger().Warn("login failed", "component", "auth", "remote_ip", ip)
		unauthorized(w)

		return
	}

	a.limiter().succeeded(ip)

	c := a.tokenCookie(r)
	if err := c.Valid(); err != nil {
		writeError(w, http.StatusInternalServerError, "the auth token cannot be stored in a cookie")

		return
	}

	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

// logout clears the token cookie. It needs no credentials, so a stale cookie
// (e.g. after a token change) can always be dropped.
func (a Auth) logout(w http.ResponseWriter, r *http.Request) {
	c := a.tokenCookie(r) //nolint:gosec // T-024: the cookie is built with HttpOnly and SameSite=Strict; Secure follows TLS.

	c.Value = ""
	c.MaxAge = -1 // delete now
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

// tokenCookie is the cookie carrying the token. Secure follows TLS on the
// request itself; behind a TLS-terminating proxy the operator needs a plain
// HTTP listener on loopback, where a Secure cookie would never come back.
func (a Auth) tokenCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{ //nolint:gosec // T-024: Secure is conditional on purpose; HttpOnly and SameSite=Strict are set.
		Name:     TokenCookie,
		Value:    a.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil,
	}
}

// remoteIP keys the rate limiter. The TCP peer is used on purpose;
// X-Forwarded-For is never trusted (docs/03 §8).
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

func (a Auth) limiter() *loginLimiter {
	if a.loginLimiter != nil {
		return a.loginLimiter
	}

	return defaultLoginLimiter
}

func (a Auth) logger() *slog.Logger {
	if a.Logger != nil {
		return a.Logger
	}

	return slog.Default()
}

// loginLimiter counts login attempts per remote IP in a fixed window. It is safe
// for concurrent use.
type loginLimiter struct {
	limit  int
	window time.Duration
	maxIPs int

	mu       sync.Mutex
	failures map[string]loginCount
}

type loginCount struct {
	count   int
	resetAt time.Time
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		limit:    limit,
		window:   window,
		maxIPs:   maxLoginIPs,
		failures: make(map[string]loginCount),
	}
}

// attempt reserves one login attempt for ip and reports whether it is within
// the limit. Check and increment share one lock, so concurrent callers cannot
// overspend the window.
func (l *loginLimiter) attempt(ip string) bool {
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	f, ok := l.failures[ip]
	if !ok || !now.Before(f.resetAt) {
		l.makeRoom(now)
		f = loginCount{resetAt: now.Add(l.window)}
	}

	if f.count >= l.limit {
		return false
	}

	f.count++
	l.failures[ip] = f

	return true
}

// succeeded forgets ip's attempts.
func (l *loginLimiter) succeeded(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.failures, ip)
}

// makeRoom keeps the map bounded: expired entries go first, and if every
// tracked IP is still inside its window the one that expires first makes room.
// Evicting a live entry only shortens someone else's block, never extends it.
func (l *loginLimiter) makeRoom(now time.Time) {
	if len(l.failures) < l.maxIPs {
		return
	}

	var (
		oldest      string
		oldestReset time.Time
		found       bool
	)

	for ip, f := range l.failures {
		if !now.Before(f.resetAt) {
			delete(l.failures, ip)

			continue
		}

		if !found || f.resetAt.Before(oldestReset) {
			oldest, oldestReset, found = ip, f.resetAt, true
		}
	}

	if found && len(l.failures) >= l.maxIPs {
		delete(l.failures, oldest)
	}
}
