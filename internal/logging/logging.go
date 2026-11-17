// Package logging builds the process logger: text records on stderr fanned out
// with an in-memory ring buffer that the Logs page reads
// (docs/03-architecture.md §9).
package logging

import (
	"io"
	"log/slog"
)

// New returns the process logger and the ring buffer that mirrors it.
//
// Records go to w (normally stderr) and into the ring, through a single
// [slog.NewMultiHandler] so both sinks share the same level and the same record.
func New(w io.Writer, level slog.Level) (*slog.Logger, *Ring) {
	ring := NewRing(Options{Level: level})

	text := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})

	return slog.New(slog.NewMultiHandler(text, ring.Handler())), ring
}
