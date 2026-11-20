package rawhttp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func request(t *testing.T, wire string) *Request {
	t.Helper()
	req, err := ReadRequest(bufio.NewReader(strings.NewReader(wire)), DefaultLimits)
	if err != nil {
		t.Fatalf("ReadRequest(%q): %v", wire, err)
	}
	return req
}

func response(t *testing.T, method, wire string) *Response {
	t.Helper()
	res, err := ReadResponse(bufio.NewReader(strings.NewReader(wire)), method, DefaultLimits)
	if err != nil {
		t.Fatalf("ReadResponse(%q): %v", method, err)
	}
	return res
}

func drain(t *testing.T, m *Message) string {
	t.Helper()
	body, err := io.ReadAll(m.DecodedBody())
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

func TestRequestStartLine(t *testing.T) {
	t.Parallel()
	req := request(t, "POST http://example.com/p?q=1 HTTP/1.1\r\nHost: example.com\r\n\r\n")
	if req.Method() != "POST" || req.Target() != "http://example.com/p?q=1" || req.Version() != "HTTP/1.1" {
		t.Errorf("got %q %q %q", req.Method(), req.Target(), req.Version())
	}
}

func TestHeadersView(t *testing.T) {
	t.Parallel()
	req := request(t, "GET / HTTP/1.1\r\nhOsT: a\r\nX-Pad:   padded value   \r\nX-Pad: second\r\nX-Empty:\r\n\r\n")

	if got, ok := req.Header("host"); !ok || got != "a" {
		t.Errorf("Header(host) = %q, %v", got, ok)
	}
	if got := req.HeaderValues("x-pad"); len(got) != 2 || got[0] != "padded value" || got[1] != "second" {
		t.Errorf("HeaderValues(x-pad) = %q", got)
	}
	if got, ok := req.Header("X-Empty"); !ok || got != "" {
		t.Errorf("Header(X-Empty) = %q, %v", got, ok)
	}
	if _, ok := req.Header("Missing"); ok {
		t.Error("Header(Missing) reported present")
	}
	var names []string
	for _, h := range req.Headers() {
		names = append(names, h.Name)
	}
	if want := []string{"hOsT", "X-Pad", "X-Pad", "X-Empty"}; !equal(names, want) {
		t.Errorf("header order/case %q, want %q", names, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFraming(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		wire    string
		method  string
		framing Framing
		length  int64
		closes  bool
	}{
		{
			name:    "no body",
			wire:    "GET / HTTP/1.1\r\nHost: a\r\n\r\n",
			framing: FramingNone,
			length:  0,
		},
		{
			name:    "content length",
			wire:    "POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello",
			framing: FramingLength,
			length:  5,
		},
		{
			name:    "chunked",
			wire:    "POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
			framing: FramingChunked,
			length:  -1,
		},
		{
			name:    "connection close",
			wire:    "GET / HTTP/1.1\r\nConnection: close\r\n\r\n",
			framing: FramingNone,
			length:  0,
			closes:  true,
		},
		{
			name:    "http 1.0 closes",
			wire:    "GET / HTTP/1.0\r\n\r\n",
			framing: FramingNone,
			length:  0,
			closes:  true,
		},
		{
			name:    "http 1.0 keep-alive",
			wire:    "GET / HTTP/1.0\r\nConnection: Keep-Alive\r\n\r\n",
			framing: FramingNone,
			length:  0,
		},
		{
			name:    "response close delimited",
			wire:    "HTTP/1.0 200 OK\r\n\r\nbody",
			framing: FramingClose,
			length:  -1,
			closes:  true,
		},
		{
			name:    "head response keeps declared length",
			wire:    "HTTP/1.1 200 OK\r\nContent-Length: 42\r\n\r\n",
			method:  "HEAD",
			framing: FramingNone,
			length:  42,
		},
		{
			name:    "304 has no body",
			wire:    "HTTP/1.1 304 Not Modified\r\nContent-Length: 42\r\n\r\n",
			framing: FramingNone,
			length:  42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var msg *Message
			if tt.method != "" || strings.HasPrefix(tt.wire, "HTTP/") {
				msg = &response(t, tt.method, tt.wire).Message
			} else {
				msg = &request(t, tt.wire).Message
			}
			if msg.Framing() != tt.framing {
				t.Errorf("framing = %v, want %v", msg.Framing(), tt.framing)
			}
			if msg.ContentLength() != tt.length {
				t.Errorf("content length = %d, want %d", msg.ContentLength(), tt.length)
			}
			if msg.Close() != tt.closes {
				t.Errorf("close = %v, want %v", msg.Close(), tt.closes)
			}
			drain(t, msg)
		})
	}
}

func TestChunkedBodyViews(t *testing.T) {
	t.Parallel()
	wire := "POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n4;ext=1\r\na\r\nb\r\n0\r\nX-Trailer: 1\r\n\r\n"

	rawReq := request(t, wire)
	rawBody, err := io.ReadAll(rawReq.Body())
	if err != nil {
		t.Fatal(err)
	}
	if want := "4;ext=1\r\na\r\nb\r\n0\r\nX-Trailer: 1\r\n\r\n"; string(rawBody) != want {
		t.Errorf("raw body %q, want %q", rawBody, want)
	}

	decoded := drain(t, &request(t, wire).Message)
	if want := "a\r\nb"; decoded != want {
		t.Errorf("decoded body %q, want %q", decoded, want)
	}
}

func TestWriteTo(t *testing.T) {
	t.Parallel()
	wires := []string{
		"GET / HTTP/1.1\r\nHost: a\r\n\r\n",
		"POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello",
		"POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n",
	}
	for _, wire := range wires {
		var buf bytes.Buffer
		if _, err := request(t, wire).WriteTo(&buf); err != nil {
			t.Fatalf("WriteTo: %v", err)
		}
		if buf.String() != wire {
			t.Errorf("WriteTo wrote %q, want %q", buf.String(), wire)
		}
	}

	closeDelimited := "HTTP/1.0 200 OK\r\n\r\nbody until eof"
	var buf bytes.Buffer
	if _, err := response(t, "", closeDelimited).WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if buf.String() != closeDelimited {
		t.Errorf("WriteTo wrote %q, want %q", buf.String(), closeDelimited)
	}
}

func TestBodyCapture(t *testing.T) {
	t.Parallel()
	wire := "POST / HTTP/1.1\r\nContent-Length: 10\r\n\r\nabcdefghij"

	req, err := ReadRequest(bufio.NewReader(strings.NewReader(wire)), Limits{MaxBodyBytes: 4})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, &req.Message)
	if !req.Truncated() {
		t.Error("Truncated() = false after the capture cap")
	}
	if got, want := string(req.Raw()), wire[:len(wire)-6]; got != want {
		t.Errorf("Raw() = %q, want %q", got, want)
	}

	req, err = ReadRequest(bufio.NewReader(strings.NewReader(wire)), Limits{MaxBodyBytes: -1})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, &req.Message)
	if !req.Truncated() {
		t.Error("Truncated() = false with capture disabled")
	}
	if got, want := req.Raw(), req.Head(); !bytes.Equal(got, want) {
		t.Errorf("Raw() = %q, want the head only", got)
	}
}

