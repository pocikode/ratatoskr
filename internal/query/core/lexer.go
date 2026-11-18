// Package core holds the lexer, parser and AST shared by the HTTPQL and
// StreamQL query languages (docs/04-query-language.md §1). A [Language]
// supplies the field table; everything else is common.
package core

import (
	"fmt"
	"strings"
)

type tokenKind uint8

const (
	tEOF tokenKind = iota
	tIdent
	tString // quoted literal; text is the decoded value
	tBare   // unquoted value after ':'
	tDot
	tColon
	tLParen
	tRParen
	tLBrack
	tRBrack
)

func (k tokenKind) String() string {
	return [...]string{"end of query", "identifier", "string", "value", "'.'", "':'", "'('", "')'", "'['", "']'"}[k]
}

type token struct {
	kind tokenKind
	text string
	span Span
}

// describe names a token for error messages.
func (t token) describe() string {
	switch t.kind {
	case tIdent, tBare:
		return fmt.Sprintf("%q", t.text)
	case tString:
		return "string " + quote(t.text)
	}
	return t.kind.String()
}

type lexer struct {
	src string
	pos int
}

func isIdentByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// skip consumes whitespace and comments.
func (l *lexer) skip() error {
	for l.pos < len(l.src) {
		switch c := l.src[l.pos]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			l.pos++
		case strings.HasPrefix(l.src[l.pos:], "//"):
			end := strings.IndexByte(l.src[l.pos:], '\n')
			if end < 0 {
				l.pos = len(l.src)
			} else {
				l.pos += end + 1
			}
		case strings.HasPrefix(l.src[l.pos:], "/*"):
			end := strings.Index(l.src[l.pos+2:], "*/")
			if end < 0 {
				return errorf(Span{l.pos, len(l.src)}, "unterminated comment")
			}
			l.pos += 2 + end + 2
		default:
			return nil
		}
	}
	return nil
}

// next returns the next token in normal mode.
func (l *lexer) next() (token, error) {
	if err := l.skip(); err != nil {
		return token{}, err
	}
	start := l.pos
	if start == len(l.src) {
		return token{kind: tEOF, span: Span{start, start}}, nil
	}
	c := l.src[start]
	if k, ok := punct[c]; ok {
		l.pos++
		return token{kind: k, text: l.src[start:l.pos], span: Span{start, l.pos}}, nil
	}
	if c == '"' {
		return l.string()
	}
	if !isIdentByte(c) {
		return token{}, errorf(Span{start, start + 1}, "unexpected character %q", c)
	}
	for l.pos < len(l.src) && isIdentByte(l.src[l.pos]) {
		l.pos++
	}
	return token{kind: tIdent, text: l.src[start:l.pos], span: Span{start, l.pos}}, nil
}

var punct = map[byte]tokenKind{'.': tDot, ':': tColon, '(': tLParen, ')': tRParen, '[': tLBrack, ']': tRBrack}

// value returns the token following ':' in a clause: a quoted string, or a
// bare run up to whitespace, a parenthesis, a quote or a comment start.
func (l *lexer) value() (token, error) {
	if err := l.skip(); err != nil {
		return token{}, err
	}
	start := l.pos
	if start < len(l.src) && l.src[start] == '"' {
		return l.string()
	}
	for l.pos < len(l.src) && !strings.ContainsRune(" \t\r\n()\"/", rune(l.src[l.pos])) {
		l.pos++
	}
	if l.pos == start {
		return l.next() // let the parser report what is there instead
	}
	return token{kind: tBare, text: l.src[start:l.pos], span: Span{start, l.pos}}, nil
}

// string decodes a quoted literal starting at l.pos.
func (l *lexer) string() (token, error) {
	start := l.pos
	l.pos++ // opening quote
	var b strings.Builder
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch c {
		case '"':
			l.pos++
			return token{kind: tString, text: b.String(), span: Span{start, l.pos}}, nil
		case '\\':
			if l.pos+1 >= len(l.src) {
				return token{}, errorf(Span{start, len(l.src)}, "unterminated string")
			}
			esc := l.src[l.pos+1]
			switch esc {
			case '"', '\\':
				b.WriteByte(esc)
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'x':
				hi, ok1 := unhex(l.src, l.pos+2)
				lo, ok2 := unhex(l.src, l.pos+3)
				if !ok1 || !ok2 {
					return token{}, errorf(Span{l.pos, min(l.pos+4, len(l.src))}, `\x needs two hex digits`)
				}
				b.WriteByte(hi<<4 | lo)
				l.pos += 2
			default:
				return token{}, errorf(Span{l.pos, l.pos + 2}, `unknown escape %q; use \", \\, \n, \r, \t or \xHH`, l.src[l.pos:l.pos+2])
			}
			l.pos += 2
		default:
			b.WriteByte(c)
			l.pos++
		}
	}
	return token{}, errorf(Span{start, len(l.src)}, "unterminated string")
}

// unhex decodes the hex digit at s[i], if there is one.
func unhex(s string, i int) (byte, bool) {
	if i >= len(s) {
		return 0, false
	}
	switch c := s[i]; {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
