// Package config resolves the configuration of one Ratatoskr instance from
// command-line flags, RATATOSKR_* environment variables and the config.toml file
// in the data dir.
//
// Precedence, highest first: flags, environment, config file, defaults
// (docs/01-feature-spec.md §I). Every flag has an environment variable with the
// same name, upper-cased and prefixed with RATATOSKR_ (--data-dir is
// RATATOSKR_DATA_DIR), and a config file key with the same name. Config keys also
// accept an underscore in place of a dash, so data_dir and data-dir are the same
// key. The repeatable --listen takes a comma-separated list from the environment.
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
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
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
	// Listen holds the addresses of the shared proxy and UI/API listeners. One
	// instance may front several proxy ports, so --listen is repeatable
	// (docs/01-feature-spec.md §I). It always holds at least one address.
	Listen []string
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

	// The file is read through an os.Root, never through a bare path
	// (docs/05-go-conventions.md §1). A data dir that does not exist yet simply
	// has no config file: app.New creates it.
	root, err := openConfigRoot(dir)
	if err != nil {
		return nil, err
	}

	if root != nil {
		defer func() { _ = root.Close() }()
	}

	cfg := &Config{}

	var sawDataDir bool

	fs, err := newFlagSet(cfg, dir)
	if err != nil {
		return nil, err
	}

	if err := ff.Parse(fs, args, parseOptions(root, &sawDataDir)...); err != nil {
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
//
// The default is only computed when neither the flag nor the environment names a
// dir: looking up the user config dir fails without $HOME, and a container or a
// systemd unit that passes --data-dir must start anyway.
func resolveDataDir(args []string) (string, error) {
	cfg := &Config{}

	fs, err := newFlagSet(cfg, "")
	if err != nil {
		return "", err
	}

	if err := ff.Parse(fs, args, ff.WithEnvVarPrefix(envPrefix)); err != nil {
		return "", fmt.Errorf("config: %w", err)
	}

	if !isSet(fs, "data-dir") {
		return defaultDataDir()
	}

	if cfg.DataDir == "" {
		return "", errEmptyDataDir
	}

	return filepath.Clean(cfg.DataDir), nil
}

// isSet reports whether name was provided on the command line or in the
// environment by the parse that just ran.
func isSet(fs *ff.FlagSet, name string) bool {
	flag, ok := fs.GetFlag(name)

	return ok && flag.IsSet()
}

// Validate reports whether the configuration is usable as-is.
func (c *Config) Validate() error {
	if c.DataDir == "" {
		return errEmptyDataDir
	}

	if len(c.Listen) == 0 {
		return errors.New("config: listen must not be empty")
	}

	for _, addr := range c.Listen {
		if err := validateAddr("listen", addr); err != nil {
			return err
		}
	}

	if c.UIListen != "" {
		if err := validateAddr("ui_listen", c.UIListen); err != nil {
			return err
		}

		if slices.Contains(c.Listen, c.UIListen) {
			return errors.New("config: ui_listen must differ from listen")
		}
	}

	// The token is the login cookie's value, so it must be cookie-safe (docs/03 §8). Never echo it.
	if strings.ContainsFunc(c.AuthToken, func(r rune) bool {
		return r < 0x21 || r > 0x7E || strings.ContainsRune(`",;\`, r)
	}) {
		return errors.New(`config: auth_token must be printable ASCII without space, '"', ',', ';' or '\'`)
	}

	return nil
}

// LogValue implements [slog.LogValuer]. It redacts the auth token so a config
// struct can be logged without leaking a secret.
func (c Config) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("listen", strings.Join(c.Listen, ", ")),
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
// --data-dir default shown in the help text; Load passes the already resolved
// dir and resolveDataDir passes "", so an explicit dir needs no default lookup.
func newFlagSet(cfg *Config, dataDir string) (*ff.FlagSet, error) {
	if len(cfg.Listen) == 0 {
		cfg.Listen = []string{DefaultListen}
	}

	listen := &listenValue{addrs: &cfg.Listen, defaults: slices.Clone(cfg.Listen)}

	fs := ff.NewFlagSet("ratatoskr")

	flags := []ff.FlagConfig{
		{
			LongName:    "listen",
			Placeholder: "addr",
			Usage:       "address of a shared proxy and UI/API listener; repeat the flag (or comma-separate it in the environment)",
			Value:       listen,
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
			Usage: "let clients whose TCP peer is a loopback address in without a token; " +
				"never combine with a same-host reverse proxy, which makes every client a loopback peer",
			Value: ffval.NewValueDefault(&cfg.AllowGuests, false),
		},
	}

	for _, fc := range flags {
		if _, err := fs.AddFlag(fc); err != nil {
			return nil, fmt.Errorf("config: define --%s: %w", fc.LongName, err)
		}
	}

	return fs, nil
}

// listenValue is the value of the repeatable --listen flag. The first value
// replaces the default instead of appending to it, and one value may hold a
// comma-separated list, which is how a single environment variable names several
// addresses (docs/01-feature-spec.md §I).
type listenValue struct {
	addrs    *[]string
	defaults []string
	set      bool
}

// String implements [flag.Value]. ff captures it as the help text's default, so
// it has to report the addresses the flag holds right now.
func (v *listenValue) String() string {
	return strings.Join(*v.addrs, ", ")
}

// Set implements [flag.Value].
func (v *listenValue) Set(s string) error {
	addrs := make([]string, 0, 1)

	for part := range strings.SplitSeq(s, ",") {
		addr := strings.TrimSpace(part)
		if err := validateAddr("listen", addr); err != nil {
			return err
		}

		addrs = append(addrs, addr)
	}

	if !v.set {
		*v.addrs = addrs
		v.set = true

		return nil
	}

	*v.addrs = append(*v.addrs, addrs...)

	return nil
}

// Reset implements [ff.Resetter]: the flag goes back to its defaults. Without
// it, ff would Set the default string, which appends to the addresses instead of
// replacing them.
func (v *listenValue) Reset() error {
	*v.addrs = slices.Clone(v.defaults)
	v.set = false

	return nil
}

// parseOptions wires environment variables and the config file into a parse. A
// nil root means the data dir does not exist yet, so there is no file to read.
// sawDataDir is set when the config file tries to set the data dir.
func parseOptions(root *os.Root, sawDataDir *bool) []ff.Option {
	opts := []ff.Option{ff.WithEnvVarPrefix(envPrefix)}

	if root == nil {
		return opts
	}

	return append(opts,
		ff.WithConfigFile(ConfigFileName),
		ff.WithConfigAllowMissingFile(),
		ff.WithFilesystem(root.FS()),
		ff.WithConfigFileParser(parseTOML(sawDataDir)),
	)
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

// openConfigRoot opens the data dir so config.toml is read through an [os.Root]
// (docs/05-go-conventions.md §1). It returns a nil root when the dir does not
// exist yet: the first start has no config file and uses the defaults.
func openConfigRoot(dir string) (*os.Root, error) {
	root, err := os.OpenRoot(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("config: open data dir %q: %w", dir, err)
	}

	return root, nil
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
