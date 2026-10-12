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
	writeYAML(t, dir, "refresh_interval: 5s\nlog_tail: 50\nlog_buffer: 200\nreadonly: true\nsystem: true\n")

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
	if !c.System() {
		t.Error("system must be true")
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
	if c.System() {
		t.Error("system must default to false")
	}
}

func TestLoadYAMLCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "refresh_interval: [unclosed\n")
	if _, err := Load(); !IsSettingsError(err) {
		t.Errorf("corrupt YAML should return a settings error, got %v", err)
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

func TestLoadServeSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "serve:\n  address: 127.0.0.1:2222\n  authorized_keys: /home/u/.ssh/authorized_keys\n  password_file: /run/secrets/quadman-pass\n  readonly: true\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := c.Settings.Serve
	if s.Address != "127.0.0.1:2222" || s.AuthorizedKeys == "" || s.PasswordFile == "" || !s.Readonly {
		t.Errorf("serve = %+v", s)
	}
}

func TestLoadQuadletDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "quadlet_dirs:\n  - /home/u/quadlets\n  - /opt/quadlets\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Settings.QuadletDirs) != 2 || c.Settings.QuadletDirs[0] != "/home/u/quadlets" {
		t.Errorf("quadlet_dirs = %+v", c.Settings.QuadletDirs)
	}
}

// TestLoadCorruptJSONKeepsYAML: a broken editor preference must never drop
// the YAML settings, least of all readonly.
func TestLoadCorruptJSONKeepsYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "readonly: true\n")
	if err := os.WriteFile(filepath.Join(dir, "quadman", "config.json"), []byte("{trunc"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err == nil {
		t.Error("the JSON problem must still be reported")
	}
	if IsSettingsError(err) {
		t.Errorf("a JSON problem is not a settings problem: %v", err)
	}
	if !c.Readonly() {
		t.Error("readonly from YAML must survive a corrupt config.json")
	}
}

func TestLoadYAMLUnknownKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "read_only: true\n")
	c, err := Load()
	if !IsSettingsError(err) {
		t.Fatalf("an unknown key must be a settings error, got %v", err)
	}
	if c.Readonly() {
		t.Error("settings must be zero when config.yaml is rejected")
	}
}

func TestLoadYAMLEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeYAML(t, dir, "# only a comment\n")
	if _, err := Load(); err != nil {
		t.Errorf("an empty config.yaml is valid: %v", err)
	}
}

func TestSSHMultiplexDefault(t *testing.T) {
	if !(Config{}).SSHMultiplex() {
		t.Error("multiplexing is on by default")
	}
	off := false
	if (Config{Settings: Settings{SSHMultiplex: &off}}).SSHMultiplex() {
		t.Error("ssh_multiplex: false must turn it off")
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := (Config{Editor: "vim"}).Save(); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "quadman", ".config-*")); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	st, err := os.Stat(filepath.Join(dir, "quadman", "config.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("config.json = %v, %v", st, err)
	}
}
