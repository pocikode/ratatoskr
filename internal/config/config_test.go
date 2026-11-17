package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/peterbourgon/ff/v4"
)

// envKeys are the flag names whose RATATOSKR_* variables a test may inherit.
var envKeys = []string{
	"listen", "ui-listen", "data-dir", "no-open", "auth-token", "debug", "allow-guests",
}

// clearEnv unsets every RATATOSKR_* variable for the duration of the test and
// restores it afterwards. t.Setenv cannot express "unset", and an inherited
// variable would silently change the defaults these tests assert on.
func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range envKeys {
		key := envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

		old, existed := os.LookupEnv(key)

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}

		t.Cleanup(func() {
			if !existed {
				return
			}

			_ = os.Setenv(key, old)
		})
	}
}

// expand replaces the $DIR placeholder with dir.
func expand(s, dir string) string {
	return strings.ReplaceAll(s, "$DIR", dir)
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		// files maps a path relative to $DIR to its content.
		files map[string]string
		// dirs are extra directories created below $DIR before Load runs.
		dirs    []string
		want    Config
		wantDir string // defaults to "$DIR"
		wantErr string
	}{
		{
			name:    "defaults",
			wantDir: "default",
			want:    Config{Listen: []string{DefaultListen}},
		},
		{
			name: "flags",
			args: []string{
				"--listen", "127.0.0.1:9000",
				"--ui-listen", "127.0.0.1:9001",
				"--data-dir", "$DIR",
				"--no-open",
				"--auth-token", "s3cret",
				"--debug",
				"--allow-guests",
			},
			want: Config{
				Listen:      []string{"127.0.0.1:9000"},
				UIListen:    "127.0.0.1:9001",
				NoOpen:      true,
				AuthToken:   "s3cret",
				Debug:       true,
				AllowGuests: true,
			},
		},
		{
			name: "repeated listen flag",
			args: []string{
				"--listen", "127.0.0.1:9000",
				"--listen", "127.0.0.1:9001",
				"--data-dir", "$DIR",
			},
			want: Config{Listen: []string{"127.0.0.1:9000", "127.0.0.1:9001"}},
		},
		{
			name:    "repeated listen flag replaces the default",
			args:    []string{"--listen", "127.0.0.1:9000", "--data-dir", "$DIR"},
			want:    Config{Listen: []string{"127.0.0.1:9000"}},
			wantDir: "$DIR",
		},
		{
			name:    "listen flag with a bad address among several",
			args:    []string{"--listen", "127.0.0.1:9000", "--listen", "nope", "--data-dir", "$DIR"},
			wantErr: "listen",
		},
		{
			name: "environment",
			env: map[string]string{
				"RATATOSKR_LISTEN":       "127.0.0.1:9000",
				"RATATOSKR_UI_LISTEN":    "127.0.0.1:9001",
				"RATATOSKR_DATA_DIR":     "$DIR",
				"RATATOSKR_NO_OPEN":      "true",
				"RATATOSKR_AUTH_TOKEN":   "s3cret",
				"RATATOSKR_DEBUG":        "true",
				"RATATOSKR_ALLOW_GUESTS": "true",
			},
			want: Config{
				Listen:      []string{"127.0.0.1:9000"},
				UIListen:    "127.0.0.1:9001",
				NoOpen:      true,
				AuthToken:   "s3cret",
				Debug:       true,
				AllowGuests: true,
			},
		},
		{
			name: "environment listen is a comma-separated list",
			env: map[string]string{
				"RATATOSKR_LISTEN":   "127.0.0.1:9000, 127.0.0.1:9001",
				"RATATOSKR_DATA_DIR": "$DIR",
			},
			want: Config{Listen: []string{"127.0.0.1:9000", "127.0.0.1:9001"}},
		},
		{
			name: "config file",
			args: []string{"--data-dir", "$DIR"},
			files: map[string]string{ConfigFileName: `
listen = "127.0.0.1:9000"
ui-listen = "127.0.0.1:9001"
no-open = true
auth-token = "s3cret"
debug = true
allow-guests = true
`},
			want: Config{
				Listen:      []string{"127.0.0.1:9000"},
				UIListen:    "127.0.0.1:9001",
				NoOpen:      true,
				AuthToken:   "s3cret",
				Debug:       true,
				AllowGuests: true,
			},
		},
		{
			name:  "config file listen is an array",
			args:  []string{"--data-dir", "$DIR"},
			files: map[string]string{ConfigFileName: "listen = [\"127.0.0.1:9000\", \"127.0.0.1:9001\"]\n"},
			want:  Config{Listen: []string{"127.0.0.1:9000", "127.0.0.1:9001"}},
		},
		{
			name:  "config file accepts underscores",
			args:  []string{"--data-dir", "$DIR"},
			files: map[string]string{ConfigFileName: "ui_listen = \"127.0.0.1:9001\"\nno_open = true\n"},
			want:  Config{Listen: []string{DefaultListen}, UIListen: "127.0.0.1:9001", NoOpen: true},
		},
		{
			name:  "flag beats environment",
			args:  []string{"--listen", "127.0.0.1:9002", "--data-dir", "$DIR"},
			env:   map[string]string{"RATATOSKR_LISTEN": "127.0.0.1:9000"},
			files: map[string]string{ConfigFileName: `listen = "127.0.0.1:9001"` + "\n"},
			want:  Config{Listen: []string{"127.0.0.1:9002"}},
		},
		{
			name:  "environment beats config file",
			args:  []string{"--data-dir", "$DIR"},
			env:   map[string]string{"RATATOSKR_LISTEN": "127.0.0.1:9000"},
			files: map[string]string{ConfigFileName: `listen = "127.0.0.1:9001"` + "\n"},
			want:  Config{Listen: []string{"127.0.0.1:9000"}},
		},
		{
			name:  "config file beats defaults",
			args:  []string{"--data-dir", "$DIR"},
			files: map[string]string{ConfigFileName: `listen = "127.0.0.1:9001"` + "\n"},
			want:  Config{Listen: []string{"127.0.0.1:9001"}},
		},
		{
			name: "explicit false flag beats environment",
			args: []string{"--debug=false", "--data-dir", "$DIR"},
			env:  map[string]string{"RATATOSKR_DEBUG": "true"},
			want: Config{Listen: []string{DefaultListen}},
		},
		{
			name: "config file may not relocate the data dir",
			args: []string{"--data-dir", "$DIR"},
			dirs: []string{"other"},
			files: map[string]string{
				ConfigFileName: "data_dir = \"$DIR/other\"\n",
			},
			wantErr: "data_dir cannot be set",
		},
		{
			name:    "data dir flag is cleaned",
			args:    []string{"--data-dir", "$DIR/./"},
			want:    Config{Listen: []string{DefaultListen}},
			wantDir: "$DIR",
		},
		{
			name:    "unknown flag",
			args:    []string{"--nope"},
			wantErr: "unknown flag",
		},
		{
			name:    "unknown config key",
			args:    []string{"--data-dir", "$DIR"},
			files:   map[string]string{ConfigFileName: "nope = true\n"},
			wantErr: "unknown flag",
		},
		{
			name:    "unexpected argument",
			args:    []string{"serve"},
			wantErr: "unexpected argument",
		},
		{
			name:    "listen without a port",
			args:    []string{"--listen", "127.0.0.1", "--data-dir", "$DIR"},
			wantErr: "listen",
		},
		{
			name:    "listen with an out of range port",
			args:    []string{"--listen", "127.0.0.1:65536", "--data-dir", "$DIR"},
			wantErr: "port",
		},
		{
			name:    "ui listen equal to a repeated listen",
			args:    []string{"--listen", "127.0.0.1:9000", "--listen", "127.0.0.1:9001", "--ui-listen", "127.0.0.1:9001", "--data-dir", "$DIR"},
			wantErr: "ui_listen must differ",
		},
		{
			name:    "empty data dir",
			args:    []string{"--data-dir", ""},
			wantErr: "data_dir must not be empty",
		},
		{
			name:    "help",
			args:    []string{"--help"},
			wantErr: ff.ErrHelp.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)

			dir := t.TempDir()
			for _, sub := range tt.dirs {
				if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
					t.Fatalf("mkdir %s: %v", sub, err)
				}
			}

			for name, content := range tt.files {
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(expand(content, dir)), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			args := make([]string, len(tt.args))
			for i, arg := range tt.args {
				args[i] = expand(arg, dir)
			}

			for key, value := range tt.env {
				t.Setenv(key, expand(value, dir))
			}

			got, err := Load(args)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Load(%q) = %+v, want error containing %q", args, got, tt.wantErr)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Load(%q) error = %q, want it to contain %q", args, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("Load(%q): %v", args, err)
			}

			want := tt.want
			if len(want.Listen) == 0 {
				want.Listen = []string{DefaultListen}
			}

			switch tt.wantDir {
			case "":
				want.DataDir = dir
			case "default":
				want.DataDir = got.DataDir

				if !filepath.IsAbs(got.DataDir) || filepath.Base(got.DataDir) != "ratatoskr" {
					t.Errorf("DataDir = %q, want an absolute <user config dir>/ratatoskr", got.DataDir)
				}
			default:
				want.DataDir = expand(tt.wantDir, dir)
			}

			if !reflect.DeepEqual(*got, want) {
				t.Errorf("Load(%q) =\n %+v\nwant\n %+v", args, *got, want)
			}
		})
	}
}

