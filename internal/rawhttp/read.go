package rawhttp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Limits bounds what a reader buffers. The zero value is usable: it applies
// [DefaultLimits].
type Limits struct {
	// MaxHeadBytes caps the start line plus every header field.
	MaxHeadBytes int
	// MaxBodyBytes caps how many body bytes are kept for [Message.Raw]. A
	// negative value disables capture; zero applies [DefaultLimits].
	MaxBodyBytes int64
}

// maxHeaders bounds the structured header view, so a head packed with one-letter
// fields cannot turn the 1 MiB budget into tens of megabytes of Header values.
const maxHeaders = 1000

// DefaultLimits are the budgets from docs/03-architecture.md §4: a 1 MiB head
// and a 64 MiB body capture cap.
var DefaultLimits = Limits{MaxHeadBytes: 1 << 20, MaxBodyBytes: 64 << 20}

func (l Limits) normalized() Limits {
	if l.MaxHeadBytes == 0 {
		l.MaxHeadBytes = DefaultLimits.MaxHeadBytes
	}
	if l.MaxBodyBytes == 0 {
		l.MaxBodyBytes = DefaultLimits.MaxBodyBytes
	}
	return l
}

// ReadRequest reads a request head from br, leaving br positioned at the first
// byte of the body. The body is not read.
func ReadRequest(br *bufio.Reader, limits Limits) (*Request, error) {
	r := &Request{}
	if err := readHead(&r.Message, br, limits, kindRequest, ""); err != nil {
		return nil, err
	}
	return r, nil
}

// ReadResponse reads a response head from br, leaving br positioned at the
// first byte of the body. method is the request method ("" means GET) and
// decides whether the response may carry a body: HEAD, 1xx, 204 and 304 have
// none.
func ReadResponse(br *bufio.Reader, method string, limits Limits) (*Response, error) {
	r := &Response{}
	if err := readHead(&r.Message, br, limits, kindResponse, responseMethod(method)); err != nil {
		return nil, err
	}
	return r, nil
}

// responseMethod names the request method a response belongs to; the body rules
// need it before the status line is even read.
func responseMethod(method string) string {
	if method == "" {
		return "GET"
	}
	return strings.ToUpper(method)
}

func readHead(m *Message, br *bufio.Reader, limits Limits, k kind, method string) error {
	limits = limits.normalized()
	raw, skipped, err := readHeadBytes(br, limits.MaxHeadBytes)
	if err != nil {
		return err
	}
	return m.init(raw, skipped, limits, k, method, br)
}

// ParseRequest is [ReadRequest] over a buffer: it parses the request head at
// the start of b without copying it and leaves the body in b. [Message.Head]
// aliases b, so b must not change while the message is in use; the body is
// read from b[n:], where n is the end of the head (leading blank lines
// included). Results and errors are those of ReadRequest on the same bytes.
func ParseRequest(b []byte, limits Limits) (r *Request, n int, err error) {
	r = &Request{}
	if n, err = parseBuffer(&r.Message, b, limits, kindRequest, ""); err != nil {
		return nil, 0, err
	}
	return r, n, nil
}

// ParseResponse is [ReadResponse] over a buffer, as [ParseRequest] is for
// requests.
func ParseResponse(b []byte, method string, limits Limits) (r *Response, n int, err error) {
	r = &Response{}
	if n, err = parseBuffer(&r.Message, b, limits, kindResponse, responseMethod(method)); err != nil {
		return nil, 0, err
	}
	return r, n, nil
}

func parseBuffer(m *Message, b []byte, limits Limits, k kind, method string) (int, error) {
	limits = limits.normalized()
	start, end, err := headSpan(b, limits.MaxHeadBytes)
	if err != nil {
		return 0, err
	}
	if err := m.init(b[start:end:end], start > 0, limits, k, method, nil); err != nil {
		return 0, err
	}
	if m.framing != FramingNone {
		// Length and close-delimited bodies read straight through bufio's
		// minimum buffer; chunk framing lines want a full one.
		size := 16
		if m.framing == FramingChunked {
			size = 4096
		}
		m.br = bufio.NewReaderSize(bytes.NewReader(b[end:]), size)
	}
	return end, nil
}

// init sets up m from its head bytes; the body streams from br.
func (m *Message) init(raw []byte, skipped bool, limits Limits, k kind, method string, br *bufio.Reader) error {
	*m = Message{
		raw:          raw,
		kind:         k,
		method:       method,
		br:           br,
		captureLimit: limits.MaxBodyBytes,
		length:       -1,
	}
	if skipped {
		m.warn(WarnLeadingBlankLine)
	}
	if err := m.parseHead(); err != nil {
		return err
	}
	if err := m.frame(); err != nil {
		return err
	}
	m.setClose()
	return nil
}

