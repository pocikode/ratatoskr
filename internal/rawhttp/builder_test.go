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

func TestBuilderSetChunkedBody(t *testing.T) {
	t.Parallel()
	const start = "POST / HTTP/1.1\r\nHost: a\r\nTransfer-Encoding:  chunked\r\n"
	const trailer = "X-Trailer: v\r\n"
	const wire = start + "Content-Length: 3\r\nTrailer: X-Trailer\r\n\r\n3\r\nabc\r\n0\r\n" + trailer + "\r\n"
	for _, tt := range []struct {
		payload, chunk string
	}{
		{"hello world", "b\r\nhello world\r\n"},
		{"", ""},
	} {
		got := request(t, wire).Builder().SetChunkedBody([]byte(tt.payload), []byte(trailer)).Bytes()
		want := start + "Trailer: X-Trailer\r\n\r\n" + tt.chunk + "0\r\n" + trailer + "\r\n"
		if string(got) != want {
			t.Errorf("payload %q wrote %q, want %q", tt.payload, got, want)
		}
	}
}

func TestBuilderSetHeadersAndHead(t *testing.T) {
	t.Parallel()
	req := request(t, "POST / HTTP/1.1\r\nHost:  a\r\nX-Old: 1\r\nContent-Length: 2\r\n\r\nhi")
	var hs []Header
	for _, h := range req.Headers() {
		if h.Name != "X-Old" {
			hs = append(hs, h)
		}
	}
	hs = append(hs, Header{Name: "X-New", Value: "2"}, Header{Raw: "X-Raw:3\r\n"})
	b := req.Builder().SetHeaders(hs)
	const head = "POST / HTTP/1.1\r\nHost:  a\r\nContent-Length: 2\r\nX-New: 2\r\nX-Raw:3\r\n\r\n"
	if got := b.Head(); string(got) != head {
		t.Errorf("Head() = %q, want %q", got, head)
	}
	if got := b.Bytes(); string(got) != head+"hi" {
		t.Errorf("Bytes() = %q, want %q", got, head+"hi")
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

// TestBuilderWrongKindStartLine keeps a rewrite rule that sets the wrong
// start-line field from panicking, and from editing the line it cannot own.
func TestBuilderWrongKindStartLine(t *testing.T) {
	t.Parallel()
	const wire = "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nhi"
	if got := response(t, "", wire).Builder().SetMethod("POST").SetTarget("/x").Bytes(); string(got) != wire {
		t.Errorf("method/target on a response wrote %q, want %q", got, wire)
	}
	const reqWire = "POST / HTTP/1.1\r\nContent-Length: 2\r\n\r\nhi"
	if got := request(t, reqWire).Builder().SetStatus(500, "Server Error").Bytes(); string(got) != reqWire {
		t.Errorf("status on a request wrote %q, want %q", got, reqWire)
	}
}

func TestBuilderSynthHeadersKeepBareLF(t *testing.T) {
	t.Parallel()
	req := request(t, "GET / HTTP/1.1\nHost: a\n\n")
	got := req.Builder().AddHeader("X-New", "1").Bytes()
	if want := "GET / HTTP/1.1\nHost: a\nX-New: 1\n\n"; string(got) != want {
		t.Errorf("wrote %q, want %q", got, want)
	}
}
