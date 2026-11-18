package rawhttp

import (
	"bufio"
	"errors"
	"io"
	"iter"
	"slices"
	"strings"
)

// Framing is how a message body is delimited on the wire (RFC 9112 §6).
type Framing uint8

const (
	// FramingNone means the message carries no body.
	FramingNone Framing = iota
	// FramingLength means Content-Length delimits the body.
	FramingLength
	// FramingChunked means Transfer-Encoding: chunked delimits the body.
	FramingChunked
	// FramingClose means the body ends when the peer closes the connection.
	FramingClose
)

// String implements [fmt.Stringer].
func (f Framing) String() string {
	switch f {
	case FramingNone:
		return "none"
	case FramingLength:
		return "length"
	case FramingChunked:
		return "chunked"
	case FramingClose:
		return "close"
	}
	return "unknown"
}

// Warning names recorded in [Message.Warnings] for input that is ambiguous or
// malformed but still forwardable. The capture layer persists them as
// framing_warning (docs/01-feature-spec.md F-NET-3).
const (
	WarnBareLF                            = "bare-lf"
	WarnObsFold                           = "obs-fold"
	WarnMalformedHeader                   = "malformed-header"
	WarnNoVersion                         = "no-version"
	WarnBadVersion                        = "bad-version"
	WarnLowerCaseMethod                   = "lowercase-method"
	WarnDuplicateContentLength            = "duplicate-content-length"
	WarnContentLengthWithTransferEncoding = "content-length-with-transfer-encoding"
	WarnMultipleTransferEncodings         = "multiple-transfer-encodings"
	WarnTransferEncodingOnHTTP10          = "transfer-encoding-on-http10"
)

// Errors returned by the readers. Every parse or framing error wraps one of
// them, so callers can answer a broken client with the right status.
var (
	// ErrMalformed means the start line could not be understood at all.
	ErrMalformed = errors.New("rawhttp: malformed message")
	// ErrFraming means the body boundaries are ambiguous (conflicting
	// Content-Length values, an unsupported Transfer-Encoding).
	ErrFraming = errors.New("rawhttp: ambiguous body framing")
	// ErrHeadTooLarge means the head exceeded Limits.MaxHeadBytes.
	ErrHeadTooLarge = errors.New("rawhttp: head too large")
)

// A Header is one header field of a message, with its name and value exactly
// as sent (only the optional leading and trailing whitespace of the value is
// trimmed, as RFC 9110 §5.5 requires of a parser).
type Header struct {
	Name  string
	Value string

	// Raw is the field exactly as it appeared on the wire, line terminators
	// and folding included. A Builder writes it back untouched unless the
	// field was edited; a Header with an empty Raw is serialized from Name and
	// Value. Headers parsed from a message always carry it.
	Raw string
}

type kind uint8

const (
	kindRequest kind = iota + 1
	kindResponse
)

// Message is a parsed HTTP/1.x message: the exact head bytes, a structured
// view of them, and a body that streams from the connection.
//
// The body is single-use: read it through [Message.Body] (or [Message.WriteTo])
// once. [Message.Raw] is complete only after the body has been read.
type Message struct {
	raw     []byte
	kind    kind
	method  string
	target  string
	version string
	status  int
	reason  string

	headers  []Header
	warnings []string
	framing  Framing
	length   int64
	close    bool

	lineEnd int // end of the start line, terminator included
	bodyAt  int // offset of the empty line that ends the head

	br           *bufio.Reader
	body         *bodyReader
	captured     []byte
	captureLimit int64
	truncated    bool
}

// Head returns the message head exactly as it was read, the final empty line
// included. The slice aliases the message and must not be modified.
func (m *Message) Head() []byte { return m.raw }

// Raw returns the head followed by the body bytes captured so far. It is the
// whole message once the body has been read to EOF; a body larger than
// Limits.MaxBodyBytes is cut short (see [Message.Truncated]).
func (m *Message) Raw() []byte {
	raw := make([]byte, 0, len(m.raw)+len(m.captured))
	raw = append(raw, m.raw...)
	return append(raw, m.captured...)
}

