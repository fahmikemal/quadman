// Package config persists quadman preferences under the user's XDG config
// dir: the editor chosen on first use plus the optional YAML settings file
// (refresh rate, log tail/buffer, readonly mode, custom commands).
package config

import (
	"encoding/json"
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
	Theme           string          `yaml:"theme,omitempty"`
	Mouse           bool            `yaml:"mouse,omitempty"`
	CustomCommands  []CustomCommand `yaml:"custom_commands,omitempty"`
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

// Load reads the config file. A missing file is not an error — it just
// means no preferences have been saved yet. The YAML settings file is
// loaded alongside when present; a corrupt YAML file is ignored so a typo
// never bricks the TUI (the editor choice from JSON still applies).
func Load() (Config, error) {
	var c Config
	path, err := Path()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is quadman's own config file under the user's config dir
	if os.IsNotExist(err) {
		return loadYAML(c)
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return loadYAML(c)
}

// loadYAML overlays the YAML settings file onto c. Missing file = no-op;
// corrupt file = returned as error so callers (and the TUI status line) can
// surface it instead of silently ignoring the user's settings.
func loadYAML(c Config) (Config, error) {
	path, err := YAMLPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is quadman's own settings file under the user's config dir
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return c, err
	}
	c.Settings = s
	return c, nil
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

// Save writes the config file, creating its directory when needed.
func (c Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// Only the editor choice lives in JSON; YAML settings have their own file.
	data, err := json.MarshalIndent(struct {
		Editor string `json:"editor"`
	}{Editor: c.Editor}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
