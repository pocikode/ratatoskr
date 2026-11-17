// Package config resolves the configuration of one Ratatoskr instance from
// command-line flags, RATATOSKR_* environment variables and the config.toml file
// in the data dir.
//
// Precedence, highest first: flags, environment, config file, defaults
// (docs/01-feature-spec.md §I). Every flag has an environment variable with the
// same name, upper-cased and prefixed with RATATOSKR_ (--data-dir is
// RATATOSKR_DATA_DIR), and a config file key with the same name. Config keys also
// accept an underscore in place of a dash, so data_dir and data-dir are the same
// key.
//
// The data dir is the one exception: config.toml lives inside it, so it has to be
// resolved from --data-dir, RATATOSKR_DATA_DIR or the default before any file is
// read. Setting data_dir inside config.toml is an error rather than a silent
// relocation.
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/peterbourgon/ff/v4"
	"github.com/peterbourgon/ff/v4/ffhelp"
	"github.com/peterbourgon/ff/v4/fftoml"
	"github.com/peterbourgon/ff/v4/ffval"
)

const (
	// DefaultListen is the default address of the shared proxy and UI/API listener.
	DefaultListen = "127.0.0.1:8080"

	// ConfigFileName is the name of the config file looked up in the data dir.
	ConfigFileName = "config.toml"

	// envPrefix is the prefix of the environment variables that mirror the flags.
	envPrefix = "RATATOSKR"

	// redacted replaces secrets in log output.
	redacted = "[redacted]"
)

// errEmptyDataDir is returned when --data-dir resolves to an empty path.
var errEmptyDataDir = errors.New("config: data_dir must not be empty")

// Config is the validated configuration of one Ratatoskr instance.
type Config struct {
	// Listen is the address of the shared proxy and UI/API listener.
	Listen string
	// UIListen optionally binds the UI/API to its own address instead of sharing
	// the proxy listener.
	UIListen string
	// DataDir holds config.toml, the CA, the instance database and the projects.
	DataDir string
	// NoOpen keeps the UI from opening a browser window on start.
	NoOpen bool
	// AuthToken protects the UI/API with a bearer token. Guests may still use a
	// local instance when AllowGuests is set.
	AuthToken string
	// Debug turns on debug logging and /debug/pprof on the UI/API server.
	Debug bool
	// AllowGuests lets localhost clients use the UI/API without a token.
	AllowGuests bool
}

// Load resolves the configuration from args, which excludes the program name.
//
// A missing config.toml is not an error: flags, environment and defaults are
// enough to run. The error wraps [ff.ErrHelp] when args ask for help, so callers
// can print [Usage].
func Load(args []string) (*Config, error) {
	// config.toml lives in the data dir, so the data dir has to be known before
	// any file can be read: flags and environment only, no config file yet.
	dir, err := resolveDataDir(args)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	var sawDataDir bool

	fs, err := newFlagSet(cfg, dir)
	if err != nil {
		return nil, err
	}

	if err := ff.Parse(fs, args, parseOptions(dir, &sawDataDir)...); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if extra := fs.GetArgs(); len(extra) > 0 {
		return nil, fmt.Errorf("config: unexpected argument %q", extra[0])
	}

	if sawDataDir {
		return nil, fmt.Errorf("config: data_dir cannot be set in %s; use --data-dir or %s_DATA_DIR",
			ConfigFileName, envPrefix)
	}

	if cfg.DataDir == "" {
		return nil, errEmptyDataDir
	}

	cfg.DataDir = filepath.Clean(cfg.DataDir)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// resolveDataDir resolves the data dir from flags, environment and the default,
// without reading any config file.
func resolveDataDir(args []string) (string, error) {
	dir, err := defaultDataDir()
	if err != nil {
		return "", err
	}

	cfg := &Config{}

	fs, err := newFlagSet(cfg, dir)
	if err != nil {
		return "", err
	}

	if err := ff.Parse(fs, args, ff.WithEnvVarPrefix(envPrefix)); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}

	if cfg.DataDir == "" {
		return "", errEmptyDataDir
	}

	return filepath.Clean(cfg.DataDir), nil
}

// Validate reports whether the configuration is usable as-is.
func (c *Config) Validate() error {
	if c.DataDir == "" {
		return errEmptyDataDir
	}

	if err := validateAddr("listen", c.Listen); err != nil {
		return err
	}

	if c.UIListen != "" {
		if err := validateAddr("ui_listen", c.UIListen); err != nil {
			return err
		}

		if c.UIListen == c.Listen {
			return errors.New("config: ui_listen must differ from listen")
		}
	}

	return nil
}

