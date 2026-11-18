package httpql

import "testing"

// Pins the sitemap composition rule (F-SITE-1, sitemap.graphqls SitemapNode.query) against the real
// parser: node query alone for an empty bar, `(node) and (bar)` otherwise; `(node) and ()` is invalid.
func TestSitemapNodeQueryComposition(t *testing.T) {
	node := `req.tls.eq:true and req.host.eq:"example.test" and req.port.eq:443 and req.path.regex:"^/+a\\.b(/|$)"`
	for _, ok := range []string{node, "(" + node + `) and ("static")`, "(" + node + `) and (req.method.eq:"GET" or "x")`} {
		if _, err := Parse(ok); err != nil {
			t.Errorf("Parse(%q) = %v, want ok", ok, err)
		}
	}
	if _, err := Parse("(" + node + ") and ()"); err == nil {
		t.Error("Parse with empty bar group succeeded, want empty parentheses error")
	}
}
