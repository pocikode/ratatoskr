package httpql

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/store"
)

func TestMatchText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		op       core.Op
		want     string
		got      string
		expected bool
	}{
		{"eq", core.Eq, "GET", "GET", true},
		{"eq case", core.Eq, "GET", "get", false},
		{"ne", core.Ne, "GET", "POST", true},
		{"cont", core.Cont, "et", "GET", true},
		{"cont folds ASCII", core.Cont, "Et", "gEt", true},
		{"cont does not fold non-ASCII", core.Cont, "ä", "Ä", false},
		{"cont empty", core.Cont, "", "GET", true},
		{"ncont", core.Ncont, "et", "GET", false},
		{"like folds ASCII", core.Like, "g%", "GET", true},
		{"like no non-ASCII fold", core.Like, "Ä", "ä", false},
		{"nlike", core.Nlike, "%x%", "GET", true},
		{"regex", core.Regex, "^G.T$", "GET", true},
		{"nregex", core.Nregex, "^G.T$", "get", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchText(tt.op, tt.want, tt.got); got != tt.expected {
				t.Errorf("matchText(%s, %q, %q) = %v, want %v", tt.op, tt.want, tt.got, got, tt.expected)
			}
			// eq/ne are text-only; bytes accept cont/like/regex (docs/04 §3).
			if tt.op != core.Eq && tt.op != core.Ne {
				if got := matchBytes(tt.op, tt.want, []byte(tt.got)); got != tt.expected {
					t.Errorf("matchBytes(%s, %q, %q) = %v, want %v", tt.op, tt.want, tt.got, got, tt.expected)
				}
			}
		})
	}
}

