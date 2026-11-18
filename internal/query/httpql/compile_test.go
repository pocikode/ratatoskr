package httpql

import (
	"encoding/json/v2"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

var update = flag.Bool("update", false, "rewrite the expectations in testdata/*.txtar")

// TestSQLGolden pins the SQL text and bind values of the compiler
// (docs/05-go-conventions.md §4). Rewrite with -update.
func TestSQLGolden(t *testing.T) {
	path := filepath.Join("testdata", "sql.txtar")
	archive, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		updateSQLGolden(t, path, archive)
		return
	}
	for _, f := range archive.Files {
		name, field, ok := strings.Cut(f.Name, "/")
		if !ok {
			t.Fatalf("%s: %q is not named case/field", path, f.Name)
		}
		if field != "query" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			got := compileQuery(t, strings.TrimSpace(string(f.Data)))
			args, err := json.Marshal(got.Args)
			if err != nil {
				t.Fatal(err)
			}
			if want := caseField(archive, name, "where"); want != got.Where {
				t.Errorf("where = %q, want %q", got.Where, want)
			}
			if want := caseField(archive, name, "args"); want != string(args) {
				t.Errorf("args = %s, want %s", args, want)
			}
			if n, want := strings.Count(got.Where, "?"), len(got.Args); n != want {
				t.Errorf("%d placeholders for %d args", n, want)
			}
		})
	}
}

func updateSQLGolden(t *testing.T, path string, archive *txtar.Archive) {
	t.Helper()
	out := &txtar.Archive{Comment: archive.Comment}
	for _, f := range archive.Files {
		name, field, ok := strings.Cut(f.Name, "/")
		if !ok || field != "query" {
			continue
		}
		query := strings.TrimSpace(string(f.Data))
		got := compileQuery(t, query)
		args, err := json.Marshal(got.Args)
		if err != nil {
			t.Fatal(err)
		}
		out.Files = append(out.Files,
			txtar.File{Name: name + "/query", Data: []byte(query + "\n")},
			txtar.File{Name: name + "/where", Data: []byte(got.Where + "\n")},
			txtar.File{Name: name + "/args", Data: []byte(string(args) + "\n")},
		)
	}
	if err := os.WriteFile(path, txtar.Format(out), 0o600); err != nil {
		t.Fatal(err)
	}
}

func compileQuery(t *testing.T, query string) SQL {
	t.Helper()
	n, err := Parse(query)
	if err != nil {
		t.Fatalf("%q: %v", query, err)
	}
	got, err := Compile(n)
	if err != nil {
		t.Fatalf("%q: %v", query, err)
	}
	return got
}

func caseField(a *txtar.Archive, name, field string) string {
	for _, f := range a.Files {
		if f.Name == name+"/"+field {
			return strings.TrimSpace(string(f.Data))
		}
	}
	return ""
}

// TestSQLValuesAreBound asserts that values only ever travel as bind
// parameters, never in the SQL text.
func TestSQLValuesAreBound(t *testing.T) {
	t.Parallel()
	tests := []struct{ query, value string }{
		{`req.host.eq:"'; DROP TABLE requests; --"`, `'; DROP TABLE requests; --`},
		{`req.path.cont:"?) OR 1=1 --"`, `?) OR 1=1 --`},
		{`req.path.like:"%' OR '1'='1"`, `%' OR '1'='1`},
		{`req.header["x'); DELETE FROM requests; --"].eq:"v"`, "v"},
		{`req.body.regex:"(?i)secret'; --"`, `(?i)secret'; --`},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := compileQuery(t, tt.query)
			for _, bad := range []string{"DROP", "DELETE", "1=1", "'1'='1'"} {
				if strings.Contains(got.Where, bad) {
					t.Errorf("SQL text holds the value: %q", got.Where)
				}
			}
			if n, want := strings.Count(got.Where, "?"), len(got.Args); n != want {
				t.Errorf("%d placeholders for %d args: %q", n, want, got.Where)
			}
			if !slices.Contains(got.Args, any(tt.value)) {
				t.Errorf("value %q is not bound: %v", tt.value, got.Args)
			}
		})
	}
}

var longLike = &core.Clause{
	Namespace: "req", Field: "body", Op: core.Like, Loc: core.Span{End: 9},
	Value: core.Value{Type: core.Bytes, Str: strings.Repeat("a", maxLikePattern+1)},
}

func TestCompileErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node core.Node
	}{
		{"preset", &core.Preset{Name: "mine", Loc: core.Span{Start: 0, End: 10}}},
		{"operator", &core.Clause{
			Namespace: "req", Field: "host", Op: core.Gt, Loc: core.Span{End: 14},
			Value: core.Value{Type: core.String, Str: "x"},
		}},
		{"field", &core.Clause{Namespace: "finding", Field: "title", Loc: core.Span{End: 7}}},
		{"like-too-long", longLike},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Compile(tt.node); err == nil {
				t.Fatal("compiled an invalid node")
			}
		})
	}
	// SQLite fails a LIKE pattern over 50,000 bytes; the evaluator must agree.
	if _, err := Eval(longLike, nil); err == nil {
		t.Error("Eval accepted a like pattern over 50000 bytes")
	}
	if got, err := Compile(nil); err != nil || got.Where != "" || len(got.Args) != 0 {
		t.Errorf("Compile(nil) = %+v, %v; want an empty fragment", got, err)
	}
}
