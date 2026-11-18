package httpql

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

func TestParseCanonical(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{``, ``},
		{"  // only a comment\n /* and a block */ ", ``},
		{`req.method.eq:"GET"`, `req.method.eq:"GET"`},
		{`resp.code.eq:200`, `resp.code.eq:200`},
		{`resp.code.eq:"200"`, `resp.code.eq:200`},
		{`resp.code.gte: -1`, `resp.code.gte:-1`},
		{`req.tls.eq:true`, `req.tls.eq:true`},
		{`req.tls.ne:"false"`, `req.tls.ne:false`},
		{`req.edited.eq:true`, `req.edited.eq:true`},
		{`req.created_at.gt:2024-01-31T12:00:00Z`, `req.created_at.gt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.gt:"2024-01-31T14:00:00+02:00"`, `req.created_at.gt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.lt:"2024-01-31"`, `req.created_at.lt:"2024-01-31T00:00:00Z"`},
		{`req.created_at.lt:"2024-01-31T12:00:00"`, `req.created_at.lt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.lt:"Wed, 31 Jan 2024 13:00:00 +0100"`, `req.created_at.lt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.lt:"Wed, 31 Jan 2024 12:00:00 GMT"`, `req.created_at.lt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.lt:"2024-01-31 12:00:00.5"`, `req.created_at.lt:"2024-01-31T12:00:00.5Z"`},
		{`req.header.cont:"json"`, `req.header.cont:"json"`},
		{`req.header.name.eq:"X-Token"`, `req.header.name.eq:"X-Token"`},
		{`resp.header.value.regex:"^no-"`, `resp.header.value.regex:"^no-"`},
		{`req.header["Content-Type"].like:"%json%"`, `req.header["Content-Type"].like:"%json%"`},
		{`req.body.ncont:"a\"b\\c\n\r\t\x00\xff"`, `req.body.ncont:"a\"b\\c\n\r\t\x00\xff"`},
		{`req.note.eq:"héllo"`, `req.note.eq:"héllo"`},
		{`"admin"`, `req.raw.cont:"admin" OR resp.raw.cont:"admin"`},
		{`"a" AND "b"`, `(req.raw.cont:"a" OR resp.raw.cont:"a") AND (req.raw.cont:"b" OR resp.raw.cont:"b")`},
		{`preset:no-images`, `preset:no-images`},
		{`preset:"No Images"`, `preset:"No Images"`},
		{`preset:"x1"`, `preset:x1`},
		{`source:replay`, `source:replay`},
		{`source:"proxy"`, `source:proxy`},
		{`NOT req.ext.eq:".js"`, `NOT req.ext.eq:".js"`},
		{`not (req.ext.eq:".js" or req.ext.eq:".css")`, `NOT (req.ext.eq:".js" OR req.ext.eq:".css")`},
		{`NOT (NOT row.id.eq:1)`, `NOT (NOT row.id.eq:1)`},
		{`row.id.eq:1 and row.id.eq:2 or row.id.eq:3`, `row.id.eq:1 AND row.id.eq:2 OR row.id.eq:3`},
		{`row.id.eq:1 AND (row.id.eq:2 OR row.id.eq:3)`, `row.id.eq:1 AND (row.id.eq:2 OR row.id.eq:3)`},
		{`row.id.eq:1 OR (row.id.eq:2 OR row.id.eq:3)`, `row.id.eq:1 OR (row.id.eq:2 OR row.id.eq:3)`},
		{`((row.id.eq:1))`, `row.id.eq:1`},
		{"req.host.eq:\"a\" // trailing\nAND /* x */ req.note.cont:\"xss\"", `req.host.eq:"a" AND req.note.cont:"xss"`},
		{`req.created_at.lt:"2024-01-31T13:00+01:00"`, `req.created_at.lt:"2024-01-31T12:00:00Z"`},
		{`req.created_at.lt:"2024-01-31T14:00:00+0200"`, `req.created_at.lt:"2024-01-31T12:00:00Z"`},
		{`(resp.code.eq:200)AND(resp.len.lt:10)`, `resp.code.eq:200 AND resp.len.lt:10`},
	}
	for _, tt := range tests {
		n, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		got := ""
		if n != nil {
			got = n.String()
		}
		if got != tt.want {
			t.Errorf("Parse(%q).String() = %s, want %s", tt.in, got, tt.want)
		}
		roundTrip(t, tt.in, n)
	}
}

