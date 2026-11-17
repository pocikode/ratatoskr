package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/pocikode/ratatoskr/internal/api/graphql/generated"
	"github.com/pocikode/ratatoskr/internal/api/graphql/resolvers"
	"github.com/pocikode/ratatoskr/internal/api/graphql/scalars"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func TestPresentError(t *testing.T) {
	t.Parallel()

	_, int64Err := scalars.UnmarshalInt64("12x")

	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
		wantSpan map[string]int
		wantLog  bool
	}{
		{
			name: "service error", err: apperr.New(apperr.NotFound, "project 7 not found"),
			wantCode: "NOT_FOUND", wantMsg: "project 7 not found",
		},
		{
			name: "query error with span", err: apperr.Query(apperr.Span{Start: 3, End: 8}, "unknown field"),
			wantCode: "BAD_QUERY", wantMsg: "unknown field", wantSpan: map[string]int{"start": 3, "end": 8},
		},
		{
			name: "stub", err: apperr.ErrNotImplemented,
			wantCode: "NOT_IMPLEMENTED", wantMsg: "not implemented yet",
		},
		{
			name: "coded internal error hides its message", err: apperr.New(apperr.Internal, "disk path /home/x leaked"),
			wantCode: "INTERNAL", wantMsg: "internal error", wantLog: true,
		},
		{
			name: "unclassified error is internal", err: io.ErrUnexpectedEOF,
			wantCode: "INTERNAL", wantMsg: "internal error", wantLog: true,
		},
		{
			name: "gqlgen validation error keeps its code",
			err: &gqlerror.Error{
				Message:    "Cannot query field",
				Extensions: map[string]any{"code": "GRAPHQL_VALIDATION_FAILED"},
			},
			wantCode: "GRAPHQL_VALIDATION_FAILED", wantMsg: "Cannot query field",
		},
		{
			// What generated code does with an unmarshal error: graphql.ErrorOnPath.
			name: "argument error from a scalar", err: graphql.ErrorOnPath(context.Background(), int64Err),
			wantCode: "BAD_INPUT", wantMsg: int64Err.Error(),
		},
		{
			name: "message built by gqlgen", err: gqlerror.Errorf("could not read request body"),
			wantCode: "BAD_INPUT", wantMsg: "could not read request body",
		},
		{
			// What gqlgen does with a resolver error before presenting it.
			name:     "plain resolver error wrapped by gqlgen",
			err:      graphql.ErrorOnPath(context.Background(), errors.New("open /home/x/project.db: locked")),
			wantCode: "INTERNAL", wantMsg: "internal error", wantLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			present := presentError(slog.New(slog.NewTextHandler(&logs, nil)))
			got := present(context.Background(), tt.err)

			if got.Extensions["code"] != tt.wantCode {
				t.Errorf("code = %v, want %s", got.Extensions["code"], tt.wantCode)
			}

			if got.Message != tt.wantMsg {
				t.Errorf("message = %q, want %q", got.Message, tt.wantMsg)
			}

			span, _ := got.Extensions["span"].(map[string]int)
			if tt.wantSpan == nil && span != nil || tt.wantSpan != nil && (span["start"] != tt.wantSpan["start"] || span["end"] != tt.wantSpan["end"]) {
				t.Errorf("span = %v, want %v", span, tt.wantSpan)
			}

			if logged := strings.Contains(logs.String(), "graphql internal error"); logged != tt.wantLog {
				t.Errorf("logged = %v, want %v (%s)", logged, tt.wantLog, logs.String())
			}
		})
	}
}

func TestPresentErrorDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	in := gqlerror.Wrap(errors.New("bad argument"))
	_ = presentError(slog.New(slog.DiscardHandler))(context.Background(), in)

	if in.Extensions != nil {
		t.Errorf("input extensions = %v, want untouched", in.Extensions)
	}
}

// failingRoot is the real root resolver with Query.projects failing with an
// unclassified error and Query.serverInfo panicking.
type failingRoot struct{ *resolvers.Resolver }

func (r failingRoot) Query() generated.QueryResolver { return failingQuery{r.Resolver.Query()} }

type failingQuery struct{ generated.QueryResolver }

func (failingQuery) Projects(context.Context) ([]generated.Project, error) {
	return nil, fmt.Errorf("open /home/x/project.db: %w", io.ErrUnexpectedEOF)
}

func (failingQuery) ServerInfo(context.Context) (*generated.ServerInfo, error) {
	panic("boom")
}

// TestHandlerErrorCodes runs errors through gqlgen's real execution path, which
// wraps resolver errors before the presenter sees them.
func TestHandlerErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantCode  string
		wantMsg   string
		wantLog   bool
		wantInLog string
	}{
		{
			name: "plain resolver error", body: `{"query":"{ projects { id } }"}`,
			wantCode: "INTERNAL", wantMsg: "internal error", wantLog: true, wantInLog: "/home/x/project.db",
		},
		{
			name: "resolver panic", body: `{"query":"{ serverInfo { version } }"}`,
			wantCode: "INTERNAL", wantMsg: "internal error", wantLog: true, wantInLog: "boom",
		},
		{
			name: "service error", body: `{"query":"mutation { selectProject(id: \"1\") { id } }"}`,
			wantCode: "NOT_IMPLEMENTED", wantMsg: "not implemented yet",
		},
		{
			name: "missing variable", body: `{"query":"query($id: ID!) { project(id: $id) { id } }"}`,
			wantCode: "GRAPHQL_VALIDATION_FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			h := NewHandler(Options{
				Resolvers: failingRoot{resolvers.New(resolvers.Deps{})},
				Logger:    slog.New(slog.NewTextHandler(&logs, nil)),
			})

			r := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)

			var out struct {
				Errors []struct {
					Message    string         `json:"message"`
					Extensions map[string]any `json:"extensions"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Errors) == 0 {
				t.Fatalf("status %d, body %s (%v)", rec.Code, rec.Body, err)
			}

			e := out.Errors[0]
			if e.Extensions["code"] != tt.wantCode || tt.wantMsg != "" && e.Message != tt.wantMsg {
				t.Errorf("error = %q %v, want %q %s", e.Message, e.Extensions, tt.wantMsg, tt.wantCode)
			}

			if logged := strings.Contains(logs.String(), "graphql"); logged != tt.wantLog ||
				!strings.Contains(logs.String(), tt.wantInLog) {
				t.Errorf("log = %q, want logged=%v containing %q", logs.String(), tt.wantLog, tt.wantInLog)
			}
		})
	}
}
