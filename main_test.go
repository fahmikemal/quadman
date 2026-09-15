package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kemal-labs/quadman/internal/systemd"
)

// fakeSystemctl writes a script that answers `systemctl show` for the two
// fixture units and records its arguments.
func fakeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "systemctl")
	script := `#!/bin/sh
cat <<'EOF'
Id=webapp.service
LoadState=loaded
ActiveState=active
SubState=running
Description=Web App

Id=cache-volume.service
LoadState=not-found
ActiveState=inactive
SubState=dead
Description=cache
EOF
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunList(t *testing.T) {
	cfg := t.TempDir()
	quadDir := filepath.Join(cfg, "containers", "systemd")
	if err := os.MkdirAll(quadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(quadDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("webapp.container", "[Container]\nImage=docker.io/library/nginx:latest\n")
	write("cache.volume", "[Volume]\n")
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir()) // no generator output → also no real units

	var buf bytes.Buffer
	sys := &systemd.Systemd{User: true, Bin: fakeSystemctl(t)}
	if err := runList(&buf, sys); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"QUADLET", "webapp", "webapp.service", "active", "running", "docker.io/library/nginx:latest", "cache", "cache-volume.service", "not-found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunListEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var buf bytes.Buffer
	sys := &systemd.Systemd{User: true, Bin: fakeSystemctl(t)}
	if err := runList(&buf, sys); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no quadlet units found") {
		t.Errorf("empty discovery should print the hint, got:\n%s", buf.String())
	}
}

func TestRunListShowFailure(t *testing.T) {
	cfg := t.TempDir()
	quadDir := filepath.Join(cfg, "containers", "systemd")
	if err := os.MkdirAll(quadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quadDir, "webapp.container"), []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	dir := t.TempDir()
	broken := filepath.Join(dir, "systemctl")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	sys := &systemd.Systemd{User: true, Bin: broken}
	if err := runList(&buf, sys); err != nil {
		t.Fatal(err) // a broken systemctl must degrade to "-" states, not fail
	}
	if !strings.Contains(buf.String(), "webapp") {
		t.Errorf("output should still list the unit, got:\n%s", buf.String())
	}
}
