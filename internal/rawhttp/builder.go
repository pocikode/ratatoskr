package rawhttp

import (
	"bytes"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Builder re-serializes a message after an edit, reusing the original bytes for
// every part that was not changed: an untouched head, header field or body is
// written back exactly as it was received.
//
// The zero-value Builder is not usable; call [Message.Builder].
type Builder struct {
	m *Message

	method, target *string
	status         *int
	reason         *string

	headers    []Header
	headersSet bool

	body    []byte
	bodySet bool
}

// Builder returns a builder for m.
func (m *Message) Builder() *Builder { return &Builder{m: m} }

// SetMethod replaces the request method. It is ignored on a response.
func (b *Builder) SetMethod(method string) *Builder {
	b.method = &method
	return b
}

// SetTarget replaces the request target. It is ignored on a response.
func (b *Builder) SetTarget(target string) *Builder {
	b.target = &target
	return b
}

// SetStatus replaces the response status code and its reason phrase. It is
// ignored on a request.
func (b *Builder) SetStatus(code int, reason string) *Builder {
	b.status, b.reason = &code, &reason
	return b
}

// SetHeader replaces every field named name (case-insensitive) with a single
// field, keeping the position of the first one.
func (b *Builder) SetHeader(name, value string) *Builder {
	b.editHeaders()
	kept := b.headers[:0]
	replaced := false
	for _, h := range b.headers {
		if strings.EqualFold(h.Name, name) {
			if !replaced {
				// Keep the field's own spelling; only the value changed.
				kept = append(kept, Header{Name: h.Name, Value: value})
				replaced = true
			}
			continue
		}
		kept = append(kept, h)
	}
	if !replaced {
		kept = append(kept, Header{Name: name, Value: value})
	}
	b.headers = kept
	return b
}

// AddHeader appends a field at the end of the header block.
func (b *Builder) AddHeader(name, value string) *Builder {
	b.editHeaders()
	b.headers = append(b.headers, Header{Name: name, Value: value})
	return b
}

// DelHeader removes every field named name (case-insensitive).
func (b *Builder) DelHeader(name string) *Builder {
	b.editHeaders()
	b.headers = slices.DeleteFunc(b.headers, func(h Header) bool {
		return strings.EqualFold(h.Name, name)
	})
	return b
}

// SetHeaders replaces the whole header block with hs, in order. A field with a
// non-empty Raw is written as Raw (terminator included); others are serialized
// from Name and Value. The builder keeps hs; the caller must not modify it.
func (b *Builder) SetHeaders(hs []Header) *Builder {
	b.headers, b.headersSet = hs, true
	return b
}

// SetBody replaces the body and re-frames it with Content-Length, dropping any
// Transfer-Encoding field. Without SetBody the original body is streamed as
// received.
func (b *Builder) SetBody(body []byte) *Builder {
	b.body, b.bodySet = body, true
	return b.DelHeader("Transfer-Encoding").
		SetHeader("Content-Length", strconv.Itoa(len(body)))
}

// SetChunkedBody replaces the body of a message that already uses chunked
// transfer encoding. Trailer fields include their line endings and exclude
// the final blank line, as returned by [SplitChunkedBody].
func (b *Builder) SetChunkedBody(payload, trailer []byte) *Builder {
	body := make([]byte, 0, len(payload)+len(trailer)+24)
	if len(payload) > 0 {
		body = strconv.AppendInt(body, int64(len(payload)), 16)
		body = append(body, "\r\n"...)
		body = append(body, payload...)
		body = append(body, "\r\n"...)
	}
	body = append(body, "0\r\n"...)
	body = append(body, trailer...)
	body = append(body, "\r\n"...)
	b.body, b.bodySet = body, true
	return b.DelHeader("Content-Length")
}

// WriteTo writes the re-serialized message. A body that was not replaced is
// read from the connection, so WriteTo consumes it.
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	head := b.head()
	n, err := w.Write(head)
	if err != nil {
		return int64(n), err
	}
	if n != len(head) {
		return int64(n), io.ErrShortWrite
	}
	if b.bodySet {
		n, err = w.Write(b.body)
		return int64(len(head) + n), err
	}
	body, err := io.Copy(w, b.m.Body())
	return int64(len(head)) + body, err
}

// Head returns the re-serialized head, final empty line included, without
// reading the body.
func (b *Builder) Head() []byte { return b.head() }

// Bytes returns the re-serialized message.
func (b *Builder) Bytes() []byte {
	var buf bytes.Buffer
	_, _ = b.WriteTo(&buf)
	return buf.Bytes()
}

func (b *Builder) editHeaders() {
	if !b.headersSet {
		b.headers, b.headersSet = slices.Clone(b.m.headers), true
	}
}

func (b *Builder) head() []byte {
	head := make([]byte, 0, len(b.m.raw))
	if b.lineEdited() {
		head = append(head, b.startLine()...)
	} else {
		head = append(head, b.m.raw[:b.m.lineEnd]...)
	}
	if b.headersSet {
		for _, h := range b.headers {
			if h.Raw != "" {
				head = append(head, h.Raw...)
				continue
			}
			head = append(head, h.Name...)
			head = append(head, ':', ' ')
			head = append(head, h.Value...)
			head = append(head, b.m.terminator()...)
		}
	} else {
		head = append(head, b.m.raw[b.m.lineEnd:b.m.bodyAt]...)
	}
	return append(head, b.m.raw[b.m.bodyAt:]...)
}

// lineEdited reports whether a start-line field of this message's own kind was
// replaced. A setter for the other kind is ignored, not a reason to rebuild
// (and not a nil dereference).
func (b *Builder) lineEdited() bool {
	if b.m.kind == kindResponse {
		return b.status != nil || b.reason != nil
	}
	return b.method != nil || b.target != nil
}

func (b *Builder) startLine() []byte {
	var line []byte
	if b.m.kind == kindRequest {
		line = append(line, b.str(b.method, b.m.method)...)
		line = append(line, ' ')
		line = append(line, b.str(b.target, b.m.target)...)
		if b.m.version != "" {
			line = append(line, ' ')
			line = append(line, b.m.version...)
		}
	} else {
		code := b.m.status
		if b.status != nil {
			code = *b.status
		}
		line = append(line, b.m.version...)
		line = append(line, ' ')
		line = strconv.AppendInt(line, int64(code), 10)
		if reason := b.str(b.reason, b.m.reason); reason != "" {
			line = append(line, ' ')
			line = append(line, reason...)
		}
	}
	return append(line, b.m.terminator()...)
}

func (b *Builder) str(edited *string, original string) string {
	if edited != nil {
		return *edited
	}
	return original
}

// terminator returns the line ending the message used, so a rebuilt start line
// keeps it.
func (m *Message) terminator() []byte {
	line := m.raw[:m.lineEnd]
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		return line[len(line)-2:]
	}
	return line[len(line)-1:]
}
