package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pocikode/ratatoskr/internal/app"
)

// clearEnv unsets the RATATOSKR_* variables a developer may have exported.
func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{"listen", "ui-listen", "data-dir", "no-open", "auth-token", "debug", "allow-guests"} {
		key := "RATATOSKR_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

		old, existed := os.LookupEnv(key)

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}

		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, old)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	if err := run(t.Context(), []string{"version"}, &stdout, &stderr); err != nil {
		t.Fatalf("run(version): %v", err)
	}

	out := stdout.String()
	if !strings.HasPrefix(out, "ratatoskr ") {
		t.Errorf("version output = %q, want it to start with %q", out, "ratatoskr ")
	}

	if !strings.Contains(out, version) {
		t.Errorf("version output = %q, want it to contain %q", out, version)
	}

	if !strings.Contains(out, "(commit ") {
		t.Errorf("version output = %q, want it to report the commit", out)
	}

	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", stderr.String())
	}
}

func TestRunSubcommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
		wantNot bool
	}{
		{name: "ca export", args: []string{"ca", "export"}, wantNot: true},
		{name: "ca regenerate", args: []string{"ca", "regenerate"}, wantNot: true},
		{name: "project list", args: []string{"project", "list"}, wantNot: true},
		{name: "project backup", args: []string{"project", "backup"}, wantNot: true},
		{name: "project restore", args: []string{"project", "restore"}, wantNot: true},
		{name: "ca without a command", args: []string{"ca"}, wantErr: "expected one of export|regenerate"},
		{name: "ca with an unknown command", args: []string{"ca", "bogus"}, wantErr: "unknown command"},
		{name: "unknown command", args: []string{"bogus"}, wantErr: "unknown command"},
		{name: "version with arguments", args: []string{"version", "extra"}, wantErr: "unexpected arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := run(t.Context(), tt.args, &bytes.Buffer{}, &bytes.Buffer{})

			if tt.wantNot {
				if !errors.Is(err, app.ErrNotImplemented) {
					t.Fatalf("run(%q) error = %v, want ErrNotImplemented", tt.args, err)
				}

				if !strings.Contains(err.Error(), "not implemented yet") {
					t.Errorf("error = %q, want it to say %q", err, "not implemented yet")
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	if err := run(t.Context(), []string{"--help"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("run(--help): %v", err)
	}

	usage := stdout.String()
	for _, want := range []string{"usage: ratatoskr", "--listen", "--data-dir", "ratatoskr version"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not contain %q:\n%s", want, usage)
		}
	}
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), []string{"--nope"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("run(--nope) error = %v, want an unknown flag error", err)
	}
}

func TestRunRejectsUnimplementedAuthToken(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	err := run(t.Context(),
		[]string{"--auth-token", "s3cret", "--data-dir", dir, "--listen", "127.0.0.1:0"},
		&bytes.Buffer{}, &bytes.Buffer{})
	if !errors.Is(err, app.ErrNotImplemented) {
		t.Fatalf("run(--auth-token) error = %v, want ErrNotImplemented", err)
	}
}

func TestRunServesAndStopsOnContextCancel(t *testing.T) {
	clearEnv(t)

	var stderr bytes.Buffer

	dir := filepath.Join(t.TempDir(), "data")

	// The context is cancelled up front: Run still has to bind, wire and shut
	// down cleanly, which is the lifecycle this test covers. Serving is covered by
	// the internal/app tests.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := run(ctx, []string{"--listen", "127.0.0.1:0", "--data-dir", dir}, &bytes.Buffer{}, &stderr)
	if err != nil {
		t.Fatalf("run: %v (stderr: %s)", err, stderr.String())
	}

	if !strings.Contains(stderr.String(), "ratatoskr listening") {
		t.Errorf("stderr = %q, want the listening record", stderr.String())
	}

	if !strings.Contains(stderr.String(), "ratatoskr stopped") {
		t.Errorf("stderr = %q, want the stopped record", stderr.String())
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat data dir: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("data dir mode = %o, want 700", got)
	}
}

func TestNotifyContextOnSIGTERM(t *testing.T) {
	// Not parallel: the signal is delivered to the whole process.
	ctx, stop := notifyContext()
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled by SIGTERM")
	}
}

func TestCommit(t *testing.T) {
	t.Parallel()

	got := commit()
	if got == "" {
		t.Fatal("commit() = \"\", want the build revision or \"unknown\"")
	}

	if strings.ContainsAny(got, " \t") {
		t.Errorf("commit() = %q, want a single token", got)
	}
}