// headSpan finds the head at the start of b as readHeadBytes reads it from a
// reader holding b: b[start:end] is the head, b[:start] the skipped blank
// lines. It returns the same errors.
func headSpan(b []byte, budget int) (start, end int, err error) {
	inHead := false
	for at := 0; ; {
		left := budget - at
		if left <= 0 {
			return 0, 0, ErrHeadTooLarge
		}
		i := bytes.IndexByte(b[at:], '\n')
		if i < 0 {
			switch {
			case len(b)-at > left:
				return 0, 0, ErrHeadTooLarge
			case len(b) > at:
				return 0, 0, io.ErrUnexpectedEOF
			case inHead:
				return 0, 0, fmt.Errorf("rawhttp: head: %w", io.ErrUnexpectedEOF)
			}
			return 0, 0, io.EOF
		}
		line := b[at : at+i+1]
		if len(line) > left {
			return 0, 0, ErrHeadTooLarge
		}
		switch {
		case !inHead && isBlank(line):
			start = at + len(line)
		case !inHead:
			inHead = true
		case isBlank(line):
			return start, at + len(line), nil
		}
		at += len(line)
	}
}

// readHeadBytes reads up to and including the empty line that ends a message
// head. Blank lines before the start line are skipped, not kept in the head:
// RFC 9112 §2.2 lets a parser ignore them, and keep-alive clients do send a
// stray one after a body. skipped reports that there was at least one.
func readHeadBytes(br *bufio.Reader, budget int) (raw []byte, skipped bool, err error) {
	used := 0
	for {
		line, err := readLine(br, budget-used)
		if err != nil {
			if len(raw) == 0 {
				return nil, skipped, err
			}
			if errors.Is(err, io.EOF) {
				return nil, skipped, fmt.Errorf("rawhttp: head: %w", io.ErrUnexpectedEOF)
			}
			return nil, skipped, err
		}
		used += len(line)
		if len(raw) == 0 && isBlank(line) {
			skipped = true
			continue
		}
		raw = append(raw, line...)
		if isBlank(line) {
			return raw, skipped, nil
		}
	}
}

// readLine reads one line, terminator included, tolerating a bare LF. budget is
// how many more bytes the head may take.
func readLine(br *bufio.Reader, budget int) ([]byte, error) {
	if budget <= 0 {
		return nil, ErrHeadTooLarge
	}
	var line []byte
	for {
		frag, err := br.ReadSlice('\n')
		if len(line)+len(frag) > budget {
			return nil, ErrHeadTooLarge
		}
		line = append(line, frag...)
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case err == nil:
			return line, nil
		case errors.Is(err, io.EOF) && len(line) > 0:
			return line, io.ErrUnexpectedEOF
		default:
			return nil, err
		}
	}
}

func isBlank(line []byte) bool {
	return len(bytes.TrimRight(line, "\r\n")) == 0
}

// lineAt splits the line starting at at. lineEnd excludes the terminator, next
// is the offset of the following line, bare reports a terminally missing CR.
func lineAt(raw []byte, at int) (lineEnd, next int, bare, ok bool) {
	i := bytes.IndexByte(raw[at:], '\n')
	if i < 0 {
		return 0, len(raw), false, false
	}
	lineEnd, next, bare = at+i, at+i+1, true
	if lineEnd > at && raw[lineEnd-1] == '\r' {
		lineEnd, bare = lineEnd-1, false
	}
	return lineEnd, next, bare, true
}