func TestLoadHelp(t *testing.T) {
	clearEnv(t)

	_, err := Load([]string{"-h"})
	if !errors.Is(err, ff.ErrHelp) {
		t.Fatalf("Load(-h) error = %v, want ff.ErrHelp", err)
	}
}

// TestLoadWithoutHome checks that an explicit data dir starts the binary on a
// host without $HOME (containers, systemd units, CI), where looking up the
// default data dir fails.
func TestLoadWithoutHome(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	dir := t.TempDir()

	t.Run("no data dir at all", func(t *testing.T) {
		_, err := Load(nil)
		if err == nil || !strings.Contains(err.Error(), "user config dir") {
			t.Fatalf("Load() error = %v, want the user config dir lookup to fail", err)
		}
	})

	t.Run("help", func(t *testing.T) {
		_, err := Load([]string{"--help"})
		if !errors.Is(err, ff.ErrHelp) {
			t.Fatalf("Load(--help) error = %v, want ff.ErrHelp", err)
		}
	})

	t.Run("flag", func(t *testing.T) {
		cfg, err := Load([]string{"--data-dir", dir})
		if err != nil {
			t.Fatalf("Load with --data-dir and no $HOME: %v", err)
		}

		if cfg.DataDir != dir {
			t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
		}
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("RATATOSKR_DATA_DIR", dir)

		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load with RATATOSKR_DATA_DIR and no $HOME: %v", err)
		}

		if cfg.DataDir != dir {
			t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
		}
	})
}