// roundTrip asserts that the canonical form parses back to an equal AST.
func roundTrip(t *testing.T, in string, n core.Node) {
	t.Helper()
	if n == nil {
		return
	}
	again, err := Parse(n.String())
	if err != nil {
		t.Fatalf("%q: canonical %q does not re-parse: %v", in, n.String(), err)
	}
	if !core.Equal(n, again) {
		t.Fatalf("%q: canonical %q re-parses to %q", in, n.String(), again.String())
	}
}

func TestPrecedence(t *testing.T) {
	t.Parallel()
	a, b, c := `req.method.eq:"A"`, `req.method.eq:"B"`, `req.method.eq:"C"`
	for _, tt := range []struct{ x, y string }{
		{a + " AND " + b + " OR " + c, "(" + a + " AND " + b + ") OR " + c},
		{a + " OR " + b + " AND " + c, a + " OR (" + b + " AND " + c + ")"},
		{"NOT " + a + " AND " + b, "(NOT " + a + ") AND " + b},
		{a + " OR " + b + " OR " + c, "(" + a + " OR " + b + ") OR " + c},
	} {
		x, err := Parse(tt.x)
		if err != nil {
			t.Fatal(err)
		}
		y, err := Parse(tt.y)
		if err != nil {
			t.Fatal(err)
		}
		if !core.Equal(x, y) {
			t.Errorf("%s parsed as %s, want %s", tt.x, x, y)
		}
	}
	n, _ := Parse(a + " AND " + b + " OR " + c)
	if _, ok := n.(*core.Or); !ok {
		t.Errorf("top node is %T, want *core.Or", n)
	}
}

