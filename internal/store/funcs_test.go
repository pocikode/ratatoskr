package store

import (
	"bytes"
	"testing"
	"time"
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

func TestContainsFoldLinear(t *testing.T) {
	hay := bytes.Repeat([]byte("e"), 1<<20)
	start := time.Now()
	if containsFold(hay, []byte("ex")) {
		t.Fatal("unexpected match")
	}
	// The quadratic version took seconds here; linear takes ~1ms.
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("containsFold too slow on adversarial input: %v", d)
	}
}

func BenchmarkContainsFold(b *testing.B) {
	hay := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog. "), 1<<20/45)
	needle := []byte("ERROR")
	for b.Loop() {
		containsFold(hay, needle)
	}
}
