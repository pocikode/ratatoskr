package matchreplace

import (
	"bytes"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/apperr"
)

func regexRule(t testing.TB, pattern, term string) *compiled {
	t.Helper()
	c, err := compile(Rule{Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: Regex, Value: pattern}, Term: term}, nil)
	if err != nil {
		t.Fatalf("pattern %q: %v", pattern, err)
	}
	return c
}

// checkLikeReplaceAll: the incremental replacer gives exactly what
// regexp.ReplaceAll gives (same matches, anchors, boundaries, empty matches).
func checkLikeReplaceAll(t *testing.T, pattern, term string, src []byte) {
	t.Helper()
	c := regexRule(t, pattern, term)
	got, err := c.replace(src, 1<<30)
	if err != nil {
		t.Fatalf("%q on %q: %v", pattern, src, err)
	}
	if want := regexp.MustCompile(pattern).ReplaceAll(src, []byte(term)); !bytes.Equal(got, want) {
		t.Errorf("%q -> %q on %q\n got %q\nwant %q", pattern, term, src, got, want)
	}
}

func TestRegexReplaceMatchesStdlib(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"", "a", "foo bar foo", "foo\nbar\nfoo", "aaa", "ab ab\tab", "€a€ ééx", "a\xffb\xe2\x82c\xe2", "x.y=z&x=1",
	}
	patterns := []struct{ re, term string }{
		{`^foo`, "X"}, {`(?m)^foo`, "X"}, {`(?m)^`, ">"}, {`(?m)$`, "<"}, {`$`, "!"}, {`\A`, "S"}, {`\z`, "E"},
		{`\b`, "|"}, {`\B`, "."}, {`\bfoo\b`, "[$0]"}, {`\Bb`, "B"}, {`a*`, "-"}, {`x*`, "-"}, {`(?:)`, "_"},
		{`(a)(b)?`, "$2$1$$"}, {`(?P<w>\w+)`, "${w}${w}"}, {`.`, "$0$0"}, {`(?s).`, "<$0>"}, {`[^a]`, ""},
		{`(?i)A`, "z"}, {`(a)`, "$01$1x$x${1}x${1$9999999999$"}, {`(?P<01>a)`, "<$01>"}, {`(a)`, "${"}, {`a|^b|\bc`, "#"}, {`\x{FFFD}`, "?"}, {`(?U)a+`, "1"},
		{`(?P<p>a)|(?P<p>b)`, "<${p}$p>"}, {`(?P<p>x)?(?P<p>a)`, "<${p}>"},
	}
	for _, p := range patterns {
		for _, in := range inputs {
			checkLikeReplaceAll(t, p.re, p.term, []byte(in))
		}
	}
}

func FuzzRegexReplace(f *testing.F) {
	f.Add(`\b`, "|", []byte("ab cd"))
	f.Add(`(?m)^(\w)`, "[$1]", []byte("x\ny\n"))
	f.Add(`a*`, "-", []byte("baaac"))
	f.Fuzz(func(t *testing.T, pattern, term string, src []byte) {
		if len(pattern) > 64 || len(src) > 1<<10 || len(term) > 64 {
			return
		}
		if _, err := compile(Rule{Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: Regex, Value: pattern}}, nil); err != nil {
			return
		}
		checkLikeReplaceAll(t, pattern, term, src)
	})
}

// TestRegexDenseMatches: a body where every byte matches stays within the
// output cap while replacing, and fails as soon as the cap is crossed.
func TestRegexDenseMatches(t *testing.T) {
	t.Parallel()
	body := bytes.Repeat([]byte("a"), 256<<10)
	raw := append([]byte("POST / HTTP/1.1\r\nHost: a\r\nContent-Length: 262144\r\n\r\n"), body...)
	rule := Rule{Name: "r", Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: Regex, Value: `(a)`}}

	rule.Term = "${1}b"
	out, _, changed, err := newEngine().Test(rule, nil, KindRequest, raw)
	if err != nil || !changed || !bytes.HasSuffix(out, bytes.Repeat([]byte("ab"), 256<<10)) {
		t.Fatalf("dense update: changed=%v err=%v len=%d", changed, err, len(out))
	}

	rule.Term = strings.Repeat("b", 1<<10) // 256 MiB if fully built
	out, _, _, err = newEngine().Test(rule, nil, KindRequest, raw)
	if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Code != apperr.BadInput || out != nil {
		t.Fatalf("dense over cap: out=%d err=%v", len(out), err)
	}
}

