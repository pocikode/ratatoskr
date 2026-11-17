package logging

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewFansOutToWriterAndRing(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	logger, ring := New(&out, slog.LevelInfo)
	logger.Info("hello", "component", "test")
	logger.Debug("dropped at info level")

	if got := ring.Len(); got != 1 {
		t.Fatalf("ring.Len() = %d, want 1", got)
	}

	if got := out.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, "hello") {
		t.Fatalf("writer got %q, want exactly the info record", got)
	}

	entries := ring.Entries()
	if entries[0].Message != "hello" {
		t.Errorf("message = %q, want %q", entries[0].Message, "hello")
	}

	if entries[0].Level != slog.LevelInfo {
		t.Errorf("level = %v, want %v", entries[0].Level, slog.LevelInfo)
	}

	if len(entries[0].Attrs) != 1 || entries[0].Attrs[0].Key != "component" {
		t.Errorf("attrs = %v, want component", entries[0].Attrs)
	}
}

func TestRingKeepsLastEntries(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 3, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	for _, msg := range []string{"one", "two", "three", "four", "five"} {
		logger.Info(msg)
	}

	entries := ring.Entries()
	if got := ring.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}

	want := []string{"three", "four", "five"}
	for i, e := range entries {
		if e.Message != want[i] {
			t.Errorf("entry %d = %q, want %q", i, e.Message, want[i])
		}
	}
}

func TestRingByteBudgetEvictsOldest(t *testing.T) {
	t.Parallel()

	// Each record is "msg" plus a 40-byte value: room for three entries.
	ring := NewRing(Options{Capacity: 100, MaxBytes: 140, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	for range 5 {
		logger.Info("msg", "k", strings.Repeat("v", 40))
	}

	if ring.bytes > ring.maxBytes {
		t.Fatalf("ring uses %d bytes, budget is %d", ring.bytes, ring.maxBytes)
	}

	entries := ring.Entries()
	if len(entries) != 3 {
		t.Fatalf("kept %d entries, want 3", len(entries))
	}
}

func TestRingTruncatesOversizedEntry(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	long := strings.Repeat("é", maxEntryBytes) // two bytes per rune
	logger.Info(long, "k", strings.Repeat("v", maxEntryBytes))

	entries := ring.Entries()
	if len(entries) != 1 {
		t.Fatalf("kept %d entries, want 1", len(entries))
	}

	e := entries[0]
	if !e.Truncated {
		t.Error("Truncated = false, want true")
	}

	if len(e.Message) > maxEntryBytes {
		t.Errorf("message is %d bytes, want at most %d", len(e.Message), maxEntryBytes)
	}

	if !strings.HasPrefix(long, e.Message) {
		t.Error("truncated message is not a prefix of the original")
	}

	if len(e.Attrs) != 0 {
		t.Errorf("attrs = %v, want none (the message used the whole budget)", e.Attrs)
	}

	if e.size > maxEntryBytes {
		t.Errorf("size = %d, want at most %d", e.size, maxEntryBytes)
	}
}

func TestRingAttrsAndGroups(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler()).With("component", "proxy").WithGroup("conn").WithGroup("tls")

	logger.Info("handshake", "sni", "example.com")

	// Attrs added after a group are qualified, like the stdlib handlers do.
	slog.New(ring.Handler()).WithGroup("conn").With("host", "example.com").Info("dial")

	entries := ring.Entries()
	if len(entries) != 2 {
		t.Fatalf("kept %d entries, want 2", len(entries))
	}

	got := map[string]string{}
	for _, a := range entries[0].Attrs {
		got[a.Key] = a.Value.String()
	}

	want := map[string]string{"component": "proxy", "conn.tls.sni": "example.com"}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("attr %q = %q, want %q (all: %v)", key, got[key], value, got)
		}
	}

	if attrs := entries[1].Attrs; len(attrs) != 1 || attrs[0].Key != "conn.host" {
		t.Errorf("attrs = %v, want conn.host", attrs)
	}
}

// attrsOf turns the attrs of e into a key→value map.
func attrsOf(e Entry) map[string]string {
	attrs := make(map[string]string, len(e.Attrs))
	for _, a := range e.Attrs {
		attrs[a.Key] = a.Value.String()
	}

	return attrs
}

func TestRingFlattensGroups(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 8, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	logger.Info("request",
		slog.Group("req",
			slog.String("method", "GET"),
			slog.Group("hdr", slog.String("host", "example.com")),
		),
		// A group without a key is inlined without a prefix, an empty group
		// contributes nothing and a zero attr is dropped.
		slog.Group("", slog.String("inline", "yes")),
		slog.Group("empty"),
		slog.Attr{},
		slog.String("plain", "value"),
	)

	slog.New(ring.Handler()).WithGroup("conn").
		Info("dial", slog.Group("tls", slog.String("sni", "example.com")), "host", "example.com")

	entries := ring.Entries()
	if len(entries) != 2 {
		t.Fatalf("kept %d entries, want 2", len(entries))
	}

	want := map[string]map[string]string{
		"request": {
			"req.method":   "GET",
			"req.hdr.host": "example.com",
			"inline":       "yes",
			"plain":        "value",
		},
		"dial": {
			"conn.tls.sni": "example.com",
			"conn.host":    "example.com",
		},
	}

	for _, e := range entries {
		got := attrsOf(e)

		if len(got) != len(want[e.Message]) {
			t.Errorf("%s: attrs = %v, want %v", e.Message, got, want[e.Message])
		}

		for key, value := range want[e.Message] {
			if got[key] != value {
				t.Errorf("%s: attr %q = %q, want %q (all: %v)", e.Message, key, got[key], value, got)
			}
		}
	}
}

