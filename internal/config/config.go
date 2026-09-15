// Package config persists small quadman preferences (today: the editor
// chosen on first use) under the user's XDG config dir.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds quadman's persisted preferences.
type Config struct {
	// Editor is the editor binary chosen by the user, e.g. "nano".
	Editor string `json:"editor"`
}

// Path returns the config file location ($XDG_CONFIG_HOME/quadman/config.json).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "quadman", "config.json"), nil
}

// Load reads the config file. A missing file is not an error — it just
// means no preferences have been saved yet.
func Load() (Config, error) {
	var c Config
	path, err := Path()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is quadman's own config file under the user's config dir
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(data, &c)
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
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
