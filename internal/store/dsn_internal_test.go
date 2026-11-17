package store

import "testing"

func TestURIPath(t *testing.T) {
	tests := []struct {
		in     string
		volume bool
		want   string
	}{
		{"/data/project.db", false, "/data/project.db"},
		{"C:/x/project.db", true, "/C:/x/project.db"},
	}
	for _, tt := range tests {
		if got := uriPath(tt.in, tt.volume); got != tt.want {
			t.Errorf("uriPath(%q, %v) = %q, want %q", tt.in, tt.volume, got, tt.want)
		}
	}
}
