package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeYAML(t *testing.T, dir, content string) {
	t.Helper()
	sub := filepath.Join(dir, "quadman")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadYAMLSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "refresh_interval: 5s\nlog_tail: 50\nlog_buffer: 200\nreadonly: true\n")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RefreshInterval() != 5*time.Second {
		t.Errorf("refresh = %v, want 5s", c.RefreshInterval())
	}
	if c.LogTail() != 50 {
		t.Errorf("tail = %d, want 50", c.LogTail())
	}
	if c.LogBuffer() != 200 {
		t.Errorf("buffer = %d, want 200", c.LogBuffer())
	}
	if !c.Readonly() {
		t.Error("readonly must be true")
	}
}

func TestLoadYAMLDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.RefreshInterval() != DefaultRefreshInterval {
		t.Errorf("refresh = %v, want default %v", c.RefreshInterval(), DefaultRefreshInterval)
	}
	if c.LogTail() != DefaultLogTail {
		t.Errorf("tail = %d, want %d", c.LogTail(), DefaultLogTail)
	}
	if c.LogBuffer() != DefaultLogBuffer {
		t.Errorf("buffer = %d, want %d", c.LogBuffer(), DefaultLogBuffer)
	}
	if c.Readonly() {
		t.Error("readonly must default to false")
	}
}

func TestLoadYAMLCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "refresh_interval: [unclosed\n")
	if _, err := Load(); err == nil {
		t.Error("corrupt YAML should return an error")
	}
}

func TestLoadJSONEditorWithYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := (Config{Editor: "nano"}).Save(); err != nil {
		t.Fatal(err)
	}
	writeYAML(t, dir, "readonly: true\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Editor != "nano" {
		t.Errorf("editor = %q, want nano (JSON still applies)", c.Editor)
	}
	if !c.Readonly() {
		t.Error("readonly from YAML must apply alongside the JSON editor")
	}
}

func TestSaveKeepsYAMLIntact(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "readonly: true\n")
	if err := (Config{Editor: "nano"}).Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "quadman", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "readonly: true\n" {
		t.Errorf("Save must not touch config.yaml, got %q", data)
	}
}

func TestLoadCustomCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "custom_commands:\n  - name: status\n    key: S\n    run: systemctl --user status {{.UnitName}}\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Settings.CustomCommands) != 1 {
		t.Fatalf("commands = %+v, want 1", c.Settings.CustomCommands)
	}
	cc := c.Settings.CustomCommands[0]
	if cc.Name != "status" || cc.Key != "S" || cc.Run == "" {
		t.Errorf("command = %+v", cc)
	}
}
