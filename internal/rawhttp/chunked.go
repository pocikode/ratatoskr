package rawhttp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
)

// maxChunkLine bounds a chunk-size line or a trailer field.
const maxChunkLine = 8 << 10

// SplitChunkedBody decodes one complete wire body and returns its trailer
// fields as a slice of wire, excluding the final blank line.
func SplitChunkedBody(wire []byte) (payload, trailer []byte, err error) {
	src := bytes.NewReader(wire)
	c := &chunkedReader{br: bufio.NewReader(src)}
	payload, err = io.ReadAll(c)
	if err != nil {
		return nil, nil, err
	}
	if src.Len()+c.br.Buffered() != 0 {
		return nil, nil, fmt.Errorf("rawhttp: %w: bytes after chunked body", ErrFraming)
	}
	end := len(wire) - len(c.line)
	return payload, wire[end-c.trailerBytes : end], nil
}

// chunkedReader reads a chunked body (RFC 9112 §7.1) leniently: bare LF line
// endings and chunk extensions are accepted, and the bytes are surfaced either
// with their framing (emitFraming, the on-wire view) or without it (payload).
type chunkedReader struct {
	br          *bufio.Reader
	emitFraming bool

	state        chunkState
	remain       int64
	buf          []byte
	off          int
	line         []byte
	err          error
	trailerBytes int
}

type chunkState uint8

const (
	stateChunkSize chunkState = iota
	stateChunkData
	stateChunkDataEnd
	stateChunkTrailer
	stateChunkDone
)

func (c *chunkedReader) Read(p []byte) (int, error) {
	for c.off == len(c.buf) {
		if c.state == stateChunkDone {
			if c.err != nil {
				return 0, c.err
			}
			return 0, io.EOF
		}
		c.buf, c.off = c.buf[:0], 0
		if err := c.step(); err != nil {
			c.err, c.state = err, stateChunkDone
		}
	}
	n := copy(p, c.buf[c.off:])
	c.off += n
	return n, nil
}

func (c *chunkedReader) step() error {
	switch c.state {
	case stateChunkSize:
		line, err := c.readLine()
		if err != nil {
			return err
		}
		size, err := parseChunkSize(line)
		if err != nil {
			return err
		}
		if c.emitFraming {
			c.buf = append(c.buf, line...)
		}
		if size == 0 {
			c.state = stateChunkTrailer
			return nil
		}
		c.remain, c.state = size, stateChunkData

	case stateChunkData:
		want := int(min(c.remain, 1<<13))
		start := len(c.buf)
		c.buf = slices.Grow(c.buf, want)[:start+want]
		n, err := io.ReadFull(c.br, c.buf[start:])
		c.buf = c.buf[:start+n]
		c.remain -= int64(n)
		if err != nil {
			return unexpectedEOF(err)
		}
		if c.remain == 0 {
			c.state = stateChunkDataEnd
		}

	case stateChunkDataEnd:
		return c.readDataEnd()

	case stateChunkTrailer:
		line, err := c.readLine()
		if err != nil {
			return err
		}
		if c.emitFraming {
			c.buf = append(c.buf, line...)
		}
		if isBlank(line) {
			c.state = stateChunkDone
		} else {
			c.trailerBytes += len(line)
		}

	default:
		c.state = stateChunkDone
	}
	return nil
}

func (c *chunkedReader) readDataEnd() error {
	b, err := c.br.ReadByte()
	if err != nil {
		return unexpectedEOF(err)
	}
	c.emit(b)
	if b == '\n' {
		c.state = stateChunkSize
		return nil
	}
	if b != '\r' {
		return fmt.Errorf("rawhttp: %w: chunk data is not followed by a line break", ErrFraming)
	}
	if b, err = c.br.ReadByte(); err != nil {
		return unexpectedEOF(err)
	}
	c.emit(b)
	if b != '\n' {
		return fmt.Errorf("rawhttp: %w: chunk data is not followed by a line break", ErrFraming)
	}
	c.state = stateChunkSize
	return nil
}

func (c *chunkedReader) emit(b byte) {
	if c.emitFraming {
		c.buf = append(c.buf, b)
	}
}

// readLine reads one line, terminator included. The result is reused by the
// next call.
func (c *chunkedReader) readLine() ([]byte, error) {
	c.line = c.line[:0]
	for {
		frag, err := c.br.ReadSlice('\n')
		if len(c.line)+len(frag) > maxChunkLine {
			return nil, fmt.Errorf("rawhttp: %w: chunk framing line too long", ErrFraming)
		}
		c.line = append(c.line, frag...)
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err == nil:
			return c.line, nil
		default:
			return nil, unexpectedEOF(err)
		}
	}
}

// parseChunkSize reads the hexadecimal size that starts a chunk-size line and
// ignores chunk extensions.
func parseChunkSize(line []byte) (int64, error) {
	var n int64
	for i, b := range line {
		switch b {
		case ';', ' ', '\t', '\r', '\n':
			if i == 0 {
				return 0, fmt.Errorf("rawhttp: %w: empty chunk size", ErrFraming)
			}
			return n, nil
		}
		d := hexDigit(b)
		if d < 0 || n > (math.MaxInt64-int64(d))/16 {
			return 0, fmt.Errorf("rawhttp: %w: bad chunk size %q", ErrFraming, line)
		}
		n = n*16 + int64(d)
	}
	if len(line) == 0 {
		return 0, fmt.Errorf("rawhttp: %w: empty chunk size", ErrFraming)
	}
	return n, nil
}

func hexDigit(b byte) int {
	switch {
	case b >= '0' && b <= '9':
		return int(b - '0')
	case b >= 'a' && b <= 'f':
		return int(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return int(b-'A') + 10
	}
	return -1
}

func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