// TestLikeMatch pins the SQLite LIKE semantics of docs/04 §3; every case was
// checked against SQLite itself (see the conformance back-ends test).
func TestLikeMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern, s string
		expected   bool
	}{
		{"", "", true},
		{"", "a", false},
		{"%", "", true},
		{"%", "abc", true},
		{"_", "", false},
		{"_", "a", true},
		{"_", "ä", true}, // one character, not one byte
		{"__", "ä", false},
		{"a_c", "abc", true},
		{"a_c", "ac", false},
		{"A%", "abc", true},
		{"a_B", "aäb", true},
		{"Ä", "ä", false}, // SQLite folds ASCII only
		{"a%b", "axyzb", true},
		{"a%b", "ab", true},
		{"%b", "ab", true},
		{"a\\%b", "a%b", true},
		{"a\\%b", "axb", false},
		{"a\\_c", "a_c", true},
		{"a\\_c", "abc", false},
		{"a\\\\b", `a\b`, true},
		{"a\\", "a", false}, // dangling escape matches nothing
		{"a", "a%", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.s, func(t *testing.T) {
			t.Parallel()
			if got := likeMatch(tt.pattern, []byte(tt.s)); got != tt.expected {
				t.Errorf("likeMatch(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.expected)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		hay, needle string
		expected    bool
	}{
		{"Hello World", "world", true},
		{"Hello", "", true},
		{"", "a", false},
		{"ÄBC", "äbc", false}, // ASCII fold only, like SQLite's lower()
		{"session=abc123", "ABC123", true},
	}
	for _, tt := range tests {
		t.Run(tt.hay+"/"+tt.needle, func(t *testing.T) {
			t.Parallel()
			if got := containsFold(tt.hay, tt.needle); got != tt.expected {
				t.Errorf("containsFold(%q, %q) = %v, want %v", tt.hay, tt.needle, got, tt.expected)
			}
			if got := containsFold([]byte(tt.hay), []byte(tt.needle)); got != tt.expected {
				t.Errorf("containsFold([]byte) = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEvalHeaders(t *testing.T) {
	t.Parallel()
	headers := []query.Header{
		{Name: "Cookie", Value: "session=abc123"},
		{Name: "Accept", Value: "*/*"},
		{Name: "X-Debug", Value: "on"},
		{Name: "X-Debug", Value: "verbose"},
	}
	tests := []struct {
		query    string
		expected bool
	}{
		{`req.header.cont:"abc123"`, true},
		{`req.header.cont:"COOKIE"`, true}, // names match too
		{`req.header.eq:"Cookie"`, true},
		{`req.header.name.eq:"Cookie"`, true},
		{`req.header.name.eq:"cookie"`, false}, // names are case-sensitive
		{`req.header.value.eq:"on"`, true},
		{`req.header["cookie"].cont:"SESSION"`, true}, // header["X"] folds the name
		{`req.header["x-debug"].eq:"on"`, true},
		{`req.header["x-debug"].eq:"verbose"`, true},
		{`req.header["x-debug"].ne:"on"`, false}, // some value equals it
		{`req.header["x-debug"].ncont:"verb"`, false},
		{`req.header["missing"].ne:"on"`, true}, // no such header
		{`req.header["missing"].ncont:"on"`, true},
		{`req.header.ncont:"nomatch"`, true},
		{`req.header.ncont:"debug"`, false}, // the name matches
		{`req.header.name.regex:"^X-"`, true},
		{`req.header.name.nregex:"^X-"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()
			n, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			m := sampleMessage{&sample{reqRaw: []byte("GET / HTTP/1.1\r\n\r\n"), reqHeaders: headers}}
			got, err := Eval(n, m)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got != tt.expected {
				t.Errorf("Eval(%s) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

// TestMissingResponse pins how an unanswered request evaluates: the response
// fields read as an empty response on both back-ends.
func TestMissingResponse(t *testing.T) {
	t.Parallel()
	reqRaw := []byte("GET /unanswered HTTP/1.1\r\nHost: example.com\r\n\r\n")
	s := &sample{
		id: 7, source: "proxy", host: "example.com", port: 80,
		createdAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		method:    "GET", target: "/unanswered", p: "/unanswered",
		reqRaw: reqRaw, reqBodyAt: len(reqRaw),
		reqHeaders: []query.Header{{Name: "Host", Value: "example.com"}},
	}

	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenProject(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insertSample(t, db, s)

	tests := []struct {
		query    string
		expected bool
	}{
		{`resp.code.eq:0`, true},
		{`resp.code.ne:0`, false},
		{`resp.code.gte:200`, false},
		{`resp.len.eq:0`, true},
		{`resp.roundtrip.lt:5`, true},
		{`resp.raw.cont:"HTTP"`, false},
		{`resp.raw.ncont:"HTTP"`, true},
		{`resp.body.cont:"x"`, false},
		{`resp.body.ncont:"x"`, true},
		{`resp.header.cont:"x"`, false},
		{`resp.header["content-type"].eq:"x"`, false},
		{`resp.header["content-type"].ne:"x"`, true},
		{`NOT resp.code.eq:0`, false},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			n, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, err := Eval(n, sampleMessage{s})
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got != tt.expected {
				t.Errorf("Eval(%s) = %v, want %v", tt.query, got, tt.expected)
			}
			compiled, err := Compile(n)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			gotSQL, err := sqlMatches(t.Context(), db, compiled, s.id)
			if err != nil {
				t.Fatalf("sql: %v", err)
			}
			if gotSQL != tt.expected {
				t.Errorf("sql %q args %v = %v, want %v", compiled.Where, compiled.Args, gotSQL, tt.expected)
			}
		})
	}
}

func TestEvalErrors(t *testing.T) {
	t.Parallel()
	m := sampleMessage{&sample{}}
	for _, n := range []core.Node{
		&core.Preset{Name: "mine", Loc: core.Span{End: 10}},
		&core.Clause{Namespace: "finding", Field: "title", Loc: core.Span{End: 7}},
	} {
		if _, err := Eval(n, m); err == nil {
			t.Errorf("Eval(%T) returned no error", n)
		}
	}
	ok, err := Eval(nil, m)
	if err != nil || !ok {
		t.Errorf("Eval(nil) = %v, %v; want true", ok, err)
	}
}

func TestRegexpCacheBounded(t *testing.T) {
	t.Parallel()
	for i := range regexpCacheSize * 2 {
		regexpMatch(fmt.Sprintf("pattern-%d", i), []byte("hay"))
	}
	if got := regexps.Len(); got > regexpCacheSize {
		t.Fatalf("cache holds %d patterns, want at most %d", got, regexpCacheSize)
	}
	if regexps.Contains("pattern-0") {
		t.Error("oldest pattern was not evicted")
	}
	if !regexps.Contains(fmt.Sprintf("pattern-%d", regexpCacheSize*2-1)) {
		t.Error("newest pattern is not cached")
	}
}
