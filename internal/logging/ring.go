package logging

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// DefaultCapacity is how many log entries the ring keeps: the last 10k lines
	// that the Logs page shows.
	DefaultCapacity = 10_000
	// DefaultMaxBytes bounds the memory a full ring may use.
	DefaultMaxBytes = 8 << 20
	// maxEntryBytes bounds a single entry, so one huge attribute cannot flush
	// the whole ring.
	maxEntryBytes = 4 << 10
)

// Entry is one captured log record. Attrs are resolved and flattened: a record
// logged through WithGroup("g") has keys like "g.key".
type Entry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   []slog.Attr
	// Truncated reports that the record exceeded maxEntryBytes and lost content.
	Truncated bool

	size int
}

// Options configure a [Ring].
type Options struct {
	// Capacity is the number of entries to keep. Zero means [DefaultCapacity].
	Capacity int
	// MaxBytes is the memory budget. Zero means [DefaultMaxBytes].
	MaxBytes int
	// Level is the lowest level the ring records.
	Level slog.Level
}

// Ring keeps the most recent log entries in memory, within a fixed entry count
// and byte budget. It is safe for concurrent use.
type Ring struct {
	mu       sync.Mutex
	buf      []Entry
	start    int
	n        int
	bytes    int
	capacity int
	maxBytes int
	level    slog.Level
}

// NewRing returns an empty ring.
func NewRing(opts Options) *Ring {
	if opts.Capacity <= 0 {
		opts.Capacity = DefaultCapacity
	}

	if opts.MaxBytes <= 0 {
		opts.MaxBytes = DefaultMaxBytes
	}

	return &Ring{
		buf:      make([]Entry, opts.Capacity),
		capacity: opts.Capacity,
		maxBytes: opts.MaxBytes,
		level:    opts.Level,
	}
}

// Handler returns the slog.Handler that records into the ring.
func (r *Ring) Handler() slog.Handler {
	return &handler{ring: r}
}

// Len returns the number of stored entries.
func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.n
}

// Clear drops every stored entry.
func (r *Ring) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.buf)
	r.start = 0
	r.n = 0
	r.bytes = 0
}

// Entries returns the stored entries, oldest first. The caller owns the slice;
// the Attrs of each entry must not be modified.
func (r *Ring) Entries() []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Entry, r.n)
	for i := range r.n {
		out[i] = r.buf[(r.start+i)%r.capacity]
	}

	return out
}

// append stores e, evicting the oldest entries until both limits hold.
func (r *Ring) append(e Entry) {
	if e.size > r.maxBytes {
		return // unreachable: entries are truncated to maxEntryBytes first.
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for r.n > 0 && (r.n == r.capacity || r.bytes+e.size > r.maxBytes) {
		r.evictOldest()
	}

	r.buf[(r.start+r.n)%r.capacity] = e
	r.n++
	r.bytes += e.size
}

// evictOldest drops the oldest entry. The caller holds the lock.
func (r *Ring) evictOldest() {
	old := r.buf[r.start]
	r.buf[r.start] = Entry{} // release the attrs for the garbage collector
	r.start = (r.start + 1) % r.capacity
	r.n--
	r.bytes -= old.size
}

// handler is the slog.Handler view of a Ring, with pre-applied attrs and groups.
type handler struct {
	ring   *Ring
	attrs  []slog.Attr
	groups []string
}

// Enabled implements [slog.Handler].
func (h *handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.ring.level
}

// Handle implements [slog.Handler].
func (h *handler) Handle(_ context.Context, rec slog.Record) error {
	attrs := make([]slog.Attr, 0, len(h.attrs)+rec.NumAttrs())
	attrs = append(attrs, h.attrs...) // already qualified by WithAttrs

	rec.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, qualify(h.groups, a))
		return true
	})

	msg, attrs, truncated := fit(rec.Message, attrs)

	h.ring.append(Entry{
		Time:      rec.Time,
		Level:     rec.Level,
		Message:   msg,
		Attrs:     attrs,
		Truncated: truncated,
		size:      entrySize(msg, attrs),
	})

	return nil
}

// WithAttrs implements [slog.Handler]. Attrs are qualified with the groups that
// are active now, so attrs added before a WithGroup keep their own keys.
func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	qualified := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		qualified[i] = qualify(h.groups, a)
	}

	c := *h
	c.attrs = slices.Concat(h.attrs, qualified)

	return &c
}

// WithGroup implements [slog.Handler].
func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	c := *h
	c.groups = slices.Concat(h.groups, []string{name})

	return &c
}

// qualify resolves a's value and prefixes its key with groups.
func qualify(groups []string, a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()

	if len(groups) == 0 || a.Key == "" {
		return a
	}

	a.Key = strings.Join(groups, ".") + "." + a.Key

	return a
}

// fit trims msg and attrs to maxEntryBytes. The message is kept first; attrs are
// dropped from the end once the budget is spent.
func fit(msg string, attrs []slog.Attr) (string, []slog.Attr, bool) {
	truncated := false

	if len(msg) > maxEntryBytes {
		msg, truncated = cut(msg, maxEntryBytes)
	}

	budget := maxEntryBytes - len(msg)

	var kept []slog.Attr

	for _, a := range attrs {
		size := attrSize(a)
		if size > budget {
			truncated = true

			break
		}

		budget -= size
		kept = append(kept, a)
	}

	return msg, kept, truncated
}

// entrySize is the memory an entry is accounted for.
func entrySize(msg string, attrs []slog.Attr) int {
	size := len(msg)
	for _, a := range attrs {
		size += attrSize(a)
	}

	return size
}

// attrSize is the memory an attribute is accounted for.
func attrSize(a slog.Attr) int {
	return len(a.Key) + len(a.Value.String())
}

// cut shortens s to at most n bytes without splitting a UTF-8 sequence.
func cut(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}

	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}

	return s[:n], true
}
