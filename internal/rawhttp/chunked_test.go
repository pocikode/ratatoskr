package rawhttp

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

// slowReader hands out at most one byte per call, so the chunk state machine
// meets every partial-read path bufio can produce.
type slowReader struct{ r io.Reader }

func (s slowReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return s.r.Read(p)
}

func TestChunkedDecode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wire string
		want string
	}{
		{"single chunk", "3\r\nabc\r\n0\r\n\r\n", "abc"},
		{"two chunks", "1\r\na\r\n1\r\nb\r\n0\r\n\r\n", "ab"},
		{"empty", "0\r\n\r\n", ""},
		{"extension", "3;name=value\r\nabc\r\n0\r\n\r\n", "abc"},
		{"bare lf", "3\nabc\n0\n\n", "abc"},
		{"crlf inside data", "4\r\na\r\nb\r\n0\r\n\r\n", "a\r\nb"},
		{"trailer", "2\r\nhi\r\n0\r\nX-Trailer: v\r\n\r\n", "hi"},
		{"hex digits", "a\r\n0123456789\r\n0\r\n\r\n", "0123456789"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for name, src := range map[string]io.Reader{
				"buffered": strings.NewReader(tt.wire),
				"slow":     slowReader{strings.NewReader(tt.wire)},
			} {
				c := &chunkedReader{br: bufio.NewReaderSize(src, 16)}
				got, err := io.ReadAll(c)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if string(got) != tt.want {
					t.Errorf("%s: decoded %q, want %q", name, got, tt.want)
				}
			}
		})
	}
}

func TestChunkedRawModeKeepsBytes(t *testing.T) {
	t.Parallel()
	wires := []string{
		"3\r\nabc\r\n0\r\n\r\n",
		"3;ext\r\nabc\r\n0\r\nX-Trailer: v\r\n\r\n",
		"3\nabc\n0\n\n",
		"1\r\na\r\n1\r\nb\r\n0\r\n\r\n",
		"100000\r\n" + strings.Repeat("x", 1<<20) + "\r\n0\r\n\r\n",
	}
	for _, wire := range wires {
		c := &chunkedReader{br: bufio.NewReaderSize(slowReader{strings.NewReader(wire)}, 4096), emitFraming: true}
		got, err := io.ReadAll(c)
		if err != nil {
			t.Fatalf("%q: %v", truncate(wire), err)
		}
		if string(got) != wire {
			t.Errorf("raw mode lost bytes: got %d bytes, want %d", len(got), len(wire))
		}
	}
}

func TestChunkedErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wire string
		want error
	}{
		{"bad size", "z\r\nabc\r\n", ErrFraming},
		{"missing size", "\r\nabc\r\n", ErrFraming},
		{"truncated data", "5\r\nabc", io.ErrUnexpectedEOF},
		{"truncated size", "3", io.ErrUnexpectedEOF},
		{"missing line break", "3\r\nabcX0\r\n\r\n", ErrFraming},
		{"no final chunk", "3\r\nabc\r\n", io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &chunkedReader{br: bufio.NewReader(strings.NewReader(tt.wire))}
			_, err := io.ReadAll(c)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSplitChunkedBody(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		wire, payload, trailer string
		err                    error
	}{
		{"2\r\nhi\r\n1\r\n!\r\n0\r\nX-Test: a\r\nY-Test: b\r\n\r\n", "hi!", "X-Test: a\r\nY-Test: b\r\n", nil},
		{"2;ext=value\nhi\n0\nX-Test: a\nY-Test: b\n\n", "hi", "X-Test: a\nY-Test: b\n", nil},
		{"0\r\n\r\n", "", "", nil},
		{"3;a=b\nabc\r\n0;last\nX-Test: a\r\nY-Test: b\n\r\n", "abc", "X-Test: a\r\nY-Test: b\n", nil},
		{"", "", "", io.ErrUnexpectedEOF},
		{"0\r\nX-Test: a\r\n", "", "", io.ErrUnexpectedEOF},
		{"0\r\n\r\nextra", "", "", ErrFraming},
	} {
		payload, trailer, err := SplitChunkedBody([]byte(tt.wire))
		if !errors.Is(err, tt.err) || string(payload) != tt.payload || string(trailer) != tt.trailer {
			t.Errorf("SplitChunkedBody(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.wire, payload, trailer, err, tt.payload, tt.trailer, tt.err)
		}
	}
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}
