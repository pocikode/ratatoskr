package rawhttp

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

func FuzzReadRequest(f *testing.F) {
	f.Add([]byte("GET / HTTP/1.1\r\nHost: a\r\n\r\n"))
	f.Add([]byte("POST / HTTP/1.1\r\nContent-Length: 3\r\n\r\nabc"))
	f.Add([]byte("POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n"))
	f.Add([]byte("POST / HTTP/1.1\r\nContent-Length: 5\r\nContent-Length: 5\r\n\r\nhello"))
	f.Add([]byte("GET /\r\n\r\n"))
	f.Add([]byte("GET / HTTP/1.1\nHost: a\n\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		req, err := ReadRequest(bufio.NewReader(bytes.NewReader(data)), DefaultLimits)
		if err != nil {
			return
		}
		checkMessage(t, &req.Message, data)

		second, err := ReadRequest(bufio.NewReader(bytes.NewReader(data)), DefaultLimits)
		if err != nil {
			return
		}
		var buf bytes.Buffer
		if _, err := second.WriteTo(&buf); err != nil {
			return
		}
		if !bytes.Equal(buf.Bytes(), second.Raw()) {
			t.Fatalf("WriteTo and Raw disagree for %q", data)
		}
	})
}

func FuzzReadResponse(f *testing.F) {
	f.Add([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nhi"))
	f.Add([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n"))
	f.Add([]byte("HTTP/1.0 200 OK\r\n\r\nbody"))
	f.Add([]byte("HTTP/1.1 204 No Content\r\n\r\n"))
	f.Add([]byte("HTTP/1.1 100 Continue\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		res, err := ReadResponse(bufio.NewReader(bytes.NewReader(data)), "", DefaultLimits)
		if err != nil {
			return
		}
		checkMessage(t, &res.Message, data)
	})
}

// checkMessage drains the body and asserts the reader never invents, drops or
// reorders bytes: Raw and Head must appear in the input exactly as sent, and
// anything dropped before them must be the blank lines RFC 9112 §2.2 lets a
// parser skip.
func checkMessage(t *testing.T, m *Message, wire []byte) {
	t.Helper()
	if _, err := io.Copy(io.Discard, m.DecodedBody()); err != nil {
		return
	}
	check := func(name string, got []byte) {
		i := bytes.Index(wire, got)
		if i < 0 || len(bytes.Trim(wire[:i], "\r\n")) != 0 {
			t.Fatalf("%s is not the input minus leading blank lines for %q", name, wire)
		}
	}
	check("Raw()", m.Raw())
	check("Head()", m.Head())
}
