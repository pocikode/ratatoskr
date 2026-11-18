package httpql

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

// conformanceCase is one row of testdata/conformance (format in the archive
// comments). T-015 evaluates them against both back-ends.
type conformanceCase struct {
	name     string // file:line
	query    string
	want     bool
	request  []byte
	response []byte
	meta     map[string]string
}

var metaKeys = []string{"host", "port", "tls", "created_at", "roundtrip", "id", "source", "color", "note", "edited"}

func loadConformance(tb testing.TB) []conformanceCase {
	tb.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "conformance", "*.txtar"))
	if err != nil || len(files) == 0 {
		tb.Fatalf("no conformance archives: %v", err)
	}
	var cases []conformanceCase
	for _, path := range files {
		ar, err := txtar.ParseFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		parts := map[string]string{}
		for _, f := range ar.Files {
			parts[f.Name] = string(f.Data)
		}
		meta := map[string]string{}
		for line := range strings.Lines(parts["meta"]) {
			k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				tb.Fatalf("%s: meta line %q is not key: value", path, line)
			}
			meta[k] = strings.TrimSpace(v)
		}
		for _, k := range metaKeys {
			if _, ok := meta[k]; !ok {
				tb.Fatalf("%s: meta lacks %q", path, k)
			}
		}
		req, resp := unquote(tb, path, parts["request"]), unquote(tb, path, parts["response"])
		caseLine := strings.Count(string(ar.Comment), "\n") + 1
		for _, f := range ar.Files {
			if f.Name == "cases" {
				break
			}
			caseLine += 1 + strings.Count(string(f.Data), "\n")
		}
		for i, line := range strings.Split(strings.TrimSuffix(parts["cases"], "\n"), "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			name := fmt.Sprintf("%s:%d", path, caseLine+1+i)
			want, query, ok := strings.Cut(line, "\t")
			if !ok || (want != "true" && want != "false") {
				tb.Fatalf("%s: want \"true|false<TAB>query\", got %q", name, line)
			}
			cases = append(cases, conformanceCase{name: name, query: query, want: want == "true", request: req, response: resp, meta: meta})
		}
	}
	return cases
}

func unquote(tb testing.TB, path, s string) []byte {
	tb.Helper()
	u, err := strconv.Unquote(strings.TrimSpace(s))
	if err != nil {
		tb.Fatalf("%s: %v", path, err)
	}
	return []byte(u)
}

func seedQueries(tb testing.TB) []string {
	var qs []string
	for _, c := range loadConformance(tb) {
		qs = append(qs, c.query)
	}
	return qs
}

func TestConformanceCorpusParses(t *testing.T) {
	t.Parallel()
	cases := loadConformance(t)
	if len(cases) < 80 {
		t.Fatalf("corpus has %d cases, want >= 80", len(cases))
	}
	for _, c := range cases {
		n, err := Parse(c.query)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		roundTrip(t, c.query, n)
	}
}
