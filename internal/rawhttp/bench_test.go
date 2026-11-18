package rawhttp

import (
	"bufio"
	"strings"
	"testing"
)

func benchmarkRequest(b testing.TB) *Request {
	b.Helper()
	wire := "GET /a/very/long/path?with=query&and=more HTTP/1.1\r\n" +
		"Host: example.com\r\nUser-Agent: bench\r\nAccept: */*\r\n" +
		"Accept-Encoding: gzip, deflate\r\nConnection: keep-alive\r\n\r\n"
	req, err := ReadRequest(bufio.NewReader(strings.NewReader(wire)), DefaultLimits)
	if err != nil {
		b.Fatal(err)
	}
	return req
}

func BenchmarkHeaderIteration(b *testing.B) {
	req := benchmarkRequest(b)
	b.ReportAllocs()
	for b.Loop() {
		for _, h := range req.Headers() {
			_ = h.Name
		}
	}
}

// BenchmarkObsFold holds a big fold block to linear time (T-010 review).
func BenchmarkObsFold(b *testing.B) {
	wire := "GET / HTTP/1.1\r\nX-Fold: first\r\n" + strings.Repeat(" x\r\n", 50_000) + "\r\n"
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ReadRequest(bufio.NewReader(strings.NewReader(wire)), DefaultLimits); err != nil {
			b.Fatal(err)
		}
	}
}

// TestHeaderIterationDoesNotAllocate holds the capture path to the budget in
// docs/05-go-conventions.md: iterating parsed headers costs nothing.
func TestHeaderIterationDoesNotAllocate(t *testing.T) {
	req := benchmarkRequest(t)
	allocs := testing.AllocsPerRun(100, func() {
		for _, h := range req.Headers() {
			_ = h.Value
		}
	})
	if allocs != 0 {
		t.Errorf("header iteration allocates %.1f times per run", allocs)
	}
}