func TestHeadLimit(t *testing.T) {
	t.Parallel()
	wire := "GET / HTTP/1.1\r\nX-Big: " + strings.Repeat("a", 200) + "\r\n\r\n"
	if _, err := ReadRequest(bufio.NewReader(strings.NewReader(wire)), Limits{MaxHeadBytes: 64}); !errors.Is(err, ErrHeadTooLarge) {
		t.Fatalf("error = %v, want ErrHeadTooLarge", err)
	}
}

func TestEmptyReader(t *testing.T) {
	t.Parallel()
	if _, err := ReadRequest(bufio.NewReader(strings.NewReader("")), DefaultLimits); !errors.Is(err, io.EOF) {
		t.Fatalf("error = %v, want io.EOF", err)
	}
}

// TestObsFoldLarge is the regression test for the O(n²) fold accumulation: the
// wire is under the 1 MiB head limit, but re-copying the value per fold line
// would take minutes.
func TestObsFoldLarge(t *testing.T) {
	t.Parallel()
	const folds = 200_000
	wire := "GET / HTTP/1.1\r\nHost: a\r\nX-Fold: first\r\n" + strings.Repeat(" x\r\n", folds) + "\r\n"
	req := request(t, wire)

	want := "first" + strings.Repeat(" x", folds)
	if got, ok := req.Header("X-Fold"); !ok || got != want {
		t.Errorf("X-Fold value is %d bytes, want %d", len(got), len(want))
	}
	for _, h := range req.Headers() {
		if h.Name == "X-Fold" && !strings.HasSuffix(h.Raw, " x\r\n") {
			t.Errorf("X-Fold raw is %d bytes, want the whole fold block", len(h.Raw))
		}
	}
}

func TestShortLengthBody(t *testing.T) {
	t.Parallel()
	const wire = "POST / HTTP/1.1\r\nContent-Length: 10\r\n\r\nabc"

	if _, err := io.ReadAll(request(t, wire).Body()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Body() error = %v, want io.ErrUnexpectedEOF", err)
	}
	if _, err := request(t, wire).WriteTo(io.Discard); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("WriteTo error = %v, want io.ErrUnexpectedEOF", err)
	}
	if _, err := io.ReadAll(request(t, "POST / HTTP/1.1\r\nContent-Length: 3\r\n\r\nabc").Body()); err != nil {
		t.Errorf("complete body: error = %v, want nil", err)
	}
}

