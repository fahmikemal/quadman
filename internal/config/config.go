// Package config persists quadman preferences under the user's XDG config
// dir: the editor chosen on first use plus the optional YAML settings file
// (refresh rate, log tail/buffer, readonly mode, custom commands).
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds quadman's persisted preferences.
type Config struct {
	// Editor is the editor binary chosen by the user, e.g. "nano".
	Editor string `json:"editor" yaml:"editor"`
	// Settings are the optional YAML-file tunables below. They are not
	// stored in the legacy JSON file.
	Settings Settings `json:"-" yaml:"-"`
}

// CustomCommand is one user-defined action from the YAML config. The command
// runs without a shell: args are split quote-aware and each arg is expanded
// as a Go template with .Name, .UnitName, .Kind and .Image.
type CustomCommand struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key,omitempty"`
	Run  string `yaml:"run"`
}

// Settings are the YAML-file tunables from config.yaml. Zero values mean
// "unset" and fall back to the built-in defaults.
type Settings struct {
	RefreshInterval time.Duration   `yaml:"refresh_interval,omitempty"`
	LogTail         int             `yaml:"log_tail,omitempty"`
	LogBuffer       int             `yaml:"log_buffer,omitempty"`
	Readonly        bool            `yaml:"readonly,omitempty"`
	System          bool            `yaml:"system,omitempty"`
	Theme           string          `yaml:"theme,omitempty"`
	Mouse           bool            `yaml:"mouse,omitempty"`
	QuadletDirs     []string        `yaml:"quadlet_dirs,omitempty"`
	CustomCommands  []CustomCommand `yaml:"custom_commands,omitempty"`
	Compartments    []string        `yaml:"compartments,omitempty"`
	// Dashboard set to false starts the unit list with the tiles and
	// "needs attention" block collapsed to one summary line (z toggles).
	Dashboard *bool         `yaml:"dashboard,omitempty"`
	Serve     ServeSettings `yaml:"serve,omitempty"`
	// SSHMultiplex set to false gives every --ssh call its own connection
	// instead of sharing one (OpenSSH ControlMaster). Default on.
	SSHMultiplex *bool `yaml:"ssh_multiplex,omitempty"`
}

// ServeSettings configures the built-in Wish SSH server mode.
type ServeSettings struct {
	Address        string `yaml:"address,omitempty"`
	Port           string `yaml:"port,omitempty"`
	HostKey        string `yaml:"host_key,omitempty"`
	AuthorizedKeys string `yaml:"authorized_keys,omitempty"`
	Password       string `yaml:"password,omitempty"`
	PasswordFile   string `yaml:"password_file,omitempty"`
	Readonly       bool   `yaml:"readonly,omitempty"`
}

// Defaults for the YAML tunables when unset.
const (
	DefaultRefreshInterval = 2500 * time.Millisecond
	DefaultLogTail         = 200
	DefaultLogBuffer       = 1000
)

// Path returns the config file location ($XDG_CONFIG_HOME/quadman/config.json).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quadman", "config.json"), nil
}

// YAMLPath returns the YAML settings location
// ($XDG_CONFIG_HOME/quadman/config.yaml).
func YAMLPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quadman", "config.yaml"), nil
}

// SettingsError reports a config.yaml that exists but cannot be used
// (unreadable, malformed, or holding unknown keys). Callers must treat the
// settings as unknown and fail closed: a dropped `readonly: true` must
// never silently grant write access.
type SettingsError struct {
	Path string
	Err  error
}

func (e *SettingsError) Error() string { return e.Path + ": " + e.Err.Error() }
func (e *SettingsError) Unwrap() error { return e.Err }

// IsSettingsError reports whether err (from Load) means config.yaml is
// unusable, as opposed to only the editor preference being lost.
func IsSettingsError(err error) bool {
	var se *SettingsError
	return errors.As(err, &se)
}

// Load reads the editor preference (config.json) and the settings file
// (config.yaml). Missing files are not errors. The two files fail
// independently: a corrupt config.json loses only the editor choice and
// still loads every YAML setting. A config.yaml problem is returned as a
// *SettingsError with zero-value settings; unknown keys count as a problem
// (a typo such as `read_only:` must not be ignored).
func Load() (Config, error) {
	var c Config
	var errs []error
	if path, err := Path(); err != nil {
		errs = append(errs, err)
	} else if data, err := os.ReadFile(path); err != nil && !os.IsNotExist(err) { // #nosec G304 -- path is quadman's own config file under the user's config dir
		errs = append(errs, fmt.Errorf("%s: %w", path, err))
	} else if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &c); err != nil {
			c = Config{}
			errs = append(errs, fmt.Errorf("%s: %w (editor preference ignored)", path, err))
		}
	}
	s, err := loadSettings()
	if err != nil {
		errs = append(errs, err)
	} else {
		c.Settings = s
	}
	return c, errors.Join(errs...)
}

// loadSettings reads config.yaml strictly. Missing file = zero settings.
func loadSettings() (Settings, error) {
	path, err := YAMLPath()
	if err != nil {
		return Settings{}, &SettingsError{Path: "config.yaml", Err: err}
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is quadman's own settings file under the user's config dir
	if os.IsNotExist(err) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, &SettingsError{Path: path, Err: err}
	}
	var s Settings
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil && !errors.Is(err, io.EOF) {
		return Settings{}, &SettingsError{Path: path, Err: err}
	}
	return s, nil
}

// RefreshInterval returns the configured poll tick or the default.
func (c Config) RefreshInterval() time.Duration {
	if c.Settings.RefreshInterval > 0 {
		return c.Settings.RefreshInterval
	}
	return DefaultRefreshInterval
}

// LogTail returns the configured journal snapshot size or the default.
func (c Config) LogTail() int {
	if c.Settings.LogTail > 0 {
		return c.Settings.LogTail
	}
	return DefaultLogTail
}

// LogBuffer returns the configured follow-buffer cap or the default.
func (c Config) LogBuffer() int {
	if c.Settings.LogBuffer > 0 {
		return c.Settings.LogBuffer
	}
	return DefaultLogBuffer
}

// Readonly reports whether the UI must refuse state-changing actions.
func (c Config) Readonly() bool {
	return c.Settings.Readonly
}

// System reports whether the UI runs in system mode by default.
func (c Config) System() bool {
	return c.Settings.System
}

// SSHMultiplex reports whether --ssh sessions share one connection.
func (c Config) SSHMultiplex() bool {
	return c.Settings.SSHMultiplex == nil || *c.Settings.SSHMultiplex
}

// Save writes the config file atomically (temp file + rename), creating
// its directory when needed, so a crash or a full disk never leaves a
// truncated config.json behind.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// Only the editor choice lives in JSON; YAML settings have their own file.
	data, err := json.MarshalIndent(struct {
		Editor string `json:"editor"`
	}{Editor: c.Editor}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