// Headers iterates the header fields in wire order, duplicates and original
// casing included. Iterating allocates nothing.
func (m *Message) Headers() iter.Seq2[int, Header] { return slices.All(m.headers) }

// Header returns the value of the first field named name (case-insensitive).
func (m *Message) Header(name string) (string, bool) {
	for _, h := range m.headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value, true
		}
	}
	return "", false
}

// HeaderValues returns every value of the fields named name, in wire order.
func (m *Message) HeaderValues(name string) []string {
	var values []string
	for _, h := range m.headers {
		if strings.EqualFold(h.Name, name) {
			values = append(values, h.Value)
		}
	}
	return values
}

// Framing reports how the body is delimited.
func (m *Message) Framing() Framing { return m.framing }

// ContentLength returns the declared body length, or -1 when the message is
// chunked, close-delimited or has no length at all.
func (m *Message) ContentLength() int64 { return m.length }

// Close reports whether the connection must be closed after the message
// (RFC 9112 §9.6).
func (m *Message) Close() bool { return m.close }

// Warnings returns the oddities found while parsing, as Warn* names. The slice
// aliases the message and must not be modified.
func (m *Message) Warnings() []string { return m.warnings }

// Truncated reports whether the body outstripped Limits.MaxBodyBytes, so
// [Message.Raw] holds only the first MaxBodyBytes bytes of it.
func (m *Message) Truncated() bool { return m.truncated }

// Body returns the body exactly as it is framed on the wire: chunked bodies
// keep their chunk-size lines and trailers, close-delimited bodies end at EOF.
// Use [Message.DecodedBody] for the payload itself.
func (m *Message) Body() io.Reader {
	if m.body == nil {
		m.body = &bodyReader{m: m, src: m.framingReader()}
	}
	return m.body
}

// DecodedBody returns the body payload: chunked framing removed, everything
// else identical to [Message.Body]. Only one of Body and DecodedBody may be
// read.
func (m *Message) DecodedBody() io.Reader {
	if m.framing != FramingChunked {
		return m.Body()
	}
	return &chunkedReader{br: bufio.NewReader(m.Body()), emitFraming: false}
}

// WriteTo writes the head followed by the rest of the body, byte for byte as
// received, and captures the body bytes on the way. It consumes the body.
func (m *Message) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(m.raw)
	if err != nil {
		return int64(n), err
	}
	if n != len(m.raw) {
		return int64(n), io.ErrShortWrite
	}
	body, err := io.Copy(w, m.Body())
	return int64(n) + body, err
}

func (m *Message) framingReader() io.Reader {
	switch m.framing {
	case FramingLength:
		return io.LimitReader(m.br, m.length)
	case FramingChunked:
		return &chunkedReader{br: m.br, emitFraming: true}
	case FramingClose:
		return m.br
	}
	return noBody
}

func (m *Message) warn(warning string) {
	if !slices.Contains(m.warnings, warning) {
		m.warnings = append(m.warnings, warning)
	}
}

// Request is an HTTP/1.x request.
type Request struct{ Message }

// Method returns the request method exactly as sent.
func (r *Request) Method() string { return r.method }

// Target returns the request target: origin-form, absolute-form,
// authority-form (CONNECT) or "*".
func (r *Request) Target() string { return r.target }

// Version returns the HTTP version of the request line, empty when it had none
// (HTTP/0.9 style).
func (r *Request) Version() string { return r.version }

// Response is an HTTP/1.x response.
type Response struct{ Message }

// Status returns the status code.
func (r *Response) Status() int { return r.status }

// Reason returns the reason phrase, possibly empty, exactly as sent.
func (r *Response) Reason() string { return r.reason }

// Version returns the HTTP version of the status line.
func (r *Response) Version() string { return r.version }
