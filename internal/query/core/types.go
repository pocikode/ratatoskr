package core

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Type is the value type of a field (docs/04 §2).
type Type uint8

// Field types.
const (
	String Type = iota + 1
	Int
	Bool
	Bytes
	Datetime
)

// String implements [fmt.Stringer] with the names used in error messages.
func (t Type) String() string {
	switch t {
	case String:
		return "string"
	case Int:
		return "integer"
	case Bool:
		return "boolean"
	case Bytes:
		return "bytes"
	case Datetime:
		return "datetime"
	}
	return "unknown"
}

// Op is a clause operator (docs/04 §3).
type Op string

// Operators.
const (
	Eq     Op = "eq"
	Ne     Op = "ne"
	Gt     Op = "gt"
	Gte    Op = "gte"
	Lt     Op = "lt"
	Lte    Op = "lte"
	Cont   Op = "cont"
	Ncont  Op = "ncont"
	Like   Op = "like"
	Nlike  Op = "nlike"
	Regex  Op = "regex"
	Nregex Op = "nregex"
)

var (
	textOps    = []Op{Cont, Ncont, Like, Nlike, Regex, Nregex}
	compareOps = []Op{Gt, Gte, Lt, Lte}
	// opsByType is the validity table of docs/04 §3.
	opsByType = map[Type][]Op{
		String:   append([]Op{Eq, Ne}, textOps...),
		Int:      append([]Op{Eq, Ne}, compareOps...),
		Bool:     {Eq, Ne},
		Bytes:    textOps,
		Datetime: compareOps,
	}
	allOps = []Op{Eq, Ne, Gt, Gte, Lt, Lte, Cont, Ncont, Like, Nlike, Regex, Nregex}
)

// Ops lists the operators valid for t, in display order.
func (t Type) Ops() []Op { return slices.Clone(opsByType[t]) }

func (t Type) allows(op Op) bool { return slices.Contains(opsByType[t], op) }

func joinOps(ops []Op) string {
	s := make([]string, len(ops))
	for i, op := range ops {
		s[i] = string(op)
	}
	return strings.Join(s, ", ")
}

// Value is a typed clause operand. Only the field matching Type is set;
// String and Bytes values both live in Str.
type Value struct {
	Type Type
	Str  string
	Int  int64
	Bool bool
	Time time.Time // always UTC
}

// String renders the value as query syntax.
func (v Value) String() string {
	switch v.Type {
	case Int:
		return strconv.FormatInt(v.Int, 10)
	case Bool:
		return strconv.FormatBool(v.Bool)
	case Datetime:
		return quote(v.Time.Format(time.RFC3339Nano))
	}
	return quote(v.Str)
}

func (v Value) equal(w Value) bool {
	return v.Type == w.Type && v.Str == w.Str && v.Int == w.Int && v.Bool == w.Bool && v.Time.Equal(w.Time)
}

// datetimeLayouts are the accepted datetime formats (docs/04 §2): RFC3339,
// ISO8601 without zone, RFC2822, RFC7231 and ISO9075. Zone-less input is UTC.
var datetimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02",
	time.RFC1123Z,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 -0700",
	time.RFC1123,
	"2006-01-02 15:04:05.999999999",
}

// coerce turns the value token into a typed Value for a field of type t.
func coerce(t Type, tok token) (Value, error) {
	quoted := tok.kind == tString
	if tok.kind != tString && tok.kind != tBare {
		return Value{}, errorf(tok.span, "expected a value, found %s", tok.describe())
	}
	switch t {
	case String, Bytes:
		if !quoted {
			return Value{}, errorf(tok.span, "%s value must be quoted: \"%s\"", t, tok.text)
		}
		return Value{Type: t, Str: tok.text}, nil
	case Int:
		n, err := strconv.ParseInt(tok.text, 10, 64)
		if err != nil {
			return Value{}, errorf(tok.span, "%s is not a valid integer", tok.describe())
		}
		return Value{Type: Int, Int: n}, nil
	case Bool:
		if tok.text != "true" && tok.text != "false" {
			return Value{}, errorf(tok.span, "%s is not a boolean; use true or false", tok.describe())
		}
		return Value{Type: Bool, Bool: tok.text == "true"}, nil
	case Datetime:
		for _, layout := range datetimeLayouts {
			tm, err := time.Parse(layout, tok.text)
			if err != nil {
				continue
			}
			if tm = tm.UTC(); tm.Year() < 0 || tm.Year() > 9999 {
				break // would not format back to RFC3339
			}
			return Value{Type: Datetime, Time: tm}, nil
		}
		return Value{}, errorf(tok.span, "%s is not a datetime; use RFC3339 like \"2024-01-31T12:00:00Z\"", tok.describe())
	}
	return Value{}, errorf(tok.span, "field has no value type")
}

// Error is a query error with the byte span to underline (end exclusive).
type Error struct {
	Span Span
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("query: %s (at %d-%d)", e.Msg, e.Span.Start, e.Span.End)
}

func errorf(sp Span, format string, args ...any) *Error {
	return &Error{Span: sp, Msg: fmt.Sprintf(format, args...)}
}
