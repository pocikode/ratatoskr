// Package scope implements scope presets: allow/deny lists of host globs with
// optional port and path prefix (docs/01-feature-spec.md F-SCOPE-1).
package scope

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Rule is one parsed allow/deny entry: host glob, optional port, optional path prefix.
type Rule struct {
	Host string // lowercase ASCII glob; * any run, ? one character
	Port int    // 0 = any port
	Path string // "" = any path, else a prefix starting with "/"
}

// ParseRule parses "host[:port][/path]". IPv6 literals are bracketed:
// "[::1]:8080". Hosts are case-insensitive; non-ASCII labels become punycode.
func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, errors.New("empty scope entry")
	}

	var r Rule

	var rest string
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return Rule{}, fmt.Errorf("scope entry %q: missing ']'", s)
		}

		r.Host, rest = s[1:end], s[end+1:]
		if a, err := netip.ParseAddr(r.Host); err == nil {
			r.Host = a.String()
		} else if !strings.ContainsAny(r.Host, "*?") {
			return Rule{}, fmt.Errorf("scope entry %q: bad IPv6 address", s)
		}
	} else {
		end := strings.IndexAny(s, ":/")
		if end < 0 {
			end = len(s)
		}

		r.Host, rest = s[:end], s[end:]
	}

	if r.Host == "" {
		return Rule{}, fmt.Errorf("scope entry %q: empty host", s)
	}

	var err error
	if r.Host, err = toASCII(strings.ToLower(r.Host)); err != nil {
		return Rule{}, fmt.Errorf("scope entry %q: %w", s, err)
	}

	if after, ok := strings.CutPrefix(rest, ":"); ok {
		portStr, p, _ := strings.Cut(after, "/")
		if p != "" || strings.HasSuffix(after, "/") {
			rest = "/" + p
		} else {
			rest = ""
		}

		n, err := strconv.Atoi(portStr)
		if err != nil || n < 1 || n > 65535 {
			return Rule{}, fmt.Errorf("scope entry %q: bad port %q", s, portStr)
		}

		r.Port = n
	}

	if rest != "" {
		if rest[0] != '/' {
			return Rule{}, fmt.Errorf("scope entry %q: unexpected %q", s, rest)
		}

		r.Path = rest
	}

	return r, nil
}

// Match reports whether host (any case, IPv6 without brackets), port and
// path fall under r. A zero port never matches a rule with a port.
func (r Rule) Match(host string, port int, path string) bool {
	if r.Port != 0 && r.Port != port {
		return false
	}

	if r.Path != "" && !strings.HasPrefix(path, r.Path) {
		return false
	}

	host = strings.ToLower(host)
	if a, err := netip.ParseAddr(host); err == nil {
		host = a.String()
	}

	return globMatch(r.Host, host)
}

// globMatch matches * (any run, dots included) and ? (one byte) in pattern.
func globMatch(pat, s string) bool {
	px, sx := 0, 0
	star, mark := -1, 0

	for sx < len(s) {
		switch {
		case px < len(pat) && pat[px] == '*':
			star, mark = px, sx
			px++
		case px < len(pat) && (pat[px] == '?' || pat[px] == s[sx]):
			px++
			sx++
		case star >= 0:
			mark++
			px, sx = star+1, mark
		default:
			return false
		}
	}

	for px < len(pat) && pat[px] == '*' {
		px++
	}

	return px == len(pat)
}

// Matcher is a compiled allow/deny list.
type Matcher struct{ allow, deny []Rule }

// NewMatcher parses the entries; the first bad entry is returned as an error.
func NewMatcher(allow, deny []string) (*Matcher, error) {
	var m Matcher

	for _, e := range []struct {
		src []string
		dst *[]Rule
	}{{allow, &m.allow}, {deny, &m.deny}} {
		for _, s := range e.src {
			r, err := ParseRule(s)
			if err != nil {
				return nil, err
			}

			*e.dst = append(*e.dst, r)
		}
	}

	return &m, nil
}

// InScope reports whether the target is allowed: no deny rule matches, and
// the allowlist is empty or one of its rules matches. A nil Matcher allows everything.
func (m *Matcher) InScope(host string, port int, path string) bool {
	if m == nil {
		return true
	}

	for _, r := range m.deny {
		if r.Match(host, port, path) {
			return false
		}
	}

	if len(m.allow) == 0 {
		return true
	}

	for _, r := range m.allow {
		if r.Match(host, port, path) {
			return true
		}
	}

	return false
}

// toASCII converts each non-ASCII label to its punycode form (RFC 3492),
// leaving glob characters alone.
func toASCII(host string) (string, error) {
	if !hasNonASCII(host) {
		return host, nil
	}

	labels := strings.Split(host, ".")
	for i, l := range labels {
		if !hasNonASCII(l) {
			continue
		}

		p, err := punycode(l)
		if err != nil {
			return "", err
		}

		labels[i] = "xn--" + p
	}

	return strings.Join(labels, "."), nil
}

func hasNonASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return true
		}
	}

	return false
}

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// punycode encodes one label (RFC 3492 section 6.3).
func punycode(label string) (string, error) {
	const base, tmin, tmax, skew, damp = 36, 1, 26, 38, 700

	in := []rune(label)

	var out []byte

	for _, r := range in {
		if r < utf8.RuneSelf {
			out = append(out, byte(r))
		}
	}

	h := len(out)
	b := h

	if b > 0 {
		out = append(out, '-')
	}

	digit := func(d int) byte { return alphabet[d] }

	n, delta, bias := 128, 0, 72

	for h < len(in) {
		m := int(^uint32(0) >> 1)

		for _, r := range in {
			if int(r) >= n && int(r) < m {
				m = int(r)
			}
		}

		if m-n > (int(^uint32(0)>>1)-delta)/(h+1) {
			return "", errors.New("host label too long to encode")
		}

		delta += (m - n) * (h + 1)
		n = m

		for _, r := range in {
			if int(r) < n {
				delta++
			}

			if int(r) != n {
				continue
			}

			q := delta

			for k := base; ; k += base {
				t := min(max(k-bias, tmin), tmax)
				if q < t {
					break
				}

				out = append(out, digit(t+(q-t)%(base-t)))
				q = (q - t) / (base - t)
			}

			out = append(out, digit(q))

			// adapt
			d := delta
			if h == b {
				d /= damp
			} else {
				d /= 2
			}

			d += d / (h + 1)

			k := 0
			for ; d > ((base-tmin)*tmax)/2; k += base {
				d /= base - tmin
			}

			bias = k + (base-tmin+1)*d/(d+skew)
			delta = 0
			h++
		}

		delta++
		n++
	}

	return string(out), nil
}