// groupValuer is a slog.LogValuer that expands to a group, like config.Config.
type groupValuer struct{ secret string }

func (v groupValuer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("data_dir", "/tmp/x"),
		slog.String("auth_token", v.secret),
	)
}

func TestRingFlattensLogValuer(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	logger.Info("starting", "config", groupValuer{secret: "[redacted]"})

	got := attrsOf(ring.Entries()[0])
	want := map[string]string{"config.data_dir": "/tmp/x", "config.auth_token": "[redacted]"}

	if len(got) != len(want) {
		t.Fatalf("attrs = %v, want %v", got, want)
	}

	for key, value := range want {
		if got[key] != value {
			t.Errorf("attr %q = %q, want %q (all: %v)", key, got[key], value, got)
		}
	}
}

// TestRingSnapshotsValues checks that the ring renders a value when the record
// is handled instead of keeping a reference the caller may still be changing,
// which would also make the byte budget a lie.
func TestRingSnapshotsValues(t *testing.T) {
	t.Parallel()

	type record struct {
		Name string
		Body []byte
	}

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	rec := &record{Name: "before", Body: []byte("before")}

	// The string the value has at Handle time, rendered the way the stdlib
	// renders a KindAny value.
	want := fmt.Sprint(rec)

	logger.Info("snapshot", "rec", rec, slog.Group("g", "rec", rec))

	rec.Name = "after"
	rec.Body[0] = 'X'

	e := ring.Entries()[0]
	got := attrsOf(e)

	if got["rec"] != want {
		t.Errorf("attr rec = %q, want the value at log time %q", got["rec"], want)
	}

	if got["g.rec"] != want {
		t.Errorf("attr g.rec = %q, want the value at log time %q", got["g.rec"], want)
	}

	for _, a := range e.Attrs {
		if a.Value.Kind() == slog.KindAny {
			t.Errorf("attr %q kept a KindAny value, want a snapshot", a.Key)
		}
	}

	// The byte budget counts the snapshot, not the object behind it.
	if e.size > maxEntryBytes {
		t.Errorf("size = %d, want at most %d", e.size, maxEntryBytes)
	}
}

func TestRingRespectsLevel(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelWarn})
	logger := slog.New(ring.Handler())

	logger.Info("info")
	logger.Warn("warn")

	entries := ring.Entries()
	if len(entries) != 1 || entries[0].Message != "warn" {
		t.Fatalf("entries = %v, want only the warn record", entries)
	}
}

func TestRingClear(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 4, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())
	logger.Info("one")

	ring.Clear()

	if got := ring.Len(); got != 0 {
		t.Fatalf("Len() = %d after Clear, want 0", got)
	}

	if got := ring.Entries(); len(got) != 0 {
		t.Fatalf("Entries() = %v after Clear, want none", got)
	}

	if ring.bytes != 0 {
		t.Fatalf("bytes = %d after Clear, want 0", ring.bytes)
	}
}

func TestRingConcurrentWrites(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 64, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for i := range 50 {
				logger.Info("concurrent", "i", i)
				_ = ring.Entries()
			}
		})
	}

	wg.Wait()

	if got := ring.Len(); got != 64 {
		t.Fatalf("Len() = %d, want the full capacity 64", got)
	}

	if ring.bytes > ring.maxBytes {
		t.Fatalf("bytes = %d, over budget %d", ring.bytes, ring.maxBytes)
	}
}

func TestCut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		n    int
		want string
		cut  bool
	}{
		{name: "short", in: "abc", n: 10, want: "abc"},
		{name: "ascii cut", in: "abcdef", n: 3, want: "abc", cut: true},
		{name: "rune boundary", in: "aé", n: 2, want: "a", cut: true},
		{name: "exact", in: "abc", n: 3, want: "abc"},
		{name: "zero", in: "abc", n: 0, want: "", cut: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, cut := cut(tt.in, tt.n)
			if got != tt.want || cut != tt.cut {
				t.Errorf("cut(%q, %d) = %q, %v; want %q, %v", tt.in, tt.n, got, cut, tt.want, tt.cut)
			}
		})
	}
}

func TestEntryTimeIsRecordTime(t *testing.T) {
	t.Parallel()

	ring := NewRing(Options{Capacity: 2, Level: slog.LevelDebug})
	logger := slog.New(ring.Handler())

	logger.Info("now")

	entries := ring.Entries()
	if entries[0].Time.IsZero() {
		t.Fatal("Time is zero, want the record time")
	}

	if since := time.Since(entries[0].Time); since < 0 || since > time.Minute {
		t.Fatalf("Time = %v, want close to now", entries[0].Time)
	}
}
