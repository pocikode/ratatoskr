package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/pocikode/ratatoskr/internal/api"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/api/middleware"
)

const token = "s3cret"

func newServer(t *testing.T, auth middleware.Auth) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(api.NewHandler(api.Options{
		Resolvers: resolvers.Deps{
			Build:        resolvers.BuildInfo{Version: "v0.0.1-test", Commit: "abc123", GoVersion: "go1.27.4"},
			StartedAt:    time.Now(),
			PingInterval: 10 * time.Millisecond,
		},
		Auth: auth,
	}))
	t.Cleanup(srv.Close)

	return srv
}

type gqlResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// post sends query and returns the status code and the decoded JSON body.
func post(t *testing.T, srv *httptest.Server, query string, edit func(*http.Request)) (int, gqlResponse) {
	t.Helper()

	body, _ := json.Marshal(map[string]string{"query": query})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.APIHeader, "1")

	if edit != nil {
		edit(req)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	var out gqlResponse
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}

	return resp.StatusCode, out
}

func TestServerInfoQuery(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{})

	status, out := post(t, srv, `{ serverInfo { version commit goVersion startedAt authRequired } }`, nil)
	if status != http.StatusOK || len(out.Errors) > 0 {
		t.Fatalf("status %d, errors %+v", status, out.Errors)
	}

	var info struct {
		Version, Commit, GoVersion, StartedAt string
		AuthRequired                          bool
	}
	if err := json.Unmarshal(out.Data["serverInfo"], &info); err != nil {
		t.Fatal(err)
	}

	if info.Version != "v0.0.1-test" || info.Commit != "abc123" || info.AuthRequired || !strings.HasSuffix(info.StartedAt, "Z") {
		t.Errorf("serverInfo = %+v", info)
	}
}

func TestErrorCodes(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{})

	tests := []struct {
		query    string
		wantCode string
	}{
		{`{ projects { id } }`, "NOT_IMPLEMENTED"},
		{`mutation { selectProject(id: "1") { id } }`, "NOT_IMPLEMENTED"},
		{`{ nope }`, "GRAPHQL_VALIDATION_FAILED"},
		{`{ serverInfo { `, "GRAPHQL_PARSE_FAILED"},
	}

	for _, tt := range tests {
		status, out := post(t, srv, tt.query, nil)
		if status != http.StatusOK && status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", tt.query, status)
			continue
		}

		if len(out.Errors) == 0 || out.Errors[0].Extensions["code"] != tt.wantCode {
			t.Errorf("%s: errors %+v, want code %s", tt.query, out.Errors, tt.wantCode)
		}
	}
}

func TestCSRF(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{})

	tests := []struct {
		name string
		edit func(*http.Request)
		want int
	}{
		{"valid", nil, http.StatusOK},
		{"missing header", func(r *http.Request) { r.Header.Del(middleware.APIHeader) }, http.StatusForbidden},
		{"form content type", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, http.StatusUnsupportedMediaType},
		{"cross-site browser request", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }, http.StatusForbidden},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }, http.StatusForbidden},
	}

	for _, tt := range tests {
		status, _ := post(t, srv, `{ serverInfo { version } }`, tt.edit)
		if status != tt.want {
			t.Errorf("%s: status %d, want %d", tt.name, status, tt.want)
		}
	}

	resp, err := srv.Client().Get(srv.URL + "/api/graphql?query={serverInfo{version}}")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/graphql: status %d, want 405", resp.StatusCode)
	}
}

func TestAuthHTTP(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{Token: token})

	if status, _ := post(t, srv, `{ serverInfo { version } }`, nil); status != http.StatusUnauthorized {
		t.Errorf("no token: status %d, want 401", status)
	}

	status, out := post(t, srv, `{ serverInfo { authRequired } }`, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if status != http.StatusOK || string(out.Data["serverInfo"]) != `{"authRequired":true}` {
		t.Errorf("bearer: status %d, data %s", status, out.Data["serverInfo"])
	}
}

// subscribe opens a graphql-transport-ws connection, sends connection_init with
// payload and returns the connection after the server's answer to it.
func dialWS(t *testing.T, srv *httptest.Server, payload map[string]any) (*websocket.Conn, map[string]any, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)

	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/graphql/ws",
		&websocket.DialOptions{Subprotocols: []string{"graphql-transport-ws"}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	closeBody(resp)

	t.Cleanup(func() { _ = conn.CloseNow() })

	write(t, conn, map[string]any{"type": "connection_init", "payload": payload})

	msg, err := read(ctx, conn)

	return conn, msg, err
}

// closeBody closes a dial response body, which is nil after a successful upgrade.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func write(t *testing.T, conn *websocket.Conn, msg any) {
	t.Helper()

	b, _ := json.Marshal(msg)
	if err := conn.Write(t.Context(), websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func read(ctx context.Context, conn *websocket.Conn) (map[string]any, error) {
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			return nil, err
		}

		var msg map[string]any
		if err := json.Unmarshal(b, &msg); err != nil {
			return nil, err
		}

		if msg["type"] != "ka" && msg["type"] != "ping" {
			return msg, nil
		}
	}
}

func TestPingSubscription(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{})

	conn, ack, err := dialWS(t, srv, nil)
	if err != nil || ack["type"] != "connection_ack" {
		t.Fatalf("init: %v, %v", ack, err)
	}

	write(t, conn, map[string]any{
		"id": "1", "type": "subscribe",
		"payload": map[string]any{"query": "subscription { ping { seq at } }"},
	})

	for want := 1; want <= 2; want++ {
		msg, err := read(t.Context(), conn)
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		b, _ := json.Marshal(msg["payload"])
		if msg["type"] != "next" || !strings.Contains(string(b), fmt.Sprintf(`"seq":"%d"`, want)) {
			t.Fatalf("message %d = %s %s", want, msg["type"], b)
		}
	}

	write(t, conn, map[string]any{"id": "1", "type": "complete"})
}

func TestAuthWebSocket(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{Token: token})

	_, ack, err := dialWS(t, srv, map[string]any{"token": token})
	if err != nil || ack["type"] != "connection_ack" {
		t.Fatalf("valid token: %v, %v", ack, err)
	}

	conn, msg, err := dialWS(t, srv, map[string]any{"token": "wrong"})
	if err == nil && msg["type"] == "connection_ack" {
		t.Fatal("wrong token was acknowledged")
	}

	// The server reports the error, then closes with 4401.
	for err == nil {
		_, err = read(t.Context(), conn)
	}

	if status := websocket.CloseStatus(err); status != 4401 {
		t.Errorf("close status = %d (%v), want 4401", status, err)
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	t.Parallel()

	srv := newServer(t, middleware.Auth{})

	_, resp, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/api/graphql/ws",
		&websocket.DialOptions{
			Subprotocols: []string{"graphql-transport-ws"},
			HTTPHeader:   http.Header{"Origin": {"https://evil.example"}},
		})
	if resp != nil {
		closeBody(resp)
	}

	if err == nil {
		t.Fatal("cross-origin WebSocket was accepted")
	}

	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("response = %v, want 403", resp)
	}
}
