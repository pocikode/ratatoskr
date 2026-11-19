package sender

import "testing"

func TestUpdateContentLength(t *testing.T) {
	const start = "POST / HTTP/1.1\r\n"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "transfer encoding",
			in:   "Transfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
			want: "Transfer-Encoding: chunked\r\n\r\n0\r\n\r\n",
		},
		{
			name: "missing length",
			in:   "Host: example.test\r\n\r\n\x00é",
			want: "Host: example.test\r\nContent-Length: 3\r\n\r\n\x00é",
		},
		{
			name: "wrong length",
			in:   "content-length: 9\r\n\r\nabc",
			want: "content-length: 3\r\n\r\nabc",
		},
		{
			name: "correct length stays byte exact",
			in:   "content-length:\t3\r\n\r\nabc",
			want: "content-length:\t3\r\n\r\nabc",
		},
		{
			name: "empty body",
			in:   "\r\n",
			want: "\r\n",
		},
		{
			name: "empty body with length",
			in:   "Content-Length: 9\r\n\r\n",
			want: "Content-Length: 0\r\n\r\n",
		},
		{
			name: "identical duplicate lengths",
			in:   "content-length: 8\r\nX-Keep:\tvalue\r\nContent-Length: 8\r\n\r\nabc",
			want: "content-length: 3\r\nX-Keep:\tvalue\r\n\r\nabc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, want := string(UpdateContentLength([]byte(start+tt.in))), start+tt.want; got != want {
				t.Errorf("UpdateContentLength() = %q, want %q", got, want)
			}
		})
	}
}

func TestUpdateContentLengthPreservesLeadingBlankLine(t *testing.T) {
	const input = "\r\nPOST / HTTP/1.1\r\n\r\nabc"
	const want = "\r\nPOST / HTTP/1.1\r\nContent-Length: 3\r\n\r\nabc"
	if got := string(UpdateContentLength([]byte(input))); got != want {
		t.Errorf("UpdateContentLength() = %q, want %q", got, want)
	}
}
