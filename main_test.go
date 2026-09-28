package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/server"
	"github.com/fahmikemal/quadman/internal/systemd"
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
	if err := runList(&buf, sys, false, []string{quadDir}); err != nil {
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
	if err := runList(&buf, sys, false, quadlet.SearchDirsMode(false)); err != nil {
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
	if err := runList(&buf, sys, false, []string{quadDir}); err != nil {
		t.Fatal(err) // a broken systemctl must degrade to "-" states, not fail
	}
	if !strings.Contains(buf.String(), "webapp") {
		t.Errorf("output should still list the unit, got:\n%s", buf.String())
	}
}

func TestResolveServeAddr(t *testing.T) {
	tests := []struct {
		name     string
		addrFlag string
		portFlag string
		scfg     config.ServeSettings
		want     string
	}{
		{"explicit address wins", "0.0.0.0:2222", "9999", config.ServeSettings{}, "0.0.0.0:2222"},
		{"port flag binds loopback", "", "2222", config.ServeSettings{}, "127.0.0.1:2222"},
		{"config address wins", "", "", config.ServeSettings{Address: "192.168.1.9:2022"}, "192.168.1.9:2022"},
		{"config port binds loopback", "", "", config.ServeSettings{Port: "2022"}, "127.0.0.1:2022"},
		{"default is loopback", "", "", config.ServeSettings{}, server.DefaultAddress},
	}
	for _, tc := range tests {
		if got := resolveServeAddr(tc.addrFlag, tc.portFlag, tc.scfg); got != tc.want {
			t.Errorf("%s: resolveServeAddr = %q, want %q", tc.name, got, tc.want)
		}
	}
	if !server.IsLoopbackAddr(server.DefaultAddress) {
		t.Errorf("server.DefaultAddress = %q, must bind loopback only", server.DefaultAddress)
	}
}

func TestFormatConnectHint(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{":2222", "-p 2222 <host>"},
		{"0.0.0.0:2222", "-p 2222 <host>"},
		{"127.0.0.1:2222", "-p 2222 127.0.0.1"},
		{"127.0.0.1:22", "127.0.0.1"},
		{":22", "<host>"},
		{"custom.host:8022", "-p 8022 <host>"},
	}
	for _, tc := range tests {
		got := formatConnectHint(tc.addr)
		if got != tc.want {
			t.Errorf("formatConnectHint(%q) = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestCompSetupUnknownUser(t *testing.T) {
	if _, err := compSetup("quadman-nosuch-user-xyz"); err == nil {
		t.Error("unknown compartment user must fail setup")
	}
}

func TestMisplacedFlag(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--as", "x"}, "as"},
		{[]string{"-as=x"}, "as"},
		{[]string{"list", "--as"}, "as"},
		{[]string{"--system"}, "system"},
		{[]string{"-system"}, "system"},
		{[]string{"--system=true"}, "system"},
		{[]string{"--readonly"}, "readonly"},
		{[]string{"--quadlet-dir=/tmp/q"}, "quadlet-dir"},
		{nil, ""},
		{[]string{}, ""},
		{[]string{"list"}, ""},
		{[]string{"--"}, ""},
		{[]string{"list", "--", "--system"}, ""},
	}
	for _, tc := range tests {
		if got := misplacedFlag(tc.args); got != tc.want {
			t.Errorf("misplacedFlag(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