func TestHeaderCountLimit(t *testing.T) {
	t.Parallel()
	head := func(fields int) string {
		var b strings.Builder
		b.WriteString("GET / HTTP/1.1\r\n")
		for range fields {
			b.WriteString("a:\r\n")
		}
		b.WriteString("\r\n")
		return b.String()
	}
	if _, err := ReadRequest(bufio.NewReader(strings.NewReader(head(maxHeaders+1))), DefaultLimits); !errors.Is(err, ErrHeadTooLarge) {
		t.Errorf("%d fields: error = %v, want ErrHeadTooLarge", maxHeaders+1, err)
	}
	if _, err := ReadRequest(bufio.NewReader(strings.NewReader(head(maxHeaders))), DefaultLimits); err != nil {
		t.Errorf("%d fields: error = %v, want nil", maxHeaders, err)
	}
}

func TestSpaceBeforeColon(t *testing.T) {
	t.Parallel()
	req := request(t, "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding : chunked\r\n\r\n2\r\nhi\r\n0\r\n\r\n")
	if got, ok := req.Header("Transfer-Encoding"); !ok || got != "chunked" {
		t.Errorf("Header(Transfer-Encoding) = %q, %v", got, ok)
	}
	if req.Framing() != FramingChunked {
		t.Errorf("framing = %v, want chunked", req.Framing())
	}
	if !slices.Contains(req.Warnings(), WarnSpaceBeforeColon) {
		t.Errorf("warnings = %q, want %q", req.Warnings(), WarnSpaceBeforeColon)
	}
	if want := "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding : chunked\r\n\r\n"; !bytes.HasPrefix(req.Raw(), []byte(want)) {
		t.Errorf("Raw() lost the odd spacing: %q", req.Raw())
	}
}

func TestLeadingBlankLine(t *testing.T) {
	t.Parallel()
	br := bufio.NewReader(strings.NewReader("\r\nGET / HTTP/1.1\r\nHost: a\r\n\r\nGET /next HTTP/1.1\r\nHost: a\r\n\r\n"))
	first, err := ReadRequest(br, DefaultLimits)
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if first.Target() != "/" {
		t.Errorf("target = %q, want /", first.Target())
	}
	if !slices.Contains(first.Warnings(), WarnLeadingBlankLine) {
		t.Errorf("warnings = %q, want %q", first.Warnings(), WarnLeadingBlankLine)
	}
	if want := "GET / HTTP/1.1\r\nHost: a\r\n\r\n"; string(first.Raw()) != want {
		t.Errorf("Raw() = %q, want %q", first.Raw(), want)
	}
	second, err := ReadRequest(br, DefaultLimits)
	if err != nil {
		t.Fatalf("second ReadRequest: %v", err)
	}
	if second.Target() != "/next" {
		t.Errorf("second target = %q, want /next", second.Target())
	}
}

func TestStatusLineReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		wire, version, reason string
		status                int
	}{
		{wire: "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n", version: "HTTP/1.1", status: 200, reason: "OK"},
		// The code also appears in the version: the reason starts after the
		// code field, not at the first match.
		{wire: "HTTP/200 200 OK\r\nContent-Length: 0\r\n\r\n", version: "HTTP/200", status: 200, reason: "OK"},
		{wire: "HTTP/1.1 204\r\n\r\n", version: "HTTP/1.1", status: 204, reason: ""},
		{wire: "HTTP/1.1 200   spaced reason\r\nContent-Length: 0\r\n\r\n", version: "HTTP/1.1", status: 200, reason: "spaced reason"},
	}
	for _, tt := range tests {
		res := response(t, "", tt.wire)
		if res.Version() != tt.version || res.Status() != tt.status || res.Reason() != tt.reason {
			t.Errorf("%q: got %q %d %q", tt.wire, res.Version(), res.Status(), res.Reason())
		}
	}
}

// RFC 9110 §9.3.6: framing fields on a 2xx to CONNECT are ignored, not validated.
func TestConnectResponseFraming(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		method  string
		wire    string
		wantErr error
	}{
		{"200 transfer-encoding gzip", "CONNECT", "HTTP/1.1 200 OK\r\nTransfer-Encoding: gzip\r\n\r\n", nil},
		{"200 bad content-length", "connect", "HTTP/1.1 200 OK\r\nContent-Length: x\r\n\r\n", nil},
		{"407 bad content-length", "CONNECT", "HTTP/1.1 407 Proxy Auth\r\nContent-Length: x\r\n\r\n", ErrFraming},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := ReadResponse(bufio.NewReader(strings.NewReader(tt.wire)), tt.method, DefaultLimits)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && res.Framing() != FramingNone {
				t.Errorf("framing = %v, want FramingNone", res.Framing())
			}
		})
	}
}
