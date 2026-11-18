package core

import (
	"regexp"
	"slices"
	"strings"
)

// Limits that keep a hostile query from exhausting memory or the stack.
const (
	MaxQueryLen = 64 << 10
	MaxDepth    = 128
)

// Field describes one queryable field.
type Field struct {
	Type   Type
	Header bool // accepts .name, .value and ["Key"] subfields
}

// Language is the field table and literal shorthand of one query language.
type Language struct {
	// Fields maps "namespace.field" to its definition.
	Fields map[string]Field
	// Sources lists the values accepted by source:; nil disables source:.
	Sources []string
	// Shorthand expands a bare string literal at sp into a clause tree.
	Shorthand func(lit string, sp Span) Node
}

// Parse parses src. An empty query (only whitespace and comments) yields a
// nil Node. Errors are *[Error].
func (lang *Language) Parse(src string) (Node, error) {
	if len(src) > MaxQueryLen {
		return nil, errorf(Span{MaxQueryLen, len(src)}, "query longer than %d bytes", MaxQueryLen)
	}
	p := &parser{lang: lang, lex: lexer{src: src}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind == tEOF {
		return nil, nil
	}
	n, err := p.expr()
	if err != nil {
		return nil, err
	}
	if p.tok.kind != tEOF {
		msg := "unexpected %s; join conditions with AND or OR"
		if p.tok.kind == tRParen {
			msg = "unexpected %s without matching '('"
		}
		return nil, errorf(p.tok.span, msg, p.tok.describe())
	}
	return n, nil
}

type parser struct {
	lang  *Language
	lex   lexer
	tok   token
	depth int
}

func (p *parser) advance() (err error) {
	p.tok, err = p.lex.next()
	return err
}

// keyword reports whether the current token is the keyword kw in upper or
// lower case (docs/04 §1: mixed case is not a keyword).
func (p *parser) keyword(kw string) bool {
	return p.tok.kind == tIdent && (p.tok.text == kw || p.tok.text == strings.ToLower(kw))
}

func (p *parser) expect(k tokenKind, what string) (token, error) {
	tok := p.tok
	if tok.kind != k {
		return tok, errorf(tok.span, "expected %s, found %s", what, tok.describe())
	}
	return tok, p.advance()
}

func (p *parser) expr() (Node, error) {
	l, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.keyword("OR") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.and()
		if err != nil {
			return nil, err
		}
		l = &Or{L: l, R: r, Loc: Span{l.Span().Start, r.Span().End}}
	}
	return l, nil
}

func (p *parser) and() (Node, error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.keyword("AND") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.unary()
		if err != nil {
			return nil, err
		}
		l = &And{L: l, R: r, Loc: Span{l.Span().Start, r.Span().End}}
	}
	return l, nil
}

func (p *parser) unary() (Node, error) {
	if !p.keyword("NOT") {
		return p.primary()
	}
	start := p.tok.span.Start
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.keyword("NOT") {
		return nil, errorf(p.tok.span, "double NOT; use NOT (NOT …) if you mean it")
	}
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	return &Not{X: x, Loc: Span{start, x.Span().End}}, nil
}

func (p *parser) primary() (Node, error) {
	switch tok := p.tok; tok.kind {
	case tLParen:
		if p.depth++; p.depth > MaxDepth {
			return nil, errorf(tok.span, "parentheses nested deeper than %d", MaxDepth)
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind == tRParen {
			return nil, errorf(Span{tok.span.Start, p.tok.span.End}, "empty parentheses")
		}
		n, err := p.expr()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tRParen, "')' or AND/OR"); err != nil {
			return nil, err
		}
		p.depth--
		return n, nil
	case tString:
		if err := p.advance(); err != nil {
			return nil, err
		}
		return p.lang.Shorthand(tok.text, tok.span), nil
	case tIdent:
		if p.keyword("AND") || p.keyword("OR") || p.keyword("NOT") {
			return nil, errorf(tok.span, "expected a condition before %s", tok.text)
		}
		return p.clause()
	}
	return nil, errorf(p.tok.span, "expected a condition, found %s", p.tok.describe())
}

