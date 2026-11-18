package streamql

import (
	"errors"
	"strings"
	"testing"

	"github.com/pocikode/ratatoskr/internal/query/core"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{`"token"`, `ws.raw.cont:"token"`},
		{`ws.direction.eq:"client" AND ws.len.gt:100`, `ws.direction.eq:"client" AND ws.len.gt:100`},
		{`stream.host.like:"%.example.com" OR NOT stream.tls.eq:true`, `stream.host.like:"%.example.com" OR NOT stream.tls.eq:true`},
		{`ws.format.eq:"binary" and (stream.port.eq:443 or row.id.lt:5)`, `ws.format.eq:"binary" AND (stream.port.eq:443 OR row.id.lt:5)`},
		{`preset:ws-noise // comment`, `preset:ws-noise`},
	}
	for _, tt := range tests {
		n, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got := n.String(); got != tt.want {
			t.Errorf("Parse(%q).String() = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ in, msg string }{
		{`req.host.eq:"a"`, `unknown namespace "req"`},
		{`source:proxy`, `unknown clause source:`},
		{`ws.len.cont:"1"`, "`cont` is not valid for integer field ws.len"},
		{`ws.raw.eq:"a"`, "`eq` is not valid for bytes field ws.raw"},
	} {
		_, err := Parse(tt.in)
		if qe, ok := errors.AsType[*core.Error](err); !ok || !strings.Contains(qe.Msg, tt.msg) {
			t.Errorf("Parse(%q) = %v, want message containing %q", tt.in, err, tt.msg)
		}
	}
}

func FuzzStreamQL(f *testing.F) {
	for _, q := range []string{`"x"`, `ws.raw.regex:"^a" AND NOT ws.direction.eq:"server"`, `(ws.len.gte:1 OR row.id.eq:2) /* c */`, `preset:"P 1"`} {
		f.Add(q)
	}
	f.Fuzz(func(t *testing.T, src string) {
		n, err := Parse(src)
		if err != nil || n == nil {
			return
		}
		again, err := Parse(n.String())
		if err != nil || !core.Equal(n, again) {
			t.Fatalf("canonical %q of %q does not round-trip: %v", n.String(), src, err)
		}
	})
}
