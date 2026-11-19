package intercept

import "testing"

func TestReplacementContentLength(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, input, want string
		update            bool
	}{
		{"transfer encoding", "Transfer-Encoding: chunked\r\n\r\n1\r\na\r\n0\r\n\r\n", "Transfer-Encoding: chunked\r\n\r\n1\r\na\r\n0\r\n\r\n", true},
		{"body without length", "\r\nabc", "Content-Length: 3\r\n\r\nabc", true},
		{"wrong length", "content-Length: 1\r\n\r\nabc", "content-Length: 3\r\n\r\nabc", true},
		{"no body or length", "\r\n", "\r\n", true},
		{"empty body with length", "Content-Length: 5\r\n\r\n", "Content-Length: 0\r\n\r\n", true},
		{"identical duplicates", "Content-Length: 3\r\nX-Test: example\r\ncontent-length: 3\r\n\r\nabc", "Content-Length: 3\r\nX-Test: example\r\n\r\nabc", true},
		{"update disabled", "Content-Length: 1\r\n\r\nabc", "Content-Length: 1\r\n\r\nabc", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for kind, line := range map[Kind]string{
				Request: "POST / HTTP/1.1\r\n", Response: "HTTP/1.1 200 OK\r\n",
			} {
				prefix := "\r\n" + line
				got, err := replacement(kind, []byte(prefix+tt.input), tt.update)
				if err != nil || string(got) != prefix+tt.want {
					t.Errorf("%s replacement = %q, %v, want %q", kind, got, err, prefix+tt.want)
				}
			}
		})
	}
}
