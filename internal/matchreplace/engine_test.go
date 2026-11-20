package matchreplace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

func newEngine() *Engine { return New(slog.New(slog.DiscardHandler)) }

func headerAdd(term string) Rule {
	return Rule{Name: "r", Enabled: true, Sources: []Source{SourceProxy}, Section: RequestHeader, Operation: Add, Term: term}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	ok := Rule{
		Name: "rule", Sources: []Source{SourceProxy}, Section: RequestHeader, Operation: UpdateValue,
		Matcher: Matcher{Kind: Name, Value: "X-A"}, Term: "1",
	}
	tests := []struct {
		name string
		edit func(*Rule)
		code apperr.Code
		msg  string // substring
	}{
		{"valid", func(*Rule) {}, "", ""},
		{"empty name", func(r *Rule) { r.Name = "  " }, apperr.BadInput, "name"},
		{"long name", func(r *Rule) { r.Name = strings.Repeat("é", 201) }, apperr.BadInput, "name"},
		{"control in name", func(r *Rule) { r.Name = "a\x01" }, apperr.BadInput, "control"},
		{"no sources", func(r *Rule) { r.Sources = nil }, apperr.BadInput, "sources"},
		{"unknown source", func(r *Rule) { r.Sources = []Source{"FTP"} }, apperr.BadInput, "sources"},
		{"unknown section", func(r *Rule) { r.Section = "REQUEST_COOKIE" }, apperr.BadInput, "section"},
		{"invalid pair", func(r *Rule) { r.Section = RequestBody }, apperr.BadInput, "operation"},
		{"method takes UPDATE only", func(r *Rule) { r.Section, r.Operation = RequestMethod, UpdateRaw }, apperr.BadInput, "operation"},
		{"NAME not with UPDATE_RAW", func(r *Rule) { r.Operation = UpdateRaw }, apperr.BadInput, "matcher.kind"},
		{"unknown matcher", func(r *Rule) { r.Matcher.Kind = "GLOB" }, apperr.BadInput, "matcher.kind"},
		{"empty STRING value", func(r *Rule) { r.Matcher = Matcher{Kind: String} }, apperr.BadInput, "matcher.value"},
		{"empty NAME value", func(r *Rule) { r.Matcher = Matcher{Kind: Name} }, apperr.BadInput, "matcher.value"},
		{"FULL ignores value", func(r *Rule) { r.Matcher = Matcher{Kind: Full} }, "", ""},
		{"bad regex", func(r *Rule) { r.Matcher = Matcher{Kind: Regex, Value: "(?=a)"} }, apperr.BadInput, "matcher.value"},
		{"ADD ignores matcher", func(r *Rule) { r.Operation, r.Matcher, r.Term = Add, Matcher{Kind: "ANY"}, "X-A: 1" }, "", ""},
		{"ADD header without colon", func(r *Rule) { r.Operation, r.Term = Add, "X-A 1" }, apperr.BadInput, "term"},
		{"ADD header bad name", func(r *Rule) { r.Operation, r.Term = Add, "X A: 1" }, apperr.BadInput, "term"},
		{"ADD param as written", func(r *Rule) { r.Section, r.Operation, r.Term = RequestQuery, Add, "a b" }, "", ""},
		{"term too large", func(r *Rule) { r.Term = strings.Repeat("a", maxTermBytes+1) }, apperr.BadInput, "term"},
		{"value too large", func(r *Rule) { r.Matcher.Value = strings.Repeat("a", maxTermBytes+1) }, apperr.BadInput, "matcher.value"},
		{"condition too large", func(r *Rule) { r.Condition = strings.Repeat(" ", 64<<10+1) }, apperr.BadInput, "condition"},
		{"bad condition", func(r *Rule) { r.Condition = "req.method.eq:" }, apperr.BadQuery, ""},
		{"unknown preset", func(r *Rule) { r.Condition = "preset:nope" }, apperr.BadQuery, ""},
		{"req.tls rejected", func(r *Rule) { r.Condition = "req.tls.eq:false" }, apperr.BadQuery, "req.tls is not available"},
		{"source rejected", func(r *Rule) { r.Condition = "source:proxy" }, apperr.BadQuery, "source: is not available"},
		{"preset bringing row.id", func(r *Rule) { r.Condition = "preset:ids" }, apperr.BadQuery, "row.id is not available"},
	}
	presets := []filterpreset.Preset{{Name: "ids", Query: "row.id.gt:1"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := ok
			tt.edit(&r)
			err := Validate(r, presets)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			e, isApp := errors.AsType[*apperr.Error](err)
			if !isApp || e.Code != tt.code || !strings.Contains(e.Msg, tt.msg) {
				t.Fatalf("Validate = %#v, want %s containing %q", err, tt.code, tt.msg)
			}
			if tt.code == apperr.BadQuery && e.Span == nil {
				t.Error("BAD_QUERY without span")
			}
		})
	}
}