func TestSpans(t *testing.T) {
	t.Parallel()
	src := `NOT req.port.eq:80 OR "x"`
	n, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	or := n.(*core.Or)
	for _, tt := range []struct {
		node core.Node
		want string
	}{
		{or, src},
		{or.L, `NOT req.port.eq:80`},
		{or.L.(*core.Not).X, `req.port.eq:80`},
		{or.R, `"x"`},
	} {
		sp := tt.node.Span()
		if got := src[sp.Start:sp.End]; got != tt.want {
			t.Errorf("span of %s = %q, want %q", tt.node, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		span string // text the error underlines; "" means empty span at end
		msg  string
	}{
		{`resp.code.cont:"2"`, `cont`, "`cont` is not valid for integer field resp.code; use eq, ne, gt, gte, lt, lte"},
		{`req.raw.eq:"x"`, `eq`, "`eq` is not valid for bytes field req.raw; use cont, ncont, like, nlike, regex, nregex"},
		{`req.tls.gt:true`, `gt`, "`gt` is not valid for boolean field req.tls; use eq, ne"},
		{`req.created_at.eq:"2024-01-01"`, `eq`, "`eq` is not valid for datetime field req.created_at"},
		{`req.method.gt:"A"`, `gt`, "`gt` is not valid for string field req.method"},
		{`req.method.foo:"A"`, `foo`, `unknown operator "foo"`},
		{`req.hots.eq:"a"`, `req.hots`, "unknown field req.hots; req has body, color, created_at"},
		{`ws.raw.cont:"a"`, `ws`, `unknown namespace "ws"`},
		{`req.method.eq:GET`, `GET`, `string value must be quoted`},
		{`resp.code.eq:"2x"`, `"2x"`, `is not a valid integer`},
		{`req.tls.eq:yes`, `yes`, `is not a boolean`},
		{`req.created_at.gt:"yesterday"`, `"yesterday"`, `is not a datetime`},
		{`req.created_at.gt:"0000-01-01T00:00:00+01:00"`, `"0000-01-01T00:00:00+01:00"`, `is not a datetime`},
		{`req.created_at.lt:"Wed, 31 Jan 2024 12:00:00 EST"`, `"Wed, 31 Jan 2024 12:00:00 EST"`, `is not a datetime`},
		{`finding.title.cont:"xss"`, `finding`, `unknown namespace "finding"`},
		{`req.path.regex:"(a"`, `"(a"`, `invalid regex`},
		{`req.path.eq:"abc`, `"abc`, `unterminated string`},
		{`req.path.eq:"a\qb"`, `\q`, `unknown escape`},
		{`req.path.eq:"a\x4"`, `\x4"`, `\x needs two hex digits`},
		{`req.path.eq:"a" /* open`, `/* open`, `unterminated comment`},
		{`req.path.eq:`, ``, `expected a value`},
		{`req.path.eq "a"`, `"a"`, `expected ':' after operator`},
		{`req.path`, ``, `expected '.' and an operator`},
		{`req`, ``, `expected '.' after namespace req`},
		{`req.method.eq:"A" req.method.eq:"B"`, `req`, `join conditions with AND or OR`},
		{`req.method.eq:"A")`, `)`, `without matching '('`},
		{`(req.method.eq:"A"`, ``, `expected ')' or AND/OR`},
		{`()`, `()`, `empty parentheses`},
		{`AND req.method.eq:"A"`, `AND`, `expected a condition before AND`},
		{`req.method.eq:"A" AND`, ``, `expected a condition`},
		{`req.method.eq:"A" And req.method.eq:"B"`, `And`, `join conditions with AND or OR`},
		{`NOT NOT row.id.eq:1`, `NOT`, `double NOT`},
		{`req.header[X].eq:"a"`, `X`, `quoted header name`},
		{`req.header["X".eq:"a"`, `.`, `expected ']'`},
		{`req.method.name.eq:"A"`, `name`, `unknown operator "name"`},
		{`preset:Bad`, `Bad`, `expected a preset alias`},
		{`preset:""`, `""`, `empty preset name`},
		{`source:foo`, `foo`, `unknown source "foo"; use proxy, intercept`},
		{`bogus:1`, `bogus`, `unknown clause bogus:`},
		{`req.method.eq:"A" & x`, `&`, `unexpected character '&'`},
		{strings.Repeat("(", core.MaxDepth+1) + `row.id.eq:1` + strings.Repeat(")", core.MaxDepth+1), `(`, `nested deeper than 128`},
	}
	for _, tt := range tests {
		_, err := Parse(tt.in)
		qe, ok := errors.AsType[*core.Error](err)
		if !ok {
			t.Errorf("Parse(%q) error = %v, want *core.Error", tt.in, err)
			continue
		}
		if !strings.Contains(qe.Msg, tt.msg) {
			t.Errorf("Parse(%q) msg = %q, want it to contain %q", tt.in, qe.Msg, tt.msg)
		}
		got := tt.in[qe.Span.Start:qe.Span.End]
		if got != tt.span {
			t.Errorf("Parse(%q) underlines %q (%d-%d), want %q", tt.in, got, qe.Span.Start, qe.Span.End, tt.span)
		}
	}
}

func TestQueryTooLong(t *testing.T) {
	t.Parallel()
	_, err := Parse(strings.Repeat(" ", core.MaxQueryLen+1))
	if qe, ok := errors.AsType[*core.Error](err); !ok || qe.Span.Start != core.MaxQueryLen {
		t.Fatalf("err = %v, want span starting at MaxQueryLen", err)
	}
}

func TestParseFindings(t *testing.T) {
	t.Parallel()
	const q = `finding.title.cont:"xss" AND finding.reporter.eq:"me" AND req.host.eq:"a"`
	n, err := ParseFindings(q)
	if err != nil {
		t.Fatalf("ParseFindings(%q): %v", q, err)
	}
	if got := n.String(); got != q {
		t.Errorf("String() = %s, want %s", got, q)
	}
	if _, ok := Language.Fields["finding.title"]; ok {
		t.Error("FindingsLanguage leaked finding.* into Language")
	}
}
