package rawhttp

import (
	"testing"
)

func TestBuilderUnchanged(t *testing.T) {
	t.Parallel()
	wire := "POST /old HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\n\r\nhi"
	if got := request(t, wire).Builder().Bytes(); string(got) != wire {
		t.Errorf("Builder without edits wrote %q", got)
	}
}

func TestBuilderEdits(t *testing.T) {
	t.Parallel()
	const wire = "POST /old HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\n\r\nhi"

	tests := []struct {
		name  string
		build func(*Builder) *Builder
		want  string
	}{
		{
			name:  "method",
			build: func(b *Builder) *Builder { return b.SetMethod("PUT") },
			want:  "PUT /old HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\n\r\nhi",
		},
		{
			name:  "target",
			build: func(b *Builder) *Builder { return b.SetTarget("/new?x=1") },
			want:  "POST /new?x=1 HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\n\r\nhi",
		},
		{
			name:  "set header keeps position and spelling",
			build: func(b *Builder) *Builder { return b.SetHeader("host", "b") },
			want:  "POST /old HTTP/1.1\r\nHost: b\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\n\r\nhi",
		},
		{
			name:  "add header",
			build: func(b *Builder) *Builder { return b.AddHeader("X-New", "1") },
			want:  "POST /old HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 2\r\nX-New: 1\r\n\r\nhi",
		},
		{
			name:  "delete header",
			build: func(b *Builder) *Builder { return b.DelHeader("x-keep") },
			want:  "POST /old HTTP/1.1\r\nHost: a\r\nContent-Length: 2\r\n\r\nhi",
		},
		{
			name:  "body",
			build: func(b *Builder) *Builder { return b.SetBody([]byte("hello")) },
			want:  "POST /old HTTP/1.1\r\nHost: a\r\nX-Keep:  spaced   \r\nContent-Length: 5\r\n\r\nhello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.build(request(t, wire).Builder()).Bytes(); string(got) != tt.want {
				t.Errorf("wrote %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuilderReplacesDuplicateHeaders(t *testing.T) {
	t.Parallel()
	req := request(t, "GET / HTTP/1.1\r\nX-Dup: 1\r\nHost: a\r\nX-Dup: 2\r\n\r\n")
	got := req.Builder().SetHeader("x-dup", "3").Bytes()
	if want := "GET / HTTP/1.1\r\nX-Dup: 3\r\nHost: a\r\n\r\n"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestBuilderDropsChunkedFraming(t *testing.T) {
	t.Parallel()
	req := request(t, "POST / HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n0\r\n\r\n")
	got := req.Builder().SetBody([]byte("new body")).Bytes()
	if want := "POST / HTTP/1.1\r\nContent-Length: 8\r\n\r\nnew body"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestBuilderStatusLine(t *testing.T) {
	t.Parallel()
	res := response(t, "", "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nhi")
	got := res.Builder().SetStatus(404, "Not Found").Bytes()
	if want := "HTTP/1.1 404 Not Found\r\nContent-Length: 2\r\n\r\nhi"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestBuilderKeepsBareLF(t *testing.T) {
	t.Parallel()
	req := request(t, "GET / HTTP/1.1\nHost: a\n\n")
	got := req.Builder().SetMethod("HEAD").Bytes()
	if want := "HEAD / HTTP/1.1\nHost: a\n\n"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}

func TestBuilderAddsHeadersToEmptyBlock(t *testing.T) {
	t.Parallel()
	req := request(t, "GET / HTTP/1.1\r\n\r\n")
	got := req.Builder().AddHeader("Host", "a").Bytes()
	if want := "GET / HTTP/1.1\r\nHost: a\r\n\r\n"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}
