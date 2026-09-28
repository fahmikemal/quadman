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

func TestCheckSecretFile(t *testing.T) {
	write := func(t *testing.T, content string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "pw")
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}

	content, warn, err := checkSecretFile(write(t, "s3cret\n", 0o600))
	if err != nil || content != "s3cret" || warn != "" {
		t.Errorf("0600 file = %q, warn %q, err %v", content, warn, err)
	}
	_, warn, err = checkSecretFile(write(t, "s3cret", 0o644))
	if err != nil || warn == "" {
		t.Errorf("0644 file must warn, warn %q err %v", warn, err)
	}
	if _, _, err := checkSecretFile(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing file must fail")
	}
	if _, _, err := checkSecretFile(write(t, "\n", 0o600)); err == nil {
		t.Error("blank file must fail")
	}
	if _, _, err := checkSecretFile(t.TempDir()); err == nil {
		t.Error("directory must fail")
	}
}

func TestResolveServePassword(t *testing.T) {
	pwFile := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(pwFile, []byte("filepw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	noEnv := func(string) string { return "" }
	withEnv := func(string) string { return "envpw" }

	tests := []struct {
		name     string
		flag     string
		fileFlag string
		scfg     config.ServeSettings
		getenv   func(string) string
		want     string
		source   string
		wantErr  bool
	}{
		{"none", "", "", config.ServeSettings{}, noEnv, "", "", false},
		{"flag", "flagpw", "", config.ServeSettings{}, noEnv, "flagpw", "--password", false},
		{"file", "", pwFile, config.ServeSettings{}, noEnv, "filepw", "--password-file", false},
		{"env", "", "", config.ServeSettings{}, withEnv, "envpw", servePasswordEnv, false},
		{"config", "", "", config.ServeSettings{Password: "cfgpw"}, noEnv, "cfgpw", "config password", false},
		{"config file", "", "", config.ServeSettings{PasswordFile: pwFile}, noEnv, "filepw", "config password_file", false},
		{"flag beats config", "flagpw", "", config.ServeSettings{Password: "cfgpw"}, noEnv, "flagpw", "--password", false},
		{"conflict flag+file", "a", pwFile, config.ServeSettings{}, noEnv, "", "", true},
		{"conflict flag+env", "a", "", config.ServeSettings{}, withEnv, "", "", true},
		{"conflict config pair", "", "", config.ServeSettings{Password: "a", PasswordFile: pwFile}, noEnv, "", "", true},
		{"missing file", "", filepath.Join(t.TempDir(), "nope"), config.ServeSettings{}, noEnv, "", "", true},
	}
	for _, tc := range tests {
		got, src, _, err := resolveServePassword(tc.flag, tc.fileFlag, tc.scfg, tc.getenv)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && (got != tc.want || src != tc.source) {
			t.Errorf("%s: = %q/%q, want %q/%q", tc.name, got, src, tc.want, tc.source)
		}
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

func TestCheckModeConflict(t *testing.T) {
	if err := checkModeConflict(true, "svc-web"); err == nil {
		t.Error("--system + --as must be rejected")
	}
	for _, tc := range [][2]any{{true, ""}, {false, "svc-web"}, {false, ""}} {
		if err := checkModeConflict(tc[0].(bool), tc[1].(string)); err != nil {
			t.Errorf("checkModeConflict(%v, %q) = %v, want nil", tc[0], tc[1], err)
		}
	}
}

func TestMisplacedFlagExcept(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--format", "json"}, ""},
		{[]string{"json"}, ""},
		{[]string{"--json"}, ""},
		{[]string{"-format=json"}, ""},
		{[]string{"--system"}, "system"},
		{[]string{"--format", "json", "--as", "u"}, "as"},
		{[]string{"--format=json", "--readonly"}, "readonly"},
		{nil, ""},
	}
	for _, tc := range tests {
		if got := misplacedFlagExcept(tc.args, "format", "json"); got != tc.want {
			t.Errorf("misplacedFlagExcept(%v) = %q, want %q", tc.args, got, tc.want)
		}
	}
}
