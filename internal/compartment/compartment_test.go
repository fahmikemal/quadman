package compartment

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelf(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	c, err := Self()
	if err != nil {
		t.Fatal(err)
	}
	if c.User != u.Username || c.UID != u.Uid || c.Home != u.HomeDir {
		t.Errorf("self = %+v, want user %+v", c, u)
	}
	if c.RuntimeDir != filepath.Join("/run", "user", u.Uid) {
		t.Errorf("runtime = %q", c.RuntimeDir)
	}
}

func TestResolveUnknown(t *testing.T) {
	if _, err := Resolve("quadman-nosuch-user-xyz"); err == nil {
		t.Error("expected error")
	}
	if _, err := Resolve("  "); err == nil {
		t.Error("blank name must fail")
	}
}

func TestRunnerCarriesSessionEnv(t *testing.T) {
	c := Compartment{User: "svc", UID: "1001", Home: "/home/svc", RuntimeDir: "/run/user/1001"}
	r := c.Runner()
	if r.As != "svc" {
		t.Errorf("As = %q", r.As)
	}
	if r.Dir != "/" {
		t.Errorf("Dir = %q, want universally traversable /", r.Dir)
	}
	want := map[string]bool{
		"XDG_RUNTIME_DIR=/run/user/1001": false,
		"HOME=/home/svc":                 false,
	}
	for _, e := range r.Env {
		want[e] = true
	}
	for e, seen := range want {
		if !seen {
			t.Errorf("Env missing %q: %v", e, r.Env)
		}
	}
}

func TestProbeFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\necho 'a password is required' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := Compartment{User: "svc", UID: "1001", Home: "/home/svc", RuntimeDir: "/run/user/1001"}
	err := c.Probe(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"sudo -n -u svc", "NOPASSWD:SETENV"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}

// TestProbeMirrorsRealInvocation: the probe must carry the same session
// variables as every real command, so a rule without SETENV fails early.
func TestProbeMirrorsRealInvocation(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "argv")
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\nprintf '%s ' \"$@\" > "+record+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := Compartment{User: "svc", UID: "1001", Home: "/home/svc", RuntimeDir: "/run/user/1001"}
	if err := c.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(record)
	if got := strings.TrimSpace(string(raw)); got != "-n -u svc XDG_RUNTIME_DIR=/run/user/1001 HOME=/home/svc -- true" {
		t.Errorf("probe argv = %q", got)
	}
}

func TestProbeSudoUnknownUser(t *testing.T) {
	if err := ProbeSudo(context.Background(), "no-such-user-quadman"); err == nil || !strings.Contains(err.Error(), "unknown user") {
		t.Errorf("err = %v", err)
	}
}

func TestSetupRuntimeMissing(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	c, err := Resolve(u.Username)
	if err != nil {
		t.Fatal(err)
	}
	if c.RuntimeReady() {
		t.Skip("this host has a runtime dir for the current user")
	}
	if _, err := Setup(context.Background(), u.Username); err == nil || !strings.Contains(err.Error(), "no runtime dir") {
		t.Errorf("Setup must refuse a compartment without a runtime dir, got %v", err)
	}
}