// TestLoadConfigFileThroughRoot checks that config.toml is read through the data
// dir's os.Root, so a file that points outside the dir is refused.
func TestLoadConfigFileThroughRoot(t *testing.T) {
	clearEnv(t)

	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("debug = true\n"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ConfigFileName)); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	_, err := Load([]string{"--data-dir", dir})
	if err == nil {
		t.Fatal("Load with an escaping config.toml symlink = nil error, want a refusal")
	}
}

func TestListenValueReset(t *testing.T) {
	addrs := []string{DefaultListen}
	v := &listenValue{addrs: &addrs, defaults: []string{DefaultListen}}

	if err := v.Set("127.0.0.1:9000,127.0.0.1:9001"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if want := []string{"127.0.0.1:9000", "127.0.0.1:9001"}; !slices.Equal(addrs, want) {
		t.Fatalf("addrs = %v, want %v", addrs, want)
	}

	if err := v.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if want := []string{DefaultListen}; !slices.Equal(addrs, want) {
		t.Fatalf("addrs after Reset = %v, want %v", addrs, want)
	}

	// The flag replaces the default again after a reset.
	if err := v.Set("127.0.0.1:9002"); err != nil {
		t.Fatalf("Set after Reset: %v", err)
	}

	if want := []string{"127.0.0.1:9002"}; !slices.Equal(addrs, want) {
		t.Fatalf("addrs = %v, want %v", addrs, want)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "valid", cfg: Config{Listen: []string{DefaultListen}, DataDir: "/tmp/x"}},
		{
			name: "several listeners",
			cfg:  Config{Listen: []string{"127.0.0.1:9000", "[::1]:9001"}, DataDir: "/tmp/x"},
		},
		{name: "no listen", cfg: Config{DataDir: "/tmp/x"}, wantErr: "listen must not be empty"},
		{
			name:    "one bad listener",
			cfg:     Config{Listen: []string{DefaultListen, "127.0.0.1"}, DataDir: "/tmp/x"},
			wantErr: "listen",
		},
		{name: "empty data dir", cfg: Config{Listen: []string{DefaultListen}}, wantErr: "data_dir must not be empty"},
		{
			name: "ui listen only",
			cfg:  Config{Listen: []string{DefaultListen}, UIListen: ":9001", DataDir: "/tmp/x"},
		},
		{
			name:    "ui listen without port",
			cfg:     Config{Listen: []string{DefaultListen}, UIListen: "localhost", DataDir: "/tmp/x"},
			wantErr: "ui_listen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLogValueRedactsAuthToken(t *testing.T) {
	cfg := Config{Listen: []string{DefaultListen}, DataDir: "/tmp/x", AuthToken: "s3cret"}

	got := cfg.LogValue().String()
	if strings.Contains(got, "s3cret") {
		t.Fatalf("LogValue() = %q, want the auth token redacted", got)
	}

	if !strings.Contains(got, "auth_token=[redacted]") {
		t.Errorf("LogValue() = %q, want an auth_token=[redacted] attribute", got)
	}

	if !strings.Contains(got, "listen="+DefaultListen) {
		t.Errorf("LogValue() = %q, want the listen attribute", got)
	}
}

func TestUsage(t *testing.T) {
	var buf bytes.Buffer

	if err := Usage(&buf); err != nil {
		t.Fatalf("Usage: %v", err)
	}

	usage := buf.String()

	for _, want := range []string{
		"ratatoskr", "--listen", "--ui-listen", "--data-dir", "--no-open", "--auth-token", "--debug", "--allow-guests",
		// The repeatable flag still advertises its default.
		DefaultListen,
	} {
		if !strings.Contains(usage, want) {
			t.Errorf("Usage() does not mention %q:\n%s", want, usage)
		}
	}
}
