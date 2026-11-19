package middleware

import (
	"bytes"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const loginToken = "s3cret"

// nextNoContent is the wrapped handler: 204, so an endpoint status is never
// confused with the pass-through.
var nextNoContent = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

// loginAuth is an [Auth] with its own limiter and a silent logger, so cases do
// not share rate-limit state.
func loginAuth() Auth {
	return Auth{
		Token:        loginToken,
		Logger:       slog.New(slog.DiscardHandler),
		loginLimiter: newLoginLimiter(loginLimit, loginWindow),
	}
}

// sessionRequest is a well-formed request for one of the session endpoints.
func sessionRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()

	r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set(APIHeader, "1")
	r.RemoteAddr = "192.0.2.10:4567"

	return r
}

func serveAuth(a Auth, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.Handler(nextNoContent).ServeHTTP(rec, r)

	return rec
}

// responseCookie returns the named cookie of the response, if any.
func responseCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}

	return nil
}

func TestLogin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		edit     func(*http.Request)
		want     int
		wantCook bool
		secure   bool
	}{
		{name: "valid token", body: `{"token":"s3cret"}`, want: http.StatusNoContent, wantCook: true},
		{
			name: "over TLS", body: `{"token":"s3cret"}`, want: http.StatusNoContent, wantCook: true, secure: true,
			edit: func(r *http.Request) { r.TLS = &tls.ConnectionState{} },
		},
		{name: "unknown body members", body: `{"token":"s3cret","next":"/"}`,
			want: http.StatusNoContent, wantCook: true},
		{name: "wrong token", body: `{"token":"s3cre"}`, want: http.StatusUnauthorized},
		{name: "empty body object", body: `{}`, want: http.StatusUnauthorized},
		{name: "null token", body: `{"token":null}`, want: http.StatusUnauthorized},
		{name: "token is not a string", body: `{"token":1}`, want: http.StatusBadRequest},
		{name: "malformed JSON", body: `{"token":`, want: http.StatusBadRequest},
		{name: "empty body", body: "", want: http.StatusBadRequest},
		{name: "body too large", body: `{"token":"` + strings.Repeat("a", maxLoginBodyBytes) + `"}`,
			want: http.StatusRequestEntityTooLarge},
		{
			name: "missing API header", body: `{"token":"s3cret"}`,
			edit: func(r *http.Request) { r.Header.Del(APIHeader) }, want: http.StatusForbidden,
		},
		{
			name: "form content type", body: `{"token":"s3cret"}`,
			edit: func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
			want: http.StatusUnsupportedMediaType,
		},
		{
			name: "not POST", body: "",
			edit: func(r *http.Request) { r.Method = http.MethodGet }, want: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := sessionRequest(t, http.MethodPost, loginPath, tt.body)
			if tt.edit != nil {
				tt.edit(r)
			}

			rec := serveAuth(loginAuth(), r)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}

			c := responseCookie(rec, TokenCookie)
			if !tt.wantCook {
				if c != nil {
					t.Errorf("unexpected cookie %v", c)
				}

				return
			}

			if c == nil {
				t.Fatal("no token cookie")
			}

			if c.Value != loginToken {
				t.Errorf("cookie value = %q, want the token", c.Value)
			}

			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
				t.Errorf("cookie attributes = %+v", c)
			}

			if c.Secure != tt.secure {
				t.Errorf("Secure = %v, want %v", c.Secure, tt.secure)
			}
		})
	}
}

// TestLoginAuthDisabled: with no --auth-token there is nothing to log in to.
func TestLoginAuthDisabled(t *testing.T) {
	t.Parallel()

	a := loginAuth()
	a.Token = ""

	rec := serveAuth(a, sessionRequest(t, http.MethodPost, loginPath, `{"token":""}`))
	if rec.Code != http.StatusUnauthorized || responseCookie(rec, TokenCookie) != nil {
		t.Errorf("status = %d, cookie = %v", rec.Code, rec.Result().Cookies())
	}
}

