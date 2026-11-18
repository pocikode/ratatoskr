package httpql

import "testing"

// compose mirrors the server-side rule for SitemapNode.query (F-SITE-1, sitemap.graphqls):
// node alone when the bar parses to empty (whitespace/comments), else `(node) and (bar\n)`.
func compose(t *testing.T, node, bar string) string {
	t.Helper()
	n, err := Parse(bar)
	if err != nil {
		t.Fatalf("Parse(bar %q) = %v", bar, err)
	}
	if n == nil {
		return node
	}
	return "(" + node + ") and (" + bar + "\n)"
}

func TestSitemapNodeQueryComposition(t *testing.T) {
	node := `req.tls.eq:true and req.host.eq:"example.test" and req.port.eq:443 and req.path.regex:"^/+a\\.b(/|$)"`
	for _, bar := range []string{
		"", "  \n", "/* note */", "// note", `"static"`, `req.method.eq:"GET" or "x"`,
		`req.method.eq:"GET" // note`, "/* a */ \"x\" /* b */",
	} {
		q := compose(t, node, bar)
		if _, err := Parse(q); err != nil {
			t.Errorf("bar %q: Parse(%q) = %v, want ok", bar, q, err)
		}
	}
	// Why the rule exists: naive grouping breaks on these bars.
	for _, bad := range []string{"(" + node + ") and ()", "(" + node + ") and (/* note */)", "(" + node + `) and ("x" // note)`} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", bad)
		}
	}
}