func TestConditionSpan(t *testing.T) {
	t.Parallel()
	r := headerAdd("X-A: 1")
	r.Condition = `req.method.eq:"GET" AND req.tls.eq:false`
	for name, err := range map[string]error{
		"Validate": Validate(r, nil),
		"Test":     testErr(r, KindRequest, "GET / HTTP/1.1\r\n\r\n"),
	} {
		e, ok := errors.AsType[*apperr.Error](err)
		if !ok || e.Code != apperr.BadQuery || e.Span == nil || *e.Span != (apperr.Span{Start: 24, End: 40}) {
			t.Errorf("%s = %#v, want BAD_QUERY at 24-40", name, err)
		}
	}
}

func testErr(r Rule, kind Kind, raw string) error {
	_, _, _, err := newEngine().Test(r, nil, kind, []byte(raw))
	return err
}

func TestTester(t *testing.T) {
	t.Parallel()
	body := Rule{Section: ResponseBody, Operation: UpdateRaw, Matcher: Matcher{Kind: Full}, Term: "x"}
	reqBody := Rule{Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: String, Value: "a"}, Term: "b"}
	tests := []struct {
		name    string
		rule    Rule
		kind    Kind
		raw     string
		code    apperr.Code
		out     string // expected output when no error
		matched bool
	}{
		{"other kind", body, KindRequest, "GET / HTTP/1.1\r\n\r\n", apperr.BadInput, "", false},
		{"bad kind", body, "WS", "GET / HTTP/1.1\r\n\r\n", apperr.BadInput, "", false},
		{"not a message", body, KindResponse, "nonsense\r\n\r\n", apperr.BadInput, "", false},
		{"content-length not decimal", reqBody, KindRequest, "POST / HTTP/1.1\r\nContent-Length: 0x1\r\n\r\n", apperr.BadInput, "", false},
		{"content-lengths disagree", reqBody, KindRequest, "POST / HTTP/1.1\r\nContent-Length: 1\r\nContent-Length: 2\r\n\r\na", apperr.BadInput, "", false},
		{"content-lengths agree with zeros", reqBody, KindRequest, "POST / HTTP/1.1\r\nContent-Length: 1\r\nContent-Length: 01\r\n\r\na",
			"", "POST / HTTP/1.1\r\nContent-Length: 1\r\n\r\nb", true},
		{"body shorter", reqBody, KindRequest, "POST / HTTP/1.1\r\nContent-Length: 5\r\n\r\na", apperr.BadInput, "", false},
		{"body longer", reqBody, KindRequest, "POST / HTTP/1.1\r\nContent-Length: 1\r\n\r\naa", apperr.BadInput, "", false},
		{"request bytes without framing", reqBody, KindRequest, "POST / HTTP/1.1\r\n\r\na", apperr.BadInput, "", false},
		{"header-only request valid", reqBody, KindRequest, "POST / HTTP/1.1\r\n\r\n", "", "POST / HTTP/1.1\r\n\r\n", true},
		{"chunked with content-length 999", reqBody, KindRequest,
			"POST / HTTP/1.1\r\nContent-Length: 999\r\nTransfer-Encoding: chunked\r\n\r\n1\r\na\r\n0\r\n\r\n",
			"", "POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n1\r\nb\r\n0\r\n\r\n", true},
		{"chunked with content-length abc", reqBody, KindRequest,
			"POST / HTTP/1.1\r\nContent-Length: abc\r\nTransfer-Encoding: chunked\r\n\r\n1\r\na\r\n0\r\n\r\n", apperr.BadInput, "", false},
		{"http/1.0 chunked length mismatch", reqBody, KindRequest,
			"POST / HTTP/1.0\r\nContent-Length: 3\r\nTransfer-Encoding: chunked\r\n\r\n1\r\na\r\n0\r\n\r\n", apperr.BadInput, "", false},
		{"transfer-encoding gzip", reqBody, KindRequest, "POST / HTTP/1.1\r\nTransfer-Encoding: gzip\r\n\r\n", apperr.BadInput, "", false},
		{"empty response any length", body, KindResponse, "HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\n",
			"", "HTTP/1.1 200 OK\r\nContent-Length: 50\r\n\r\n", true},
		{"empty response length 0 filled", body, KindResponse, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n",
			"", "HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\nx", true},
		{"empty response overflowing length", body, KindResponse, "HTTP/1.1 200 OK\r\nContent-Length: 9223372036854775808\r\n\r\n",
			apperr.BadInput, "", false},
		{"response body shorter", body, KindResponse, "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nab", apperr.BadInput, "", false},
		{"bad condition", Rule{Section: RequestHeader, Operation: Add, Term: "X: 1", Condition: "(("}, KindRequest,
			"GET / HTTP/1.1\r\n\r\n", apperr.BadQuery, "", false},
		{"unknown preset", Rule{Section: RequestHeader, Operation: Add, Term: "X: 1", Condition: "preset:nope"}, KindRequest,
			"GET / HTTP/1.1\r\n\r\n", apperr.BadQuery, "", false},
		{"condition not matched", Rule{Section: RequestHeader, Operation: Add, Term: "X: 1", Condition: `req.method.eq:"PUT"`}, KindRequest,
			"GET / HTTP/1.1\r\n\r\n", "", "GET / HTTP/1.1\r\n\r\n", false},
		{"resp clause false on request", Rule{Section: RequestHeader, Operation: Add, Term: "X: 1", Condition: "resp.code.eq:0"}, KindRequest,
			"GET / HTTP/1.1\r\n\r\n", "", "GET / HTTP/1.1\r\n\r\n", false},
		{"req clause false on response", Rule{Section: ResponseHeader, Operation: Add, Term: "X: 1", Condition: `req.method.ne:"x"`}, KindResponse,
			"HTTP/1.1 204 No Content\r\n\r\n", "", "HTTP/1.1 204 No Content\r\n\r\n", false},
		{"skipped status", Rule{Section: ResponseStatusCode, Operation: Update, Matcher: Matcher{Kind: Full}, Term: "20"}, KindResponse,
			"HTTP/1.1 204 No Content\r\n\r\n", "", "HTTP/1.1 204 No Content\r\n\r\n", true},
		{"skipped invalid result", Rule{Section: RequestFirstLine, Operation: UpdateRaw, Matcher: Matcher{Kind: Full}, Term: "x"}, KindRequest,
			"GET / HTTP/1.1\r\n\r\n", "", "GET / HTTP/1.1\r\n\r\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := []byte(tt.raw)
			out, matched, changed, err := newEngine().Test(tt.rule, nil, tt.kind, raw)
			if tt.code != "" {
				if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Code != tt.code || out != nil {
					t.Fatalf("Test = (%q, %v), want %s and nil output", out, err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.out || matched != tt.matched || changed != (tt.out != tt.raw) {
				t.Errorf("Test = (%q, matched %v, changed %v), want (%q, %v, %v)", out, matched, changed, tt.out, tt.matched, tt.out != tt.raw)
			}
			if !changed && &out[0] != &raw[0] {
				t.Error("unchanged output is not the input slice")
			}
		})
	}
}

func TestTesterCaps(t *testing.T) {
	t.Parallel()
	e := newEngine()
	big := []byte("POST / HTTP/1.1\r\nContent-Length: " + strconv.Itoa(maxTestInput) + "\r\n\r\n" + strings.Repeat("a", maxTestInput))
	if _, _, _, err := e.Test(Rule{Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: Full}}, nil, KindRequest, big); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("12 MiB + head: %v, want BAD_INPUT", err)
	}
	// 64 KiB body, each byte replaced by 1 KiB: 64 MiB would be built.
	raw := []byte("POST / HTTP/1.1\r\nContent-Length: 65536\r\n\r\n" + strings.Repeat("a", 64<<10))
	for _, kind := range []MatcherKind{String, Regex} {
		r := Rule{Section: RequestBody, Operation: UpdateRaw, Matcher: Matcher{Kind: kind, Value: "a"}, Term: strings.Repeat("b", 1<<10)}
		out, _, _, err := e.Test(r, nil, KindRequest, raw)
		if e, ok := errors.AsType[*apperr.Error](err); !ok || e.Code != apperr.BadInput || e.Msg != "tester output must be at most 16 MiB" || out != nil {
			t.Errorf("%s: (%d bytes, %v), want the 16 MiB BAD_INPUT", kind, len(out), err)
		}
	}
	// Header values, item by item.
	var head strings.Builder
	head.WriteString("GET / HTTP/1.1\r\n")
	for range 100 {
		head.WriteString("X: a\r\n")
	}
	head.WriteString("\r\n")
	r := Rule{Section: RequestHeader, Operation: UpdateValue, Matcher: Matcher{Kind: Full}, Term: strings.Repeat("b", 1<<20)}
	if _, _, _, err := e.Test(r, nil, KindRequest, []byte(head.String())); apperr.CodeOf(err) != apperr.BadInput {
		t.Errorf("header values: %v, want BAD_INPUT", err)
	}
}

