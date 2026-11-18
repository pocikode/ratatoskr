// Command ratatoskr is the Ratatoskr web security proxy.
//
// It resolves its configuration (internal/config), builds the app (internal/app)
// and serves until SIGINT or SIGTERM. The CLI surface is
// docs/01-feature-spec.md §I.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/peterbourgon/ff/v4"

	"github.com/pocikode/ratatoskr/internal/app"
	"github.com/pocikode/ratatoskr/internal/config"
	"github.com/pocikode/ratatoskr/internal/logging"
)

// version is stamped at build time via -ldflags "-X main.version=$(VERSION)".
var version = "dev"

// subcommands lists the commands of each subcommand group. The implementations
// land with the CA (T-008) and the project registry (T-006); until then they
// report app.ErrNotImplemented.
var subcommands = map[string][]string{
	"ca":      {"export", "regenerate"},
	"project": {"list", "backup", "restore"},
}

func main() {
	ctx, stop := notifyContext()
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "ratatoskr: %v\n", err)
		os.Exit(1)
	}
}

// notifyContext returns a context that SIGINT or SIGTERM cancels.
func notifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

// run is main without the process-level bits: it dispatches a subcommand, or
// resolves the configuration and serves until ctx is cancelled.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if name, rest := splitSubcommand(args); name != "" {
		return runSubcommand(name, rest, stdout)
	}

	cfg, err := config.Load(args)
	if err != nil {
		if errors.Is(err, ff.ErrHelp) {
			return config.Usage(stdout)
		}

		return err
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	logger, ring := logging.New(stderr, level)
	// Anything that logs through slog.Default (the stdlib, later packages,
	// app's own fallback) has to land in the ring too, or the Logs page misses it.
	slog.SetDefault(logger)

	logger.Info("ratatoskr starting", "version", version, "commit", commit(), "config", *cfg)

	instance, err := app.New(ctx, app.Options{
		Config: *cfg, Logger: logger, Logs: ring, Version: version, Commit: commit(),
	})
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := instance.Close(); closeErr != nil {
			logger.Error("close instance", "error", closeErr)
		}
	}()

	return instance.Run(ctx)
}

// splitSubcommand returns the leading subcommand and the arguments after it.
// Flags never precede a subcommand, so anything starting with a dash is the root
// command.
func splitSubcommand(args []string) (name string, rest []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", args
	}

	return args[0], args[1:]
}

// runSubcommand executes one of the CLI subcommands.
func runSubcommand(name string, args []string, stdout io.Writer) error {
	switch name {
	case "version":
		if len(args) > 0 {
			return fmt.Errorf("version: unexpected arguments %q", args)
		}

		_, err := fmt.Fprintf(stdout, "ratatoskr %s (commit %s)\n", version, commit())

		return err
	case "ca", "project":
		want := strings.Join(subcommands[name], "|")

		if len(args) == 0 {
			return fmt.Errorf("%s: missing command, expected one of %s", name, want)
		}

		if !slices.Contains(subcommands[name], args[0]) {
			return fmt.Errorf("%s: unknown command %q, expected one of %s", name, args[0], want)
		}

		return fmt.Errorf("%s %s: %w", name, args[0], app.ErrNotImplemented)
	default:
		return fmt.Errorf("unknown command %q, run \"ratatoskr --help\"", name)
	}
}

// commit reports the VCS revision the binary was built from.
func commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	var revision, modified string

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}

	if revision == "" {
		return "unknown"
	}

	if len(revision) > 12 {
		revision = revision[:12]
	}

	if modified == "true" {
		revision += "-dirty"
	}

	return revision
}
