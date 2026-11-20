package matchreplace

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

var (
	// errTooLarge means the result would exceed the output cap.
	errTooLarge = errors.New("matchreplace: result over the output cap")
	// errSkip means the rule cannot apply to this message (logged, not an error).
	errSkip = errors.New("matchreplace: rule skipped")
)

// budget is how many bytes the message may still grow under the output cap.
type budget struct{ left int }

// max is the longest replacement allowed for a section of n bytes.
func (b *budget) max(n int) int { return n + b.left }

// take records a section going from before to after bytes.
func (b *budget) take(before, after int) error {
	b.left -= after - before
	if b.left < 0 {
		return errTooLarge
	}
	return nil
}

// apply returns the message c makes of x, or nil when c changes nothing. The
// result is at most limit bytes (errTooLarge), checked while replacing.
func (c *compiled) apply(x *msg, limit int) ([]byte, error) {
	b := &budget{left: limit - len(x.raw)}
	switch c.section {
	case RequestFirstLine, ResponseFirstLine:
		return c.firstLine(x, b)
	case RequestMethod:
		return c.method(x, b)
	case ResponseStatusCode:
		return c.status(x, b)
	case RequestPath, RequestQuery:
		return c.target(x, b)
	case RequestHeader, ResponseHeader:
		return c.headers(x, b)
	case RequestBody, ResponseBody:
		return c.body(x, b)
	}
	return nil, fmt.Errorf("matchreplace: unknown section %q", c.section)
}

func (c *compiled) firstLine(x *msg, b *budget) ([]byte, error) {
	line, eol := x.startLine()
	out, err := c.replace(line, b.max(len(line)))
	if err != nil || bytes.Equal(out, line) {
		return nil, err
	}
	return x.with(slices.Concat(out, eol, x.head()[len(line)+len(eol):]), x.body), nil
}

func (c *compiled) method(x *msg, b *budget) ([]byte, error) {
	tok := []byte(x.req.Method())
	out, err := c.replace(tok, b.max(len(tok)))
	if err != nil || bytes.Equal(out, tok) {
		return nil, err
	}
	return x.with(x.m.Builder().SetMethod(string(out)).Head(), x.body), nil
}

func (c *compiled) status(x *msg, b *budget) ([]byte, error) {
	tok := fmt.Appendf(nil, "%03d", x.res.Status())
	out, err := c.replace(tok, b.max(len(tok)))
	if err != nil || bytes.Equal(out, tok) {
		return nil, err
	}
	if len(out) != 3 || bytes.ContainsFunc(out, func(r rune) bool { return r < '0' || r > '9' }) {
		return nil, fmt.Errorf("%w: status result %q is not three digits", errSkip, out)
	}
	code, _ := strconv.Atoi(string(out))
	return x.with(x.m.Builder().SetStatus(code, x.res.Reason()).Head(), x.body), nil
}

// target handles the path and the query of the request target.
func (c *compiled) target(x *msg, b *budget) ([]byte, error) {
	t := splitTarget(x.req.Target())
	var changed bool
	var err error
	switch {
	case c.section == RequestPath:
		t.path, changed, err = c.replaceString(t.path, b)
	case c.op == UpdateRaw:
		t.query, changed, err = c.replaceString(t.query, b)
		t.hasQ = t.hasQ || t.query != ""
	default:
		t.query, changed, err = c.params(t.query, b)
		t.hasQ = t.query != ""
	}
	if err != nil || !changed {
		return nil, err
	}
	return x.with(x.m.Builder().SetTarget(t.String()).Head(), x.body), nil
}

func (c *compiled) replaceString(s string, b *budget) (string, bool, error) {
	out, err := c.replace([]byte(s), b.max(len(s)))
	if err != nil || string(out) == s {
		return s, false, err
	}
	return string(out), true, b.take(len(s), len(out))
}

// params applies UPDATE_VALUE, ADD or REMOVE to the &-separated params of q.
func (c *compiled) params(q string, b *budget) (string, bool, error) {
	var items []string
	if q != "" {
		items = strings.Split(q, "&")
	}
	out := items[:0:0]
	changed := false
	for _, it := range items {
		name, value, _ := strings.Cut(it, "=")
		switch c.op {
		case UpdateValue:
			if c.kind == Name && name != c.match || c.kind != Name && !c.hits(value) {
				break
			}
			v, err := c.replace([]byte(value), b.max(len(value)))
			if err != nil {
				return "", false, err
			}
			if next := name + "=" + string(v); next != it {
				if err := b.take(len(it), len(next)); err != nil {
					return "", false, err
				}
				out, changed = append(out, next), true
				continue
			}
		case Remove:
			if c.kind == Name && name == c.match || c.kind != Name && c.hits(it) {
				changed = true
				continue
			}
		}
		out = append(out, it)
	}
	if c.op == Add {
		if err := b.take(0, len(c.term)+1); err != nil {
			return "", false, err
		}
		out, changed = append(out, string(c.term)), true
	}
	return strings.Join(out, "&"), changed, nil
}

