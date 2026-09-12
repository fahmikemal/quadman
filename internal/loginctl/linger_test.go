package loginctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeBin(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnabled(t *testing.T) {
	for _, tc := range []struct {
		out  string
		want bool
	}{
		{"yes\n", true},
		{"no\n", false},
		{"", false},
	} {
		bin := fakeBin(t, "loginctl", "printf '"+tc.out+"'")
		l := &Loginctl{Bin: bin}
		got, err := l.Enabled(context.Background(), "alice")
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("Enabled with output %q = %v, want %v", tc.out, got, tc.want)
		}
	}
}

func TestEnabledArgs(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := fakeBin(t, "loginctl", "printf 'yes\\n'\nprintf '%s\\n' \"$@\" > \""+argsFile+"\"")
	l := &Loginctl{Bin: bin}

	if _, err := l.Enabled(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	want := "show-user --property=Linger --value -- alice"
	if got := strings.Join(args, " "); got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestEnabledError(t *testing.T) {
	bin := fakeBin(t, "loginctl", "echo 'Failed' >&2\nexit 1\n")
	l := &Loginctl{Bin: bin}
	if _, err := l.Enabled(context.Background(), "alice"); err == nil {
		t.Fatal("Enabled should fail when loginctl exits non-zero")
	}
}

func TestEnabledTimeout(t *testing.T) {
	bin := fakeBin(t, "loginctl", "sleep 2\n")
	l := &Loginctl{Bin: bin, Timeout: 100 * time.Millisecond}
	if _, err := l.Enabled(context.Background(), "alice"); err == nil {
		t.Fatal("Enabled should fail when loginctl exceeds the timeout")
	}
}

func TestSet(t *testing.T) {
	cases := []struct {
		on   bool
		verb string
	}{
		{true, "enable-linger"},
		{false, "disable-linger"},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		argsFile := filepath.Join(dir, "args")
		bin := fakeBin(t, "loginctl", "printf '%s\\n' \"$@\" > \""+argsFile+"\"")
		l := &Loginctl{Bin: bin}

		if err := l.Set(context.Background(), "alice", tc.on); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if got := strings.Join(args, " "); got != tc.verb+" -- alice" {
			t.Errorf("args = %q, want %q", got, tc.verb+" -- alice")
		}
	}
}

func TestSetError(t *testing.T) {
	bin := fakeBin(t, "loginctl", "echo 'Access denied'\nexit 1\n")
	l := &Loginctl{Bin: bin}
	err := l.Set(context.Background(), "alice", true)
	if err == nil {
		t.Fatal("Set should fail when loginctl exits non-zero")
	}
	if !strings.Contains(err.Error(), "enable-linger") || !strings.Contains(err.Error(), "Access denied") {
		t.Errorf("error should wrap verb and output, got %v", err)
	}
}

func TestCurrentUser(t *testing.T) {
	name, err := CurrentUser()
	if err != nil {
		t.Skipf("cannot determine current user in this environment: %v", err)
	}
	if name == "" {
		t.Error("CurrentUser should not return an empty name")
	}
}
