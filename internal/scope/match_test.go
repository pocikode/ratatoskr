package scope_test

import (
	"testing"

	"github.com/pocikode/ratatoskr/internal/scope"
)

func TestMatcher(t *testing.T) {
	t.Parallel()

	type target struct {
		host string
		port int
		path string
		want bool
	}

	tests := []struct {
		name        string
		allow, deny []string
		targets     []target
	}{
		{"empty allows all", nil, nil, []target{{"anything.test", 80, "/", true}}},
		{"only deny", nil, []string{"evil.com"}, []target{{"evil.com", 80, "/", false}, {"ok.com", 80, "/", true}}},
		{"leading wildcard", []string{"*.example.com"}, nil, []target{
			{"a.example.com", 443, "/", true},
			{"A.B.Example.COM", 443, "/", true},
			{"example.com", 443, "/", false},
			{"badexample.com", 443, "/", false},
		}},
		{"middle wildcard", []string{"api.*.com"}, nil, []target{{"api.foo.com", 1, "/", true}, {"api.foo.org", 1, "/", false}}},
		{"question mark", []string{"h?.test"}, nil, []target{{"h1.test", 1, "/", true}, {"h12.test", 1, "/", false}}},
		{"port", []string{"a.com:8080"}, nil, []target{{"a.com", 8080, "/", true}, {"a.com", 80, "/", false}, {"a.com", 0, "/", false}}},
		{"path prefix", []string{"a.com/api"}, nil, []target{{"a.com", 80, "/api/x", true}, {"a.com", 80, "/apix", true}, {"a.com", 80, "/web", false}}},
		{"port and path", []string{"a.com:81/api"}, nil, []target{{"a.com", 81, "/api", true}, {"a.com", 82, "/api", false}, {"a.com", 81, "/", false}}},
		{"deny wins over allow", []string{"*.example.com"}, []string{"admin.example.com"}, []target{
			{"www.example.com", 80, "/", true}, {"admin.example.com", 80, "/", false}}},
		{"deny by path", []string{"a.com"}, []string{"a.com/private"}, []target{{"a.com", 80, "/", true}, {"a.com", 80, "/private/x", false}}},
		{"ipv4", []string{"10.0.*.*"}, nil, []target{{"10.0.3.4", 80, "/", true}, {"10.1.3.4", 80, "/", false}}},
		{"ipv6 literal", []string{"[::1]:8080"}, nil, []target{{"::1", 8080, "/", true}, {"::1", 80, "/", false}, {"::2", 8080, "/", false}}},
		{"ipv6 normalised", []string{"[2001:0DB8:0:0:0:0:0:1]"}, nil, []target{{"2001:db8::1", 80, "/", true}}},
		{"idn rule", []string{"bücher.example"}, nil, []target{{"xn--bcher-kva.example", 443, "/", true}, {"bucher.example", 443, "/", false}}},
		{"idn glob", []string{"*.日本.jp"}, nil, []target{{"www.xn--wgv71a.jp", 443, "/", true}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := scope.NewMatcher(tt.allow, tt.deny)
			if err != nil {
				t.Fatal(err)
			}

			for _, g := range tt.targets {
				if got := m.InScope(g.host, g.port, g.path); got != g.want {
					t.Errorf("InScope(%q, %d, %q) = %v, want %v", g.host, g.port, g.path, got, g.want)
				}
			}
		})
	}
}

func TestParseRuleErrors(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"", " ", ":80", "a.com:", "a.com:0", "a.com:99999", "a.com:x", "[::1", "[zz]", "/path"} {
		if _, err := scope.ParseRule(s); err == nil {
			t.Errorf("ParseRule(%q) succeeded", s)
		}
	}
}

func FuzzParseRule(f *testing.F) {
	for _, s := range []string{"a.com", "*.a.com:80/x", "[::1]:1/", "bücher.de", "a:1/"} {
		f.Add(s)
	}

	f.Fuzz(func(_ *testing.T, s string) {
		r, err := scope.ParseRule(s)
		if err == nil {
			r.Match("a.com", 80, "/")
		}
	})
}
