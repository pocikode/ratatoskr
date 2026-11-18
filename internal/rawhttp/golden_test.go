package rawhttp

import (
	"bufio"
	"bytes"
	"flag"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"
)

var update = flag.Bool("update", false, "rewrite the expectations in testdata/*.txtar")

// TestGolden round-trips the committed corpus: every wire message must come
// back byte for byte from Raw, and the bytes left in the reader must match.
func TestGolden(t *testing.T) {
	for _, name := range []string{"requests", "responses"} {
		t.Run(name, func(t *testing.T) {
			runGolden(t, name+".txtar")
		})
	}
}

type goldenResult struct {
	raw      []byte
	rest     []byte
	warnings []string
}

func runGolden(t *testing.T, file string) {
	t.Helper()
	path := filepath.Join("testdata", file)
	archive, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := make(map[string]map[string]string, len(archive.Files))
	for _, f := range archive.Files {
		name, field, ok := strings.Cut(f.Name, "/")
		if !ok {
			t.Fatalf("%s: %q is not named case/field", path, f.Name)
		}
		if cases[name] == nil {
			cases[name] = map[string]string{}
		}
		cases[name][field] = string(f.Data)
	}

	names := slices.Sorted(maps.Keys(cases))
	for _, name := range names {
		fields := cases[name]
		result, err := execute(t, fields)
		if *update {
			if err != nil {
				t.Logf("%s: skipping update of error case: %v", name, err)
				continue
			}
			updateCase(fields, result)
			continue
		}
		check(t, name, fields, result, err)
	}
	if *update {
		writeArchive(t, path, archive.Comment, names, cases)
	}
}

func execute(t *testing.T, fields map[string]string) (goldenResult, error) {
	t.Helper()
	var result goldenResult
	wire := unquote(t, fields["wire"])
	limits := DefaultLimits
	if field, ok := fields["limit"]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			t.Fatalf("limit: %v", err)
		}
		limits.MaxHeadBytes = n
	}

	br := bufio.NewReader(bytes.NewReader(wire))
	kindFields := strings.Fields(fields["kind"])
	if len(kindFields) == 0 {
		t.Fatalf("case has no kind")
	}
	var msg *Message
	switch kindFields[0] {
	case "request":
		req, err := ReadRequest(br, limits)
		if err != nil {
			return result, err
		}
		msg = &req.Message
	case "response":
		method := "GET"
		if len(kindFields) > 1 {
			method = kindFields[1]
		}
		res, err := ReadResponse(br, method, limits)
		if err != nil {
			return result, err
		}
		msg = &res.Message
	default:
		t.Fatalf("unknown kind %q", kindFields[0])
	}

	if _, err := io.Copy(io.Discard, msg.DecodedBody()); err != nil {
		return result, err
	}
	result.raw = msg.Raw()
	result.warnings = msg.Warnings()
	rest, err := io.ReadAll(br)
	if err != nil {
		return result, err
	}
	result.rest = rest
	return result, nil
}

func check(t *testing.T, name string, fields map[string]string, result goldenResult, err error) {
	t.Helper()
	if want, ok := fields["error"]; ok {
		if err == nil {
			t.Fatalf("%s: want error containing %q, got none", name, strings.TrimSpace(want))
		}
		if !strings.Contains(err.Error(), strings.TrimSpace(want)) {
			t.Fatalf("%s: error %q does not contain %q", name, err, strings.TrimSpace(want))
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	wantRaw := unquote(t, fields["wire"])
	if field, ok := fields["raw"]; ok {
		wantRaw = unquote(t, field)
	}
	if !bytes.Equal(result.raw, wantRaw) {
		t.Errorf("%s: raw\n got %q\nwant %q", name, result.raw, wantRaw)
	}
	var wantRest []byte
	if field, ok := fields["rest"]; ok {
		wantRest = unquote(t, field)
	}
	if !bytes.Equal(result.rest, wantRest) {
		t.Errorf("%s: reader rest\n got %q\nwant %q", name, result.rest, wantRest)
	}
	if want := warningSet(fields["warnings"]); !slices.Equal(want, sorted(result.warnings)) {
		t.Errorf("%s: warnings\n got %q\nwant %q", name, sorted(result.warnings), want)
	}
}

func updateCase(fields map[string]string, result goldenResult) {
	wire := unquote(nil, fields["wire"])
	set := func(name string, value string) {
		if value == "" {
			delete(fields, name)
			return
		}
		fields[name] = value + "\n"
	}
	if !bytes.Equal(result.raw, wire) {
		set("raw", strconv.Quote(string(result.raw)))
	} else {
		set("raw", "")
	}
	if len(result.rest) > 0 {
		set("rest", strconv.Quote(string(result.rest)))
	} else {
		set("rest", "")
	}
	if warnings := sorted(result.warnings); len(warnings) > 0 {
		set("warnings", strings.Join(warnings, ","))
	} else {
		set("warnings", "")
	}
}

func writeArchive(t *testing.T, path string, comment []byte, names []string, cases map[string]map[string]string) {
	t.Helper()
	archive := &txtar.Archive{Comment: comment}
	fields := []string{"kind", "wire", "limit", "raw", "rest", "warnings", "error"}
	for _, name := range names {
		for _, field := range fields {
			value, ok := cases[name][field]
			if !ok {
				continue
			}
			archive.Files = append(archive.Files, txtar.File{Name: name + "/" + field, Data: []byte(value)})
		}
	}
	if err := os.WriteFile(path, txtar.Format(archive), 0o600); err != nil {
		t.Fatal(err)
	}
}

func unquote(t *testing.T, text string) []byte {
	if t != nil {
		t.Helper()
	}
	s, err := strconv.Unquote(strings.TrimSpace(text))
	if err != nil {
		if t != nil {
			t.Fatalf("unquote %q: %v", text, err)
		}
		return nil
	}
	return []byte(s)
}

func warningSet(text string) []string {
	var warnings []string
	for w := range strings.SplitSeq(text, ",") {
		if w = strings.TrimSpace(w); w != "" {
			warnings = append(warnings, w)
		}
	}
	slices.Sort(warnings)
	return warnings
}

func sorted(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return out
}
