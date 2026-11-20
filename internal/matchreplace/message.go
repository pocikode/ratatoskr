package matchreplace

import (
	"bytes"
	"io"
	"net"
	"strings"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// limits parse heads with the default budget and keep no body copy: the body
// is located in the input buffer instead.
var limits = rawhttp.Limits{MaxBodyBytes: -1}

// msg is one message parsed from a buffer: the bytes, the rawhttp view of its
// head, and where its framed body and any bytes after it sit.
type msg struct {
	raw     []byte
	resp    bool
	method  string // for a response, the request method it answers ("" = GET)
	m       *rawhttp.Message
	req     *rawhttp.Request
	res     *rawhttp.Response
	pre     []byte // blank lines before the start line (rawhttp skips them)
	body    []byte // the framed body as on the wire; aliases raw
	rest    []byte // bytes after the framed body; kept, never body
	headers int    // number of header fields
}

// parse reads one message from raw. It fails when the head does not parse or
// the framed body does not end inside raw.
func parse(raw []byte, resp bool, method string) (*msg, error) {
	x := &msg{raw: raw, resp: resp, method: method}
	var headEnd int
	if resp {
		r, n, err := rawhttp.ParseResponse(raw, method, limits)
		if err != nil {
			return nil, err
		}
		x.res, x.m, headEnd = r, &r.Message, n
	} else {
		r, n, err := rawhttp.ParseRequest(raw, limits)
		if err != nil {
			return nil, err
		}
		x.req, x.m, headEnd = r, &r.Message, n
	}
	for range x.m.Headers() {
		x.headers++
	}
	n, err := io.Copy(io.Discard, x.m.Body())
	if err != nil {
		return nil, err
	}
	end := headEnd + int(n)
	x.pre = raw[:headEnd-len(x.m.Head())]
	x.body = raw[headEnd:end]
	x.rest = raw[end:]
	return x, nil
}

// with returns the message rebuilt from a new head and body, the leading blank
// lines and trailing bytes kept.
func (x *msg) with(head, body []byte) []byte {
	out := make([]byte, 0, len(x.pre)+len(head)+len(body)+len(x.rest))
	out = append(out, x.pre...)
	out = append(out, head...)
	out = append(out, body...)
	return append(out, x.rest...)
}

// head returns the head bytes as received.
func (x *msg) head() []byte { return x.m.Head() }

// whole is the message without the bytes after its framed body.
func (x *msg) whole() []byte { return x.raw[:len(x.raw)-len(x.rest)] }

// startLine returns the start line without its terminator, and the terminator.
func (x *msg) startLine() (line, eol []byte) {
	h := x.head()
	i := bytes.IndexByte(h, '\n')
	line, eol = h[:i], h[i:i+1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line, eol = line[:len(line)-1], h[i-1:i+1]
	}
	return line, eol
}

// target is a request target split for path and query edits.
type target struct {
	prefix string // scheme://authority of an absolute-form target
	path   string
	query  string // without "?"
	hasQ   bool
	frag   string // "#..." or ""
}

func splitTarget(t string) target {
	var r target
	// Absolute-form only when "scheme://" leads: an origin-form query may
	// itself hold a URL (/login?next=https://...).
	if i := strings.Index(t, "://"); i > 0 && !strings.ContainsAny(t[:i], "/?#") {
		j := strings.IndexAny(t[i+3:], "/?#")
		if j < 0 {
			return target{prefix: t}
		}
		r.prefix, t = t[:i+3+j], t[i+3+j:]
	}
	if i := strings.IndexByte(t, '#'); i >= 0 {
		t, r.frag = t[:i], t[i:]
	}
	r.path, r.query, r.hasQ = strings.Cut(t, "?")
	return r
}

func (t target) String() string {
	s := t.prefix + t.path
	if t.hasQ {
		s += "?" + t.query
	}
	return s + t.frag
}

// host is req.host: the Host header, else the authority of an absolute-form
// target, without the port.
func host(r *rawhttp.Request) string {
	h, ok := r.Header("Host")
	if !ok {
		_, h, _ = strings.Cut(splitTarget(r.Target()).prefix, "://")
		if i := strings.LastIndexByte(h, '@'); i >= 0 {
			h = h[i+1:]
		}
	}
	if name, _, err := net.SplitHostPort(h); err == nil {
		return name
	}
	return h
}
