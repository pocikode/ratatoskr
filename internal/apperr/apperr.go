// Package apperr is how services tell the API layer what kind of failure they hit.
//
// A service returns (or wraps) an [*Error]; the GraphQL error presenter finds it with
// errors.AsType and turns Code into extensions.code and Span into extensions.span
// (docs/03-architecture.md §6). Errors without an *Error in their chain are INTERNAL.
// This package imports nothing from the module, so any service may depend on it.
package apperr

import (
	"errors"
	"fmt"
)

// Code classifies an error for API clients. The values match the GraphQL enum
// ErrorCode in internal/api/graphql/schema/base.graphqls.
type Code string

// Error codes, see docs/03-architecture.md §6.
const (
	BadQuery        Code = "BAD_QUERY"
	BadInput        Code = "BAD_INPUT"
	NotFound        Code = "NOT_FOUND"
	Conflict        Code = "CONFLICT"
	Unauthenticated Code = "UNAUTHENTICATED"
	NotImplemented  Code = "NOT_IMPLEMENTED"
	Internal        Code = "INTERNAL"
)

// ErrNotImplemented is returned by contract stubs until the feature lands.
var ErrNotImplemented = &Error{Code: NotImplemented, Msg: "not implemented yet"}

// Span locates a query error as byte offsets into the query text; End is exclusive.
type Span struct {
	Start int
	End   int
}

// Error is a classified error. Msg is the text API clients see and what Error
// returns; Err (optional) is the cause, kept for errors.Is and errors.As. [New]
// includes the cause's text in Msg when the format uses %w.
type Error struct {
	Code Code
	Msg  string
	Err  error
	// Span is set for BAD_QUERY errors.
	Span *Span
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return string(e.Code)
	}

	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

// New returns an error with code and a formatted message. A %w verb in format
// sets the cause.
func New(code Code, format string, args ...any) *Error {
	err := fmt.Errorf(format, args...)

	return &Error{Code: code, Msg: err.Error(), Err: errors.Unwrap(err)}
}

// Query returns a BAD_QUERY error located at span.
func Query(span Span, format string, args ...any) *Error {
	e := New(BadQuery, format, args...)
	e.Span = &span

	return e
}

// CodeOf is the code of the first *Error in err's chain, or Internal.
func CodeOf(err error) Code {
	if e, ok := errors.AsType[*Error](err); ok && e.Code != "" {
		return e.Code
	}

	return Internal
}
