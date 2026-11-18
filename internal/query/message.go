// Package query holds the types the query languages evaluate against
// (docs/04-query-language.md §5).
package query

import "time"

// Header is one header field, as sent.
type Header struct{ Name, Value string }

// Side is one message of an exchange: the captured bytes, where the body
// starts in them, and the header fields.
type Side struct {
	// Raw is the message as captured, head and body. It aliases the
	// implementation's buffer and is never modified.
	Raw []byte
	// BodyOffset is the offset of the body in Raw; a message without a body
	// has BodyOffset equal to len(Raw).
	BodyOffset int
	// Headers holds the header fields in wire order, duplicates and original
	// casing included.
	Headers []Header
}

// Body returns the body bytes of s.
func (s Side) Body() []byte {
	off := min(max(s.BodyOffset, 0), len(s.Raw))
	return s.Raw[off:]
}

// Response is the response side of an exchange.
type Response struct {
	Side
	Code      int
	RoundTrip time.Duration
}

// Message is one HTTP exchange as the query evaluator reads it. Every method
// must be cheap: the in-memory back-end runs it per clause and per message.
//
// An answered request whose response was never captured reports a nil
// [Response]; the evaluator then sees code 0, an empty body and no headers.
type Message interface {
	ID() int64
	Source() string
	Host() string
	Port() int
	TLS() bool
	Method() string
	Path() string  // request path, without the query
	Query() string // query string, without the leading "?"
	Ext() string   // extension with its dot, or "" when the path has none
	CreatedAt() time.Time
	Color() string
	Note() string
	Edited() bool
	Request() Side
	Response() *Response
}