// TestSessionEndpointsTakeNoCredentials: both endpoints work without a token,
// cookie or guest allowance.
func TestSessionEndpointsTakeNoCredentials(t *testing.T) {
	t.Parallel()

	a := loginAuth()
	a.Logger = slog.New(slog.DiscardHandler)

	if rec := serveAuth(a, sessionRequest(t, http.MethodPost, loginPath, `{"token":"s3cret"}`)); rec.Code != http.StatusNoContent {
		t.Errorf("login: status = %d, want 204", rec.Code)
	}

	if rec := serveAuth(a, sessionRequest(t, http.MethodPost, logoutPath, "")); rec.Code != http.StatusNoContent {
		t.Errorf("logout: status = %d, want 204", rec.Code)
	}
}

// TestSessionExemptionIsNarrow: only the two session paths skip auth.
func TestSessionExemptionIsNarrow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, loginPath, `{"token":"s3cret"}`, http.StatusNoContent},
		{http.MethodPost, logoutPath, "", http.StatusNoContent},
		{http.MethodGet, loginPath, "", http.StatusMethodNotAllowed},
		{http.MethodGet, loginPath + "/", "", http.StatusUnauthorized},
		{http.MethodPost, logoutPath + "/", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/rest/auth/", "", http.StatusUnauthorized},
		{http.MethodPost, "/api/rest/auth/Login", `{"token":"s3cret"}`, http.StatusUnauthorized},
		{http.MethodGet, "/api/rest/files/1", "", http.StatusUnauthorized},
		{http.MethodGet, "/api/graphql", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		rec := serveAuth(loginAuth(), sessionRequest(t, tt.method, tt.path, tt.body))
		if rec.Code != tt.want {
			t.Errorf("%s %s: status = %d, want %d", tt.method, tt.path, rec.Code, tt.want)
		}
	}
}

func TestLogout(t *testing.T) {
	t.Parallel()

	rec := serveAuth(loginAuth(), sessionRequest(t, http.MethodPost, logoutPath, ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}

	c := responseCookie(rec, TokenCookie)
	if c == nil {
		t.Fatal("logout did not clear the cookie")
	}

	if c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("cookie = %+v, want an already expired empty cookie", c)
	}

	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Errorf("cookie attributes = %+v", c)
	}
}

// TestLoginCrossSite: the session endpoints follow the same CSRF rules as the
// GraphQL endpoint (docs/03 §8).
func TestLoginCrossSite(t *testing.T) {
	t.Parallel()

	r := sessionRequest(t, http.MethodPost, loginPath, `{"token":"s3cret"}`)
	r.Header.Set("Sec-Fetch-Site", "cross-site")

	rec := httptest.NewRecorder()
	CrossOrigin(loginAuth().Handler(nextNoContent)).ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden || responseCookie(rec, TokenCookie) != nil {
		t.Errorf("status = %d, want 403 without a cookie", rec.Code)
	}
}

// TestLoginRateLimit: five failures block the sixth attempt, the block ends
// with the window, and a successful login clears the count.
func TestLoginRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := loginAuth()
		wrong := `{"token":"s3cre"}`
		right := `{"token":"s3cret"}`

		login := func(body string) int {
			return serveAuth(a, sessionRequest(t, http.MethodPost, loginPath, body)).Code
		}

		for i := range loginLimit {
			if got := login(wrong); got != http.StatusUnauthorized {
				t.Fatalf("failure %d: status = %d, want 401", i+1, got)
			}
		}

		if got := login(wrong); got != http.StatusTooManyRequests {
			t.Errorf("after %d failures: status = %d, want 429", loginLimit, got)
		}

		if got := login(right); got != http.StatusTooManyRequests {
			t.Errorf("correct token while blocked: status = %d, want 429", got)
		}

		synctest.Sleep(loginWindow)

		if got := login(right); got != http.StatusNoContent {
			t.Errorf("after the window: status = %d, want 204", got)
		}

		// A success clears the count, so the next window starts empty.
		for i := range loginLimit - 1 {
			if got := login(wrong); got != http.StatusUnauthorized {
				t.Fatalf("failure %d after a success: status = %d, want 401", i+1, got)
			}
		}

		if got := login(right); got != http.StatusNoContent {
			t.Errorf("success below the limit: status = %d, want 204", got)
		}
	})
}