// TestRegexBudget: the cap counts only output already decided, so growth then
// shrinkage that ends under the cap is fine, and one oversized expansion fails
// before it is built.
func TestRegexBudget(t *testing.T) { // not parallel: measures allocated bytes
	c := regexRule(t, `(a)|bbbb`, "${1}${1}")
	if got, err := c.replace([]byte("abbbb"), 2); err != nil || string(got) != "aa" {
		t.Fatalf("grow then shrink: %q %v", got, err)
	}
	if _, err := c.replace([]byte("abbbb"), 1); !errors.Is(err, errTooLarge) {
		t.Fatalf("over cap: %v", err)
	}
	big := regexRule(t, `(.*)`, strings.Repeat("$1", 1<<10))
	src := bytes.Repeat([]byte("x"), 1<<20) // a 1 GiB expansion if built
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := big.replace(src, 16<<20)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("single expansion: %v", err)
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("single expansion allocated %d bytes before failing", n)
	}
}

// TestParseTemplateLinear: a 1 MiB term of escaped dollars compiles in linear
// time and space (literal runs are not recopied per escape).
func TestParseTemplateLinear(t *testing.T) { // not parallel: measures allocated bytes
	term := strings.Repeat("$$", 1<<19)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	c := regexRule(t, `a`, term)
	runtime.ReadMemStats(&after)
	if len(c.tmpl.pieces) != 1 || c.tmpl.pieces[0].lit != strings.Repeat("$", 1<<19) {
		t.Fatalf("pieces: %d", len(c.tmpl.pieces))
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 16<<20 {
		t.Errorf("compiling allocated %d bytes", n)
	}
}

// TestParseTemplateSharedGroups: repeated references to a name shared by many
// groups compile without a per-reference index list or group scan.
func TestParseTemplateSharedGroups(t *testing.T) { // not parallel: measures allocated bytes
	re := strings.TrimSuffix(strings.Repeat("(?P<p>a)|", 1000), "|")
	term := strings.Repeat("${p}", 1<<18) // 1 MiB
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	c := regexRule(t, re, term)
	runtime.ReadMemStats(&after)
	if len(c.tmpl.pieces) != 1<<18 || len(c.tmpl.refs) != 1 || len(c.tmpl.refs[0]) != 1000 {
		t.Fatalf("pieces: %d refs: %d", len(c.tmpl.pieces), len(c.tmpl.refs))
	}
	// ~45 MiB (pieces + append growth); a scan per reference allocated ~2 GiB.
	if n := after.TotalAlloc - before.TotalAlloc; n > 96<<20 {
		t.Errorf("compiling allocated %d bytes", n)
	}
}

// TestRegexSharedGroupsPerMatch: many references to a name whose 1000 groups
// are all absent (or only the last takes part) resolve the name once per match,
// not once per reference: 100 matches x 2^18 refs x 1000 groups would take minutes.
func TestRegexSharedGroupsPerMatch(t *testing.T) {
	t.Parallel()
	groups := strings.Repeat("(?P<p>a)|", 999)
	term := strings.Repeat("${p}", 1<<18)
	for _, tc := range []struct{ re, src, want string }{
		{groups + "(?P<p>a)|b", strings.Repeat("b", 100), ""},                         // absent
		{groups + "(?P<p>b)", strings.Repeat("b", 100), strings.Repeat("b", 100<<18)}, // late
	} {
		c := regexRule(t, tc.re, term)
		start := time.Now()
		got, err := c.replace([]byte(tc.src), 64<<20)
		if err != nil || string(got) != tc.want {
			t.Fatalf("%.20q: len %d %v", tc.src, len(got), err)
		}
		if d := time.Since(start); !raceEnabled && d > 10*time.Second {
			// The detector makes the engine ~10x slower (22-26 s on a CI runner),
			// so the wall-clock bound only guards the plain build.
			t.Errorf("replace took %v", d)
		}
	}
}
