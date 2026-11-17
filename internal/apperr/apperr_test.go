package apperr_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

func TestCodeOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want apperr.Code
	}{
		{"plain error", io.EOF, apperr.Internal},
		{"direct", apperr.New(apperr.NotFound, "project %d not found", 7), apperr.NotFound},
		{"wrapped", fmt.Errorf("rename: %w", apperr.New(apperr.Conflict, "name taken")), apperr.Conflict},
		{"stub sentinel", apperr.ErrNotImplemented, apperr.NotImplemented},
		{"empty code", &apperr.Error{Msg: "x"}, apperr.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := apperr.CodeOf(tt.err); got != tt.want {
				t.Errorf("CodeOf() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewKeepsCause(t *testing.T) {
	t.Parallel()

	err := apperr.New(apperr.Internal, "open project: %w", io.ErrUnexpectedEOF)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("errors.Is(%v, io.ErrUnexpectedEOF) = false", err)
	}

	if err.Msg != "open project: unexpected EOF" {
		t.Errorf("Msg = %q", err.Msg)
	}
}

func TestQuerySpan(t *testing.T) {
	t.Parallel()

	err := apperr.Query(apperr.Span{Start: 4, End: 9}, "unknown field %q", "rq.x")
	if err.Code != apperr.BadQuery || err.Span == nil || *err.Span != (apperr.Span{Start: 4, End: 9}) {
		t.Errorf("Query() = %+v", err)
	}
}