func (p *parser) clause() (Node, error) {
	first := p.tok
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind == tColon {
		switch first.text {
		case "preset":
			return p.preset(first)
		case "source":
			if p.lang.Sources != nil {
				return p.source(first)
			}
		}
		return nil, errorf(first.span, "unknown clause %s:", first.text)
	}
	if _, err := p.expect(tDot, "'.' after namespace "+first.text); err != nil {
		return nil, err
	}
	name, err := p.expect(tIdent, "a field name")
	if err != nil {
		return nil, err
	}
	key := first.text + "." + name.text
	field, ok := p.lang.Fields[key]
	if !ok {
		return nil, p.unknownField(first, name)
	}
	c := &Clause{Namespace: first.text, Field: name.text}
	typ := field.Type
	if field.Header {
		if c.Sub, c.Key, err = p.subfield(); err != nil {
			return nil, err
		}
		typ = String
	}
	if _, err := p.expect(tDot, "'.' and an operator"); err != nil {
		return nil, err
	}
	opTok, err := p.expect(tIdent, "an operator")
	if err != nil {
		return nil, err
	}
	op := Op(opTok.text)
	if !slices.Contains(allOps, op) {
		return nil, errorf(opTok.span, "unknown operator %q; %s field %s accepts %s", opTok.text, typ, key, joinOps(typ.Ops()))
	}
	if !typ.allows(op) {
		return nil, errorf(opTok.span, "`%s` is not valid for %s field %s; use %s", op, typ, key, joinOps(typ.Ops()))
	}
	c.Op = op
	if p.tok.kind != tColon {
		return nil, errorf(p.tok.span, "expected ':' after operator, found %s", p.tok.describe())
	}
	valTok, err := p.lex.value()
	if err != nil {
		return nil, err
	}
	if c.Value, err = coerce(typ, valTok); err != nil {
		return nil, err
	}
	if op == Regex || op == Nregex {
		if _, err := regexp.Compile(c.Value.Str); err != nil {
			return nil, errorf(valTok.span, "invalid regex: %v", err)
		}
	}
	c.Loc = Span{first.span.Start, valTok.span.End}
	return c, p.advance()
}

// subfield parses the optional .name, .value or ["Key"] after a header field.
func (p *parser) subfield() (Sub, string, error) {
	if p.tok.kind == tLBrack {
		if err := p.advance(); err != nil {
			return 0, "", err
		}
		key, err := p.expect(tString, `a quoted header name like ["X-Name"]`)
		if err != nil {
			return 0, "", err
		}
		if _, err := p.expect(tRBrack, "']'"); err != nil {
			return 0, "", err
		}
		return SubKey, key.text, nil
	}
	if p.tok.kind != tDot {
		return SubNone, "", nil
	}
	// Peek past the dot without consuming it when it starts the operator.
	save, saveTok := p.lex, p.tok
	if err := p.advance(); err != nil {
		return 0, "", err
	}
	if p.tok.kind == tIdent && (p.tok.text == "name" || p.tok.text == "value") {
		sub := map[string]Sub{"name": SubName, "value": SubValue}[p.tok.text]
		return sub, "", p.advance()
	}
	p.lex, p.tok = save, saveTok
	return SubNone, "", nil
}

func (p *parser) unknownField(ns, name token) error {
	sp := Span{ns.span.Start, name.span.End}
	var known []string
	for k := range p.lang.Fields {
		if after, ok := strings.CutPrefix(k, ns.text+"."); ok {
			known = append(known, after)
		}
	}
	if known == nil {
		return errorf(ns.span, "unknown namespace %q", ns.text)
	}
	slices.Sort(known)
	return errorf(sp, "unknown field %s.%s; %s has %s", ns.text, name.text, ns.text, strings.Join(known, ", "))
}

func (p *parser) preset(kw token) (Node, error) {
	tok, err := p.lex.value()
	if err != nil {
		return nil, err
	}
	switch {
	case tok.kind == tString:
		if tok.text == "" {
			return nil, errorf(tok.span, "empty preset name")
		}
	case tok.kind == tBare && isAlias(tok.text):
	default:
		return nil, errorf(tok.span, "expected a preset alias (lowercase letters, digits, _ and -) or a quoted name, found %s", tok.describe())
	}
	return &Preset{Name: tok.text, Loc: Span{kw.span.Start, tok.span.End}}, p.advance()
}

func (p *parser) source(kw token) (Node, error) {
	tok, err := p.lex.value()
	if err != nil {
		return nil, err
	}
	if (tok.kind != tString && tok.kind != tBare) || !slices.Contains(p.lang.Sources, tok.text) {
		return nil, errorf(tok.span, "unknown source %s; use %s", tok.describe(), strings.Join(p.lang.Sources, ", "))
	}
	return &Source{Name: tok.text, Loc: Span{kw.span.Start, tok.span.End}}, p.advance()
}
