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
