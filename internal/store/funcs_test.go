package store

import (
	"bytes"
	"testing"
)

func asciiLower(b []byte) []byte {
	out := bytes.Clone(b)
	for i := range out {
		out[i] = lowerASCII(out[i])
	}
	return out
}

func FuzzContainsFold(f *testing.F) {
	f.Add([]byte("Hello WORLD"), []byte("world"))
	f.Add([]byte{0, 0xff, 'A'}, []byte("a"))
	f.Add([]byte(""), []byte(""))
	f.Fuzz(func(t *testing.T, hay, needle []byte) {
		want := bytes.Contains(asciiLower(hay), asciiLower(needle))
		if got := containsFold(hay, needle); got != want {
			t.Fatalf("containsFold(%q, %q) = %v, want %v", hay, needle, got, want)
		}
	})
}
