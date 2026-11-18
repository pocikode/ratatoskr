package rawhttp

import (
	"errors"
	"io"
)

// noBody is the reader of a message without a body.
var noBody io.Reader = emptyReader{}

type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

// bodyReader feeds the caller from the wire and keeps the first
// Limits.MaxBodyBytes bytes for Raw.
type bodyReader struct {
	m   *Message
	src io.Reader
}

func (b *bodyReader) Read(p []byte) (int, error) {
	n, err := b.src.Read(p)
	if n > 0 {
		b.m.capture(p[:n])
	}
	return n, err
}

func (m *Message) capture(p []byte) {
	if room := m.captureLimit - int64(len(m.captured)); room < int64(len(p)) {
		m.truncated = true
		if room < 0 {
			room = 0
		}
		p = p[:room]
	}
	m.captured = append(m.captured, p...)
}

// lengthReader reads a Content-Length body. A peer that closes before the
// declared bytes arrive is an error, not a clean EOF: forwarding the short body
// as complete is how a truncated message gets mistaken for a whole one.
type lengthReader struct {
	r      io.Reader
	remain int64
}

func (l *lengthReader) Read(p []byte) (int, error) {
	if l.remain <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.remain {
		p = p[:l.remain]
	}
	n, err := l.r.Read(p)
	l.remain -= int64(n)
	if l.remain > 0 && errors.Is(err, io.EOF) {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}
