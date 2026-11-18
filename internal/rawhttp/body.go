package rawhttp

import "io"

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