// TestLoginRateLimitPerIP: one address cannot spend another address's failures.
func TestLoginRateLimitPerIP(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := loginAuth()

		attempt := func(remote string) int {
			r := sessionRequest(t, http.MethodPost, loginPath, `{"token":"s3cre"}`)
			r.RemoteAddr = remote

			return serveAuth(a, r).Code
		}

		for range loginLimit {
			attempt("192.0.2.10:1")
		}

		if got := attempt("192.0.2.10:2"); got != http.StatusTooManyRequests {
			t.Errorf("same IP, other port: status = %d, want 429", got)
		}

		if got := attempt("198.51.100.7:1"); got != http.StatusUnauthorized {
			t.Errorf("other IP: status = %d, want 401", got)
		}
	})
}

// TestLoginLimiterBounded: the map never outgrows maxIPs and expired entries
// are swept, not evicted live ones.
func TestLoginLimiterBounded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLoginLimiter(1, time.Minute)
		l.maxIPs = 2

		for _, ip := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
			l.attempt(ip)
		}

		if len(l.failures) != 2 {
			t.Fatalf("map holds %d entries, want maxIPs", len(l.failures))
		}

		synctest.Sleep(time.Minute)
		l.attempt("192.0.2.4")

		if len(l.failures) != 1 {
			t.Errorf("expired entries were not swept: %v", l.failures)
		}
	})
}

// TestLoginLimiterConcurrent: racing attempts never overspend the window.
func TestLoginLimiterConcurrent(t *testing.T) {
	t.Parallel()

	l := newLoginLimiter(loginLimit, loginWindow)

	var (
		wg      sync.WaitGroup
		allowed atomic.Int32
	)

	for range 2 * loginLimit {
		wg.Go(func() {
			if l.attempt("192.0.2.1") {
				allowed.Add(1)
			}
		})
	}

	wg.Wait()

	if got := allowed.Load(); got != loginLimit {
		t.Errorf("allowed = %d, want %d", got, loginLimit)
	}
}

// TestLoginAuthDisabledQuiet: with no token configured, login neither logs nor
// spends limiter slots.
func TestLoginAuthDisabledQuiet(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := loginAuth()
	a.Token = ""
	a.Logger = slog.New(slog.NewTextHandler(&buf, nil))

	for range 2 * loginLimit {
		if got := serveAuth(a, sessionRequest(t, http.MethodPost, loginPath, `{"token":""}`)).Code; got != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", got)
		}
	}

	if buf.Len() != 0 || len(a.loginLimiter.failures) != 0 {
		t.Errorf("log %q, limiter %v: want both empty", buf.String(), a.loginLimiter.failures)
	}
}

// TestLoginLogsWithoutTheSubmittedValue: the warn line names the IP, never the
// token the client sent.
func TestLoginLogsWithoutTheSubmittedValue(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	a := loginAuth()
	a.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	rec := serveAuth(a, sessionRequest(t, http.MethodPost, loginPath, `{"token":"hunter2"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	logged := buf.String()
	if !strings.Contains(logged, "level=WARN") || !strings.Contains(logged, `msg="login failed"`) ||
		!strings.Contains(logged, "remote_ip=192.0.2.10") {
		t.Errorf("log = %q, want a warn with the remote IP", logged)
	}

	if strings.Contains(logged, "hunter2") {
		t.Errorf("log leaks the submitted token: %q", logged)
	}
}