func encodeWith(t *testing.T, coding string, p []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w io.WriteCloser
	switch coding {
	case "gzip":
		w = gzip.NewWriter(&buf)
	case "deflate":
		w = zlib.NewWriter(&buf)
	case "deflate-raw":
		w, _ = flate.NewWriter(&buf, flate.BestSpeed)
	case "br":
		w = brotli.NewWriter(&buf)
	case "zstd":
		w, _ = zstd.NewWriter(&buf)
	}
	if _, err := w.Write(p); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readResponse(t *testing.T, raw []byte) *msg {
	t.Helper()
	x, err := parse(raw, true, "")
	if err != nil {
		t.Fatalf("output does not parse: %v\n%q", err, raw)
	}
	if len(x.rest) != 0 {
		t.Fatalf("output has %d bytes after the body", len(x.rest))
	}
	return x
}

func TestBodyEncodings(t *testing.T) {
	t.Parallel()
	rule := Rule{Name: "b", Enabled: true, Sources: []Source{SourceProxy}, Section: ResponseBody, Operation: UpdateRaw,
		Matcher: Matcher{Kind: String, Value: "world"}, Term: "there"}
	for _, coding := range []string{"gzip", "deflate", "deflate-raw", "br", "zstd"} {
		for _, chunked := range []bool{false, true} {
			t.Run(coding+map[bool]string{true: "/chunked"}[chunked], func(t *testing.T) {
				t.Parallel()
				header := strings.TrimSuffix(coding, "-raw")
				enc := encodeWith(t, coding, []byte("hello world"))
				var raw []byte
				if chunked {
					raw = []byte("HTTP/1.1 200 OK\r\nContent-Encoding: " + header + "\r\nTransfer-Encoding: chunked\r\n\r\n" +
						strconv.FormatInt(int64(len(enc)), 16) + "\r\n" + string(enc) + "\r\n0\r\nX-T: 1\r\n\r\n")
				} else {
					raw = []byte("HTTP/1.1 200 OK\r\nContent-Encoding: " + header + "\r\nContent-Length: " + strconv.Itoa(len(enc)) + "\r\n\r\n" + string(enc))
				}
				e := newEngine()
				e.Set([]Rule{rule}, nil)
				out, err := e.Response(context.Background(), "proxy", []byte(defaultGoldenRequest), raw)
				if err != nil {
					t.Fatal(err)
				}
				x := readResponse(t, out)
				if v, _ := x.m.Header("Content-Encoding"); v != header {
					t.Errorf("Content-Encoding = %q", v)
				}
				payload := x.body
				if chunked {
					var trailer []byte
					payload, trailer, err = rawhttp.SplitChunkedBody(x.body)
					if err != nil || string(trailer) != "X-T: 1\r\n" {
						t.Fatalf("chunked body: %v, trailer %q", err, trailer)
					}
					if _, has := x.m.Header("Content-Length"); has {
						t.Error("chunked output has Content-Length")
					}
				} else if x.m.ContentLength() != int64(len(x.body)) {
					t.Errorf("Content-Length %d, body %d", x.m.ContentLength(), len(x.body))
				}
				if coding == "deflate-raw" && isZlib(payload) {
					t.Error("raw deflate re-encoded with a zlib wrapper")
				}
				plain, err := decode(map[bool]string{true: deflateRaw, false: header}[coding == "deflate-raw"], payload)
				if err != nil || string(plain) != "hello there" {
					t.Errorf("decoded %q, %v", plain, err)
				}
			})
		}
	}
}

func TestBodySkipped(t *testing.T) {
	t.Parallel()
	rule := Rule{Name: "b", Enabled: true, Sources: []Source{SourceProxy}, Section: ResponseBody, Operation: UpdateRaw,
		Matcher: Matcher{Kind: Full}, Term: "x"}
	gz := encodeWith(t, "gzip", []byte("hi"))
	for name, raw := range map[string]string{
		"stacked":   "HTTP/1.1 200 OK\r\nContent-Encoding: gzip, br\r\nContent-Length: 2\r\n\r\nhi",
		"two lines": "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Encoding: gzip\r\nContent-Length: " + strconv.Itoa(len(gz)) + "\r\n\r\n" + string(gz),
		"unknown":   "HTTP/1.1 200 OK\r\nContent-Encoding: compress\r\nContent-Length: 2\r\n\r\nhi",
		"corrupt":   "HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 2\r\n\r\nhi",
		"204":       "HTTP/1.1 204 No Content\r\n\r\n",
	} {
		e := newEngine()
		e.Set([]Rule{rule}, nil)
		in := []byte(raw)
		out, err := e.Response(context.Background(), "proxy", []byte(defaultGoldenRequest), in)
		if err != nil || &out[0] != &in[0] || len(out) != len(in) {
			t.Errorf("%s: got %q, %v; want the input slice", name, out, err)
		}
	}
}

func TestHooks(t *testing.T) {
	t.Parallel()
	e := newEngine()
	if e.Active("proxy", false) {
		t.Error("Active before Set")
	}
	raw := []byte("GET / HTTP/1.1\r\nHost: a\r\n\r\n")
	if out, err := e.Request(context.Background(), "proxy", raw); err != nil || &out[0] != &raw[0] {
		t.Errorf("no rules: %q, %v", out, err)
	}
	r := headerAdd("X: 1")
	r.Sources = []Source{SourceReplay, SourceIntercept}
	e.Set([]Rule{r}, nil)
	if !e.Active("replay", false) || !e.Active("intercept", false) || e.Active("proxy", false) || e.Active("replay", true) {
		t.Error("Active does not follow sources and side")
	}
	if _, err := e.Request(context.Background(), "replay", []byte("\r\n")); err == nil {
		t.Error("unparsable input: no error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Request(ctx, "replay", raw); !errors.Is(err, context.Canceled) {
		t.Errorf("done context: %v", err)
	}
	r.Section = ResponseHeader
	e.Set([]Rule{r}, nil)
	connect := []byte("HTTP/1.1 200 Connection established\r\n\r\n")
	out, err := e.Response(context.Background(), "replay", []byte("CONNECT a:443 HTTP/1.1\r\nHost: a:443\r\n\r\n"), connect)
	if err != nil || &out[0] != &connect[0] {
		t.Errorf("CONNECT 2xx rewritten: %q, %v", out, err)
	}
}

// BenchmarkHeaderRules is the F-MR-1 budget: 20 header rules on a 2 KiB request.
func BenchmarkHeaderRules(b *testing.B) {
	var head strings.Builder
	head.WriteString("GET /search?q=1 HTTP/1.1\r\nHost: example.com\r\n")
	for i := 0; head.Len() < 2<<10-40; i++ {
		head.WriteString("X-Header-" + strconv.Itoa(i) + ": " + strings.Repeat("v", 60) + "\r\n")
	}
	head.WriteString("\r\n")
	raw := []byte(head.String())
	var rules []Rule
	for i := range 20 {
		r := Rule{Name: "r", Enabled: true, Sources: []Source{SourceProxy}, Section: RequestHeader}
		switch i % 4 {
		case 0:
			r.Operation, r.Term = Add, "X-Added-"+strconv.Itoa(i)+": 1"
		case 1:
			r.Operation, r.Matcher, r.Term = UpdateValue, Matcher{Kind: Name, Value: "X-Header-" + strconv.Itoa(i)}, "new"
		case 2:
			r.Operation, r.Matcher, r.Term = UpdateRaw, Matcher{Kind: Regex, Value: `^X-Header-` + strconv.Itoa(i) + `:`}, "X-Renamed:"
		case 3:
			r.Operation, r.Matcher = Remove, Matcher{Kind: String, Value: "X-Header-" + strconv.Itoa(i) + ":"}
		}
		rules = append(rules, r)
	}
	e := newEngine()
	e.Set(rules, nil)
	ctx := context.Background()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		if _, err := e.Request(ctx, "proxy", raw); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSkippedRuleIsLogged(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	e := New(slog.New(slog.NewTextHandler(&logs, nil)))
	e.Set([]Rule{{Name: "bad-status", Enabled: true, Sources: []Source{SourceProxy}, Section: ResponseStatusCode,
		Operation: Update, Matcher: Matcher{Kind: Full}, Term: "2xx"}}, nil)
	resp := []byte("HTTP/1.1 204 No Content\r\n\r\n")
	out, err := e.Response(context.Background(), "proxy", []byte(defaultGoldenRequest), resp)
	if err != nil || &out[0] != &resp[0] {
		t.Fatalf("got %q, %v", out, err)
	}
	if s := logs.String(); !strings.Contains(s, "level=WARN") || !strings.Contains(s, "rule=bad-status") || !strings.Contains(s, "not three digits") {
		t.Errorf("log = %q", s)
	}
}

// TestSetCopiesSources: reusing the caller's Sources slice after Set must not
// change which traffic the published rules apply to.
func TestSetCopiesSources(t *testing.T) {
	t.Parallel()
	e := newEngine()
	r := headerAdd("X: 1")
	e.Set([]Rule{r}, nil)
	r.Sources[0] = SourceReplay
	if !e.Active("proxy", false) || e.Active("replay", false) {
		t.Fatal("caller's Sources slice changed the live rules")
	}
	raw := []byte("GET / HTTP/1.1\r\nHost: a\r\n\r\n")
	if out, err := e.Request(context.Background(), "proxy", raw); err != nil || bytes.Equal(out, raw) {
		t.Errorf("rule not applied after caller mutation: %q, %v", out, err)
	}
}
