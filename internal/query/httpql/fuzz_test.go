package httpql

import (
	"errors"
	"testing"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

// FuzzParseHTTPQL checks the parser never panics, errors stay inside the
// input, and every accepted query's String() re-parses to an equal AST.
func FuzzParseHTTPQL(f *testing.F) {
	for _, q := range seedQueries(f) {
		f.Add(q)
	}
	f.Fuzz(func(t *testing.T, src string) {
		n, err := Parse(src)
		if err != nil {
			if qe, ok := errors.AsType[*core.Error](err); !ok || qe.Span.Start < 0 || qe.Span.Start > qe.Span.End || qe.Span.End > len(src) {
				t.Fatalf("span %v outside input of %d bytes", qe.Span, len(src))
			}
			return
		}
		if n == nil {
			return
		}
		if sp := n.Span(); sp.Start < 0 || sp.End > len(src) || sp.Start > sp.End {
			t.Fatalf("span %v outside input of %d bytes", sp, len(src))
		}
		canon := n.String()
		again, err := Parse(canon)
		if err != nil {
			t.Fatalf("canonical %q of %q does not re-parse: %v", canon, src, err)
		}
		if !core.Equal(n, again) {
			t.Fatalf("canonical %q of %q re-parses to %q", canon, src, again.String())
		}
	})
}
