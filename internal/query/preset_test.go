package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocikode/ratatoskr/internal/query"
	"github.com/pocikode/ratatoskr/internal/query/core"
	"github.com/pocikode/ratatoskr/internal/query/httpql"
)

func lookupMap(m map[string]string) query.PresetLookup {
	return func(ref string) (string, string, bool) {
		src, ok := m[ref]
		return ref, src, ok
	}
}

func TestExpandPresets(t *testing.T) {
	t.Parallel()

	chain := map[string]string{}
	for i := range 9 { // p0 -> p1 -> ... -> p8: nine levels
		chain["p"+string(rune('0'+i))] = "preset:p" + string(rune('1'+i))
	}
	chain["p8"] = `req.method.eq:"GET"`

	tests := []struct {
		name    string
		query   string
		presets map[string]string
		want    string
		wantErr string
	}{
		{"none", `req.host.eq:"a"`, nil, `req.host.eq:"a"`, ""},
		{"simple", `preset:ok AND req.port.eq:80`, map[string]string{"ok": `resp.code.eq:200`}, `resp.code.eq:200 AND req.port.eq:80`, ""},
		{"quoted name", `NOT preset:"My Preset"`, map[string]string{"My Preset": `req.tls.eq:true OR req.port.eq:1`}, `NOT (req.tls.eq:true OR req.port.eq:1)`, ""},
		{"nested", `preset:a`, map[string]string{"a": `preset:b`, "b": `row.id.gt:3`}, `row.id.gt:3`, ""},
		{"same preset twice is no cycle", `preset:a OR preset:a`, map[string]string{"a": `row.id.gt:3`}, `row.id.gt:3 OR row.id.gt:3`, ""},
		{"unknown", `preset:nope`, nil, "", `unknown preset "nope"`},
		{"self cycle", `preset:a`, map[string]string{"a": `preset:a`}, "", "cycle"},
		{"indirect cycle", `preset:a`, map[string]string{"a": `preset:b`, "b": `req.port.eq:1 AND preset:a`}, "", "cycle"},
		{"depth 8 ok", `preset:p1`, chain, `req.method.eq:"GET"`, ""},
		{"depth 9 rejected", `preset:p0`, chain, "", "deeper than 8"},
		{"bad preset query", `preset:a`, map[string]string{"a": `req.nope.eq:1`}, "", `preset "a"`},
		{"empty preset", `preset:a`, map[string]string{"a": ` `}, "", "is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			n, err := httpql.Language.Parse(tt.query)
			if err != nil {
				t.Fatal(err)
			}

			got, err := query.ExpandPresets(httpql.Language, n, lookupMap(tt.presets))
			if tt.wantErr != "" {
				ce, ok := errors.AsType[*core.Error](err)
				if !ok || !strings.Contains(ce.Msg, tt.wantErr) {
					t.Fatalf("err = %v, want core.Error containing %q", err, tt.wantErr)
				}

				if ce.Span.End > len(tt.query) {
					t.Errorf("span %v outside query %q", ce.Span, tt.query)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got.String() != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