// LogValue implements [slog.LogValuer]. It redacts the auth token so a config
// struct can be logged without leaking a secret.
func (c Config) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("listen", c.Listen),
		slog.String("data_dir", c.DataDir),
		slog.Bool("no_open", c.NoOpen),
		slog.Bool("debug", c.Debug),
		slog.Bool("allow_guests", c.AllowGuests),
	}

	if c.UIListen != "" {
		attrs = append(attrs, slog.String("ui_listen", c.UIListen))
	}

	if c.AuthToken != "" {
		attrs = append(attrs, slog.String("auth_token", redacted))
	}

	return slog.GroupValue(attrs...)
}

// Usage writes the help text for the root command.
func Usage(w io.Writer) error {
	dir, err := defaultDataDir()
	if err != nil {
		dir = ""
	}

	fs, err := newFlagSet(&Config{}, dir)
	if err != nil {
		return err
	}

	_, err = ffhelp.Flags(fs,
		"usage: ratatoskr [flags]",
		"       ratatoskr version",
		"       ratatoskr ca export|regenerate",
		"       ratatoskr project list|backup|restore",
	).WriteTo(w)

	if err != nil {
		return fmt.Errorf("config: write usage: %w", err)
	}

	return nil
}

// newFlagSet defines the flags of the root command on cfg. dataDir is the
// --data-dir default: the dir whose config.toml is being read.
func newFlagSet(cfg *Config, dataDir string) (*ff.FlagSet, error) {
	fs := ff.NewFlagSet("ratatoskr")

	flags := []ff.FlagConfig{
		{
			LongName:    "listen",
			Placeholder: "addr",
			Usage:       "address of the shared proxy and UI/API listener (host:port)",
			Value:       ffval.NewValueDefault(&cfg.Listen, DefaultListen),
		},
		{
			LongName:    "ui-listen",
			Placeholder: "addr",
			Usage:       "serve the UI/API on its own address instead of the proxy listener",
			Value:       ffval.NewValueDefault(&cfg.UIListen, ""),
		},
		{
			LongName:    "data-dir",
			Placeholder: "path",
			Usage:       "directory holding config.toml, the CA and the project databases",
			Value:       ffval.NewValueDefault(&cfg.DataDir, dataDir),
		},
		{
			LongName: "no-open",
			Usage:    "do not open a browser window on start",
			Value:    ffval.NewValueDefault(&cfg.NoOpen, false),
		},
		{
			LongName:    "auth-token",
			Placeholder: "token",
			Usage:       "require this bearer token for UI/API access",
			Value:       ffval.NewValueDefault(&cfg.AuthToken, ""),
		},
		{
			LongName: "debug",
			Usage:    "log at debug level and serve /debug/pprof",
			Value:    ffval.NewValueDefault(&cfg.Debug, false),
		},
		{
			LongName: "allow-guests",
			Usage:    "let localhost clients in without a token",
			Value:    ffval.NewValueDefault(&cfg.AllowGuests, false),
		},
	}

	for _, fc := range flags {
		if _, err := fs.AddFlag(fc); err != nil {
			return nil, fmt.Errorf("config: define --%s: %w", fc.LongName, err)
		}
	}

	return fs, nil
}

// parseOptions wires environment variables and the config file into a parse.
// sawDataDir is set when the config file tries to set the data dir.
func parseOptions(dir string, sawDataDir *bool) []ff.Option {
	return []ff.Option{
		ff.WithEnvVarPrefix(envPrefix),
		ff.WithConfigFile(filepath.Join(dir, ConfigFileName)),
		ff.WithConfigAllowMissingFile(),
		ff.WithConfigFileParser(parseTOML(sawDataDir)),
	}
}

// parseTOML returns a parser for a TOML config file. Keys may use underscores
// where the flag uses a dash, so both data_dir and data-dir set --data-dir.
func parseTOML(sawDataDir *bool) ff.ConfigFileParseFunc {
	return func(r io.Reader, set func(name, value string) error) error {
		return fftoml.Parse(r, func(name, value string) error {
			name = strings.ReplaceAll(name, "_", "-")
			if name == "data-dir" {
				*sawDataDir = true
			}

			return set(name, value)
		})
	}
}

// defaultDataDir is the per-user data dir, <user config dir>/ratatoskr.
func defaultDataDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: locate user config dir: %w", err)
	}

	return filepath.Join(dir, "ratatoskr"), nil
}

// validateAddr checks that addr is a host:port pair with a numeric port.
func validateAddr(name, addr string) error {
	if addr == "" {
		return fmt.Errorf("config: %s must not be empty", name)
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("config: %s: %w", name, err)
	}

	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return fmt.Errorf("config: %s: port %q must be a number between 0 and 65535", name, port)
	}

	return nil
}
