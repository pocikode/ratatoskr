package query

import "testing"

func TestSideBody(t *testing.T) {
	t.Parallel()
	raw := []byte("GET / HTTP/1.1\r\n\r\nbody")
	tests := []struct {
		offset int
		want   string
	}{
		{len(raw) - 4, "body"},
		{len(raw), ""},
		{len(raw) + 10, ""}, // past the end clamps
		{-3, string(raw)},   // negative clamps to the start
	}
	for _, tt := range tests {
		if got := string(Side{Raw: raw, BodyOffset: tt.offset}.Body()); got != tt.want {
			t.Errorf("BodyOffset %d: body = %q, want %q", tt.offset, got, tt.want)
		}
	}
}
