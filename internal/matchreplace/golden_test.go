package matchreplace

import (
	"bytes"
	"encoding/json/v2"
	"flag"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/pocikode/ratatoskr/internal/filterpreset"
)

var update = flag.Bool("update", false, "rewrite the expected outputs in testdata/*.txtar")

const defaultGoldenRequest = "GET / HTTP/1.1\r\nHost: a\r\n\r\n"

// TestGolden runs each case of testdata/*.txtar through the engine as proxy
// traffic. A case has `rules` (one Rule JSON per line; enabled and sources
// default to true and PROXY), optional `presets` (one Preset JSON per line),
// `in` (Go-quoted; default: the archive's `default/request` or
// `default/response`), `req` for responses (Go-quoted; default a bare GET) and
// the expected `out` (Go-quoted). An output equal to the input must be the
// input slice itself.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.txtar"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files: %v", err)
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) { runGolden(t, path) })
	}
}

func runGolden(t *testing.T, path string) {
	archive, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]map[string]string{}
	var order []string
	for _, f := range archive.Files {
		name, field, ok := strings.Cut(f.Name, "/")
		if !ok {
			t.Fatalf("%q is not case/field", f.Name)
		}
		if cases[name] == nil {
			cases[name] = map[string]string{}
			order = append(order, name)
		}
		cases[name][field] = string(f.Data)
	}
	defaults := cases["default"]
	for _, name := range order {
		if name == "default" {
			continue
		}
		fields := cases[name]
		t.Run(name, func(t *testing.T) {
			got := runCase(t, fields, defaults)
			if *update {
				fields["out"] = strconv.Quote(string(got)) + "\n"
				return
			}
			if want := unquote(t, fields["out"]); !bytes.Equal(got, want) {
				t.Errorf("output\n got %q\nwant %q", got, want)
			}
		})
	}
	if *update {
		var files []txtar.File
		for _, name := range order {
			for _, field := range slices.Sorted(maps.Keys(cases[name])) {
				files = append(files, txtar.File{Name: name + "/" + field, Data: []byte(cases[name][field])})
			}
		}
		archive.Files = files
		if err := os.WriteFile(path, txtar.Format(archive), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func runCase(t *testing.T, fields, defaults map[string]string) []byte {
	t.Helper()
	var rules []Rule
	for line := range strings.Lines(fields["rules"]) {
		r := Rule{Enabled: true, Sources: []Source{SourceProxy}}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("rule %q: %v", line, err)
		}
		rules = append(rules, r)
	}
	if len(rules) == 0 {
		t.Fatal("case has no rules")
	}
	var presets []filterpreset.Preset
	for line := range strings.Lines(fields["presets"]) {
		var p filterpreset.Preset
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			t.Fatalf("preset %q: %v", line, err)
		}
		presets = append(presets, p)
	}
	resp := rules[0].Section.response()
	in, ok := fields["in"]
	if !ok {
		in = defaults[kindName(resp)]
	}
	raw := unquote(t, in)

	e := New(slog.New(slog.DiscardHandler))
	e.Set(rules, presets)
	var got []byte
	var err error
	if resp {
		req := []byte(defaultGoldenRequest)
		if s, ok := fields["req"]; ok {
			req = unquote(t, s)
		}
		got, err = e.Response(t.Context(), "proxy", req, raw)
	} else {
		got, err = e.Request(t.Context(), "proxy", raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, raw) && &got[0] != &raw[0] {
		t.Error("unchanged output is a copy, want the input slice")
	}
	return got
}

func unquote(t *testing.T, s string) []byte {
	t.Helper()
	u, err := strconv.Unquote(strings.TrimSpace(s))
	if err != nil {
		t.Fatalf("unquote %q: %v", s, err)
	}
	return []byte(u)
}