func (m *Message) parseHead() error {
	lineEnd, next, bare, ok := lineAt(m.raw, 0)
	if !ok {
		return fmt.Errorf("rawhttp: %w: unterminated start line", ErrMalformed)
	}
	if bare {
		m.warn(WarnBareLF)
	}
	// One string copy of the head; every name, value and Raw is a substring
	// of it, so a field costs no allocation unless it is folded.
	head := string(m.raw)
	if err := m.parseStartLine(head[:lineEnd]); err != nil {
		return err
	}
	m.lineEnd = next

	cur, curStart := -1, 0
	var value string  // value of the field at cur
	var folded []byte // value of the field at cur once an obs-fold joined it

	// endHeader records the field's value and its raw bytes once its last line
	// is known: a block of folds then costs one append per line, not a copy of
	// the whole field each time (T-010 review, blocker).
	endHeader := func(end int) {
		if cur < 0 {
			return
		}
		if folded != nil {
			value = string(folded)
		}
		m.headers[cur].Value = value
		m.headers[cur].Raw = head[curStart:end]
		cur, folded = -1, nil
	}

	for at := next; ; {
		lineEnd, next, bare, ok := lineAt(m.raw, at)
		if !ok {
			return fmt.Errorf("rawhttp: head: %w", io.ErrUnexpectedEOF)
		}
		if bare {
			m.warn(WarnBareLF)
		}
		switch line := head[at:lineEnd]; {
		case len(line) == 0:
			endHeader(at)
			m.bodyAt = at
			return nil
		case line[0] == ' ' || line[0] == '\t':
			// obs-fold: RFC 9112 §5.2 allows a parser to replace it with a
			// single space, which is what the structured view does. Raw keeps
			// the folded bytes.
			m.warn(WarnObsFold)
			if cur < 0 {
				m.warn(WarnMalformedHeader)
			} else {
				if folded == nil {
					folded = append([]byte(nil), value...)
				}
				folded = append(append(folded, ' '), trimOWS(line)...)
			}
		default:
			endHeader(at)
			name, rest, found := strings.Cut(line, ":")
			if !found {
				m.warn(WarnMalformedHeader)
				break
			}
			if trimmed := trimOWS(name); len(trimmed) != len(name) {
				// RFC 9112 §5.1 rejects a space between the name and the
				// colon; frame on the name the peer meant instead of ignoring
				// the field (a smuggling vector).
				m.warn(WarnSpaceBeforeColon)
				name = trimmed
			}
			if len(name) == 0 {
				m.warn(WarnMalformedHeader)
				break
			}
			if len(m.headers) >= maxHeaders {
				return ErrHeadTooLarge
			}
			value = trimOWS(rest)
			m.headers = append(m.headers, Header{Name: name})
			cur, curStart = len(m.headers)-1, at
		}
		at = next
	}
}

func trimOWS(s string) string { return strings.Trim(s, " \t") }

func (m *Message) parseStartLine(line string) error {
	if m.kind == kindResponse {
		return m.parseStatusLine(line)
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return fmt.Errorf("rawhttp: %w: request line %q", ErrMalformed, line)
	}
	m.method = fields[0]
	m.target = fields[1]
	if len(fields) < 3 {
		m.warn(WarnNoVersion)
	} else {
		m.version = fields[2]
	}
	if m.method != strings.ToUpper(m.method) {
		m.warn(WarnLowerCaseMethod)
	}
	return nil
}

func (m *Message) parseStatusLine(line string) error {
	versionStart, versionEnd := nextField(line, 0)
	codeStart, codeEnd := nextField(line, versionEnd)
	if versionStart == versionEnd || codeStart == codeEnd {
		return fmt.Errorf("rawhttp: %w: status line %q", ErrMalformed, line)
	}
	code, err := parseCode(line[codeStart:codeEnd])
	if err != nil {
		return fmt.Errorf("rawhttp: %w: status line %q", ErrMalformed, line)
	}
	m.version = line[versionStart:versionEnd]
	m.status = code
	m.reason = trimOWS(line[codeEnd:])
	if m.version != "HTTP/1.1" && m.version != "HTTP/1.0" {
		m.warn(WarnBadVersion)
	}
	return nil
}

// nextField returns the span of the space- or tab-separated field at or after
// at. An empty span means the line has no field left.
func nextField(line string, at int) (start, end int) {
	for at < len(line) && (line[at] == ' ' || line[at] == '\t') {
		at++
	}
	start = at
	for at < len(line) && line[at] != ' ' && line[at] != '\t' {
		at++
	}
	return start, at
}