// headers applies UPDATE_RAW, UPDATE_VALUE, ADD or REMOVE to the header fields.
func (c *compiled) headers(x *msg, b *budget) ([]byte, error) {
	hs := make([]rawhttp.Header, 0, x.headers+1)
	changed := false
	for _, h := range x.m.Headers() {
		text, eol := cutEOL(h.Raw)
		switch c.op {
		case UpdateRaw:
			if !c.hits(text) {
				break
			}
			out, err := c.replace([]byte(text), b.max(len(text)))
			if err != nil {
				return nil, err
			}
			if string(out) == text {
				break
			}
			if err := b.take(len(text), len(out)); err != nil {
				return nil, err
			}
			changed = true
			if len(out) > 0 { // an emptied line is dropped, not left as a blank line
				hs = append(hs, rawhttp.Header{Name: h.Name, Value: h.Value, Raw: string(out) + eol})
			}
			continue
		case UpdateValue:
			if c.kind == Name && !strings.EqualFold(h.Name, c.match) || c.kind != Name && !c.hits(h.Value) {
				break
			}
			v, err := c.replace([]byte(h.Value), b.max(len(h.Value)))
			if err != nil {
				return nil, err
			}
			if string(v) == h.Value {
				break
			}
			if err := b.take(len(h.Value), len(v)); err != nil {
				return nil, err
			}
			hs, changed = append(hs, rawhttp.Header{Name: h.Name, Value: string(v)}), true
			continue
		case Remove:
			if c.kind == Name && strings.EqualFold(h.Name, c.match) || c.kind != Name && c.hits(text) {
				changed = true
				continue
			}
		}
		hs = append(hs, h)
	}
	if c.op == Add {
		if err := b.take(0, len(c.term)+2); err != nil {
			return nil, err
		}
		hs, changed = append(hs, rawhttp.Header{Name: c.addHeader[0], Value: c.addHeader[1]}), true
	}
	if !changed {
		return nil, nil
	}
	return x.with(x.m.Builder().SetHeaders(hs).Head(), x.body), nil
}

// hits reports whether a FULL, STRING or REGEX matcher finds anything in text
// (cheap check before replacing, and the REMOVE test).
func (c *compiled) hits(text string) bool {
	switch c.kind {
	case String:
		return strings.Contains(text, c.match)
	case Regex:
		return c.re.MatchString(text)
	}
	return true
}

// replace applies the matcher and term to src: FULL replaces it whole, STRING
// every occurrence, REGEX every match with $1, ${name} and $$ expanded. The
// result is at most limit bytes (errTooLarge), checked before it is built. It
// may alias src or the term.
func (c *compiled) replace(src []byte, limit int) ([]byte, error) {
	switch c.kind {
	case String:
		n := bytes.Count(src, c.value)
		if n == 0 {
			return src, nil
		}
		if len(src)+n*(len(c.term)-len(c.value)) > limit {
			return nil, errTooLarge
		}
		return bytes.ReplaceAll(src, c.value, c.term), nil
	case Regex:
		// Mirrors regexp's FindAll loop one match at a time, so memory stays
		// bounded by the output cap however dense the matches are.
		var dst []byte
		last, found := 0, false
		for pos, prevEnd := 0, -1; pos <= len(src); {
			m := c.findAt(src, pos)
			if m == nil {
				break
			}
			accept := true
			if m[1] == pos { // empty match: never right after the previous match
				accept = m[0] != prevEnd
				_, w := utf8.DecodeRune(src[pos:])
				pos += max(w, 1)
			} else {
				pos = m[1]
			}
			prevEnd = m[1]
			if !accept {
				continue
			}
			found = true
			dst = append(dst, src[last:m[0]]...)
			dst = c.re.Expand(dst, c.term, src, m)
			last = m[1]
			if len(dst)+len(src)-last > limit {
				return nil, errTooLarge
			}
		}
		if !found {
			return src, nil
		}
		return append(dst, src[last:]...), nil
	}
	if len(c.term) > limit {
		return nil, errTooLarge
	}
	return c.term, nil
}

// findAt returns the leftmost match of c.re in src at or after pos, with the
// text before pos as context for ^, \b and \B, as regexp's FindAll sees it.
// Slicing src at pos would lose that context, so after the start c.resume
// (one rune, then the rule's regex as group 1) is run from the rune before pos.
func (c *compiled) findAt(src []byte, pos int) []int {
	if pos == 0 {
		return c.re.FindSubmatchIndex(src)
	}
	_, w := utf8.DecodeLastRune(src[:pos])
	m := c.resume.FindSubmatchIndex(src[pos-w:])
	if m == nil {
		return nil
	}
	m = m[2:]
	for i, v := range m {
		if v >= 0 {
			m[i] = v + pos - w
		}
	}
	return m
}

// cutEOL splits a raw header line into its text and line terminator.
func cutEOL(raw string) (text, eol string) {
	if s, ok := strings.CutSuffix(raw, "\r\n"); ok {
		return s, "\r\n"
	}
	if s, ok := strings.CutSuffix(raw, "\n"); ok {
		return s, "\n"
	}
	return raw, ""
}
