package graphql

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

func TestPresentError(t *testing.T) {
	t.Parallel()

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
			name: "argument error from gqlgen", err: gqlerror.Wrap(errors.New("invalid Int64: bad")),
			wantCode: "BAD_INPUT", wantMsg: "invalid Int64: bad",
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
