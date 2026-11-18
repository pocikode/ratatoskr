package core

import (
	"strings"
	"unicode/utf8"
)

// Span is a byte range in the query text, end exclusive.
type Span struct{ Start, End int }

// Node is an AST node. String renders canonical query text that parses back
// to an equal node (see [Equal]).
type Node interface {
	Span() Span
	String() string
}

// And is "L AND R".
type And struct {
	L, R Node
	Loc  Span
}

// Or is "L OR R".
type Or struct {
	L, R Node
	Loc  Span
}

// Not is "NOT X".
type Not struct {
	X   Node
	Loc Span
}

// Sub selects part of a header field.
type Sub uint8

// Header subfields.
const (
	SubNone  Sub = iota // any header line
	SubName             // header.name
	SubValue            // header.value
	SubKey              // header["Key"]: values of the named header
)

// Clause is "namespace.field[subfield].op:value".
type Clause struct {
	Namespace, Field string
	Sub              Sub
	Key              string // header name when Sub == SubKey
	Op               Op
	Value            Value
	Loc              Span
}

// Preset is "preset:name"; Name is a preset name or alias, resolved later.
type Preset struct {
	Name string
	Loc  Span
}

// Source is "source:name".
type Source struct {
	Name string
	Loc  Span
}

// Span implements [Node].
func (n *And) Span() Span { return n.Loc }

// Span implements [Node].
func (n *Or) Span() Span { return n.Loc }

// Span implements [Node].
func (n *Not) Span() Span { return n.Loc }

// Span implements [Node].
func (n *Clause) Span() Span { return n.Loc }

// Span implements [Node].
func (n *Preset) Span() Span { return n.Loc }

// Span implements [Node].
func (n *Source) Span() Span { return n.Loc }

func (n *And) String() string { return binary(n.L, n.R, " AND ", 2) }

func (n *Or) String() string { return binary(n.L, n.R, " OR ", 1) }

func (n *Not) String() string { return "NOT " + group(n.X, !isLeaf(n.X)) }

func (n *Clause) String() string {
	var b strings.Builder
	b.WriteString(n.Namespace + "." + n.Field)
	switch n.Sub {
	case SubName:
		b.WriteString(".name")
	case SubValue:
		b.WriteString(".value")
	case SubKey:
		b.WriteString("[" + quote(n.Key) + "]")
	}
	b.WriteString("." + string(n.Op) + ":" + n.Value.String())
	return b.String()
}

func (n *Preset) String() string {
	if isAlias(n.Name) {
		return "preset:" + n.Name
	}
	return "preset:" + quote(n.Name)
}

func (n *Source) String() string { return "source:" + n.Name }

func isLeaf(n Node) bool {
	switch n.(type) {
	case *Clause, *Preset, *Source:
		return true
	}
	return false
}

// prec is the binding strength of n: OR < AND < NOT and leaves.
func prec(n Node) int {
	switch n.(type) {
	case *Or:
		return 1
	case *And:
		return 2
	}
	return 3
}

// binary prints a left-associative operator of precedence p: the left child
// needs parentheses only when it binds looser, the right one also when equal.
func binary(l, r Node, op string, p int) string {
	return group(l, prec(l) < p) + op + group(r, prec(r) <= p)
}

func group(n Node, paren bool) string {
	if paren {
		return "(" + n.String() + ")"
	}
	return n.String()
}

// isAlias reports whether s matches /[a-z0-9][a-z0-9_-]*/.
func isAlias(s string) bool {
	if s == "" || s[0] == '_' || s[0] == '-' {
		return false
	}
	for i := range len(s) {
		if c := s[i]; !isIdentByte(c) || c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

// quote renders s as a string literal, escaping control bytes and invalid
// UTF-8 as \xHH so the literal decodes back to the same bytes.
func quote(s string) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == utf8.RuneError && size == 1, r < 0x20, r == 0x7f:
			b.WriteString(`\x`)
			b.WriteByte(hex[s[i]>>4])
			b.WriteByte(hex[s[i]&0xf])
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}

// Equal reports whether a and b are the same query, ignoring spans.
func Equal(a, b Node) bool {
	switch a := a.(type) {
	case nil:
		return b == nil
	case *And:
		b, ok := b.(*And)
		return ok && Equal(a.L, b.L) && Equal(a.R, b.R)
	case *Or:
		b, ok := b.(*Or)
		return ok && Equal(a.L, b.L) && Equal(a.R, b.R)
	case *Not:
		b, ok := b.(*Not)
		return ok && Equal(a.X, b.X)
	case *Clause:
		b, ok := b.(*Clause)
		return ok && a.Namespace == b.Namespace && a.Field == b.Field && a.Sub == b.Sub &&
			a.Key == b.Key && a.Op == b.Op && a.Value.equal(b.Value)
	case *Preset:
		b, ok := b.(*Preset)
		return ok && a.Name == b.Name
	case *Source:
		b, ok := b.(*Source)
		return ok && a.Name == b.Name
	}
	return false
}