func parseCode(b string) (int, error) {
	if len(b) != 3 {
		return 0, ErrMalformed
	}
	n := 0
	for _, c := range []byte(b) {
		if c < '0' || c > '9' {
			return 0, ErrMalformed
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func (m *Message) http10() bool { return m.version != "HTTP/1.1" }

// frame decides where the body ends and records the choices it had to make.
// net/http's choices are the reference for ambiguous input (task T-010).
func (m *Message) frame() error {
	// A 2xx answer to CONNECT turns the connection into a tunnel; the client
	// MUST ignore Content-Length and Transfer-Encoding (RFC 9110 §9.3.6).
	if m.kind == kindResponse && m.method == "CONNECT" && m.status/100 == 2 {
		m.framing = FramingNone
		return nil
	}

	var contentLengths, encodings []string
	for _, h := range m.headers {
		switch {
		case strings.EqualFold(h.Name, "Content-Length"):
			contentLengths = append(contentLengths, h.Value)
		case strings.EqualFold(h.Name, "Transfer-Encoding"):
			encodings = append(encodings, h.Value)
		}
	}

	// Issue 12785: net/http ignores Transfer-Encoding below HTTP/1.1.
	if len(encodings) > 0 && m.http10() {
		m.warn(WarnTransferEncodingOnHTTP10)
		encodings = nil
	}
	chunked, err := parseTransferEncoding(encodings)
	if err != nil {
		return err
	}
	m.warnTransferEncodingCount(encodings)
	n, hasLength, err := m.contentLength(contentLengths)
	if err != nil {
		return err
	}
	if hasLength {
		m.length = n
	}

	// Responses to HEAD and 1xx/204/304 have no body whatever the fields say
	// (RFC 9112 §6.3); net/http makes the same call before looking at the
	// Transfer-Encoding, and keeps the declared length.
	if m.kind == kindResponse && noResponseBody(m.status, m.method) {
		m.framing = FramingNone
		return nil
	}

	if chunked {
		if hasLength {
			// RFC 9112 §6.3: Transfer-Encoding wins; net/http drops the CL.
			m.warn(WarnContentLengthWithTransferEncoding)
			m.length = -1
		}
		m.framing = FramingChunked
		return nil
	}

	switch {
	case hasLength:
		m.framing = FramingLength
	case m.kind == kindResponse:
		m.framing = FramingClose
		m.length = -1
	default:
		m.framing = FramingNone
		m.length = 0
	}
	return nil
}

// noResponseBody reports the responses that never carry a body. A 2xx answer to
// CONNECT is handled at the top of frame, before the framing fields are checked;
// the tunnel itself belongs to the connection loop (T-011).
func noResponseBody(status int, method string) bool {
	return status/100 == 1 || status == 204 || status == 304 || method == "HEAD"
}

func (m *Message) warnTransferEncodingCount(encodings []string) {
	tokens := 0
	for _, v := range encodings {
		for tok := range strings.SplitSeq(v, ",") {
			if strings.TrimSpace(tok) != "" {
				tokens++
			}
		}
	}
	if tokens > 1 {
		m.warn(WarnMultipleTransferEncodings)
	}
}

// parseTransferEncoding reports whether the body is chunked. The last coding
// must be chunked; anything else leaves the body boundary unknown.
func parseTransferEncoding(encodings []string) (bool, error) {
	last := ""
	for _, v := range encodings {
		for tok := range strings.SplitSeq(v, ",") {
			if tok = strings.TrimSpace(tok); tok != "" {
				last = tok
			}
		}
	}
	switch {
	case last == "":
		return false, nil
	case strings.EqualFold(last, "chunked"):
		return true, nil
	default:
		return false, fmt.Errorf("rawhttp: %w: unsupported Transfer-Encoding %q", ErrFraming, last)
	}
}

// contentLength parses every Content-Length field. Identical duplicates are
// deduped like net/http does; conflicting ones are refused, because picking one
// of them is the request-smuggling primitive we must not invent.
func (m *Message) contentLength(values []string) (int64, bool, error) {
	if len(values) == 0 {
		return 0, false, nil
	}
	first, err := parseDecimal(strings.TrimSpace(values[0]))
	if err != nil {
		return 0, false, err
	}
	for _, v := range values[1:] {
		n, err := parseDecimal(strings.TrimSpace(v))
		if err != nil {
			return 0, false, err
		}
		if n != first {
			return 0, false, fmt.Errorf("rawhttp: %w: conflicting Content-Length values %q", ErrFraming, values)
		}
	}
	if len(values) > 1 {
		m.warn(WarnDuplicateContentLength)
	}
	return first, true, nil
}

func parseDecimal(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("rawhttp: %w: empty Content-Length", ErrFraming)
	}
	var n int64
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("rawhttp: %w: Content-Length %q", ErrFraming, s)
		}
		if n > (1<<63-1-int64(c-'0'))/10 {
			return 0, fmt.Errorf("rawhttp: %w: Content-Length %q overflows", ErrFraming, s)
		}
		n = n*10 + int64(c-'0')
	}
	return n, nil
}

func (m *Message) setClose() {
	connection := m.HeaderValues("Connection")
	switch {
	case m.framing == FramingClose:
		m.close = true
	case hasToken(connection, "close"):
		m.close = true
	case m.http10() && !hasToken(connection, "keep-alive"):
		m.close = true
	}
}

func hasToken(values []string, token string) bool {
	for _, v := range values {
		for tok := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), token) {
				return true
			}
		}
	}
	return false
}
