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

func TestProbeSudoFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\necho 'a password is required' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := ProbeSudo(context.Background(), "svc")
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"sudo -n -u svc", "NOPASSWD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want %q", err, want)
		}
	}
}
