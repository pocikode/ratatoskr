package core

import (
	"testing"
	"time"
)

func TestQuoteRoundTrip(t *testing.T) {
	t.Parallel()
	var all []byte
	for i := range 256 {
		all = append(all, byte(i))
	}
	for _, s := range []string{"", `a"b\c`, "héllo wörld ✓", string(all), "\xe2\x82"} {
		l := lexer{src: quote(s)}
		tok, err := l.next()
		if err != nil || tok.kind != tString || tok.text != s || l.pos != len(l.src) {
			t.Errorf("quote(%q) = %s decodes to %q, %v", s, l.src, tok.text, err)
		}
	}
}

func TestEqualIgnoresSpans(t *testing.T) {
	t.Parallel()
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	a := &And{L: &Clause{Namespace: "a", Field: "b", Op: Gt, Value: Value{Type: Datetime, Time: ts}, Loc: Span{0, 5}},
		R: &Not{X: &Preset{Name: "p"}}, Loc: Span{0, 9}}
	b := &And{L: &Clause{Namespace: "a", Field: "b", Op: Gt, Value: Value{Type: Datetime, Time: ts.In(time.FixedZone("x", 3600))}},
		R: &Not{X: &Preset{Name: "p"}}}
	if !Equal(a, b) {
		t.Error("Equal = false for nodes differing only in spans and zone")
	}
	b.R.(*Not).X = &Source{Name: "p"}
	if Equal(a, b) {
		t.Error("Equal = true for preset vs source")
	}
	if !Equal(nil, nil) || Equal(a, nil) {
		t.Error("nil handling")
	}
}
