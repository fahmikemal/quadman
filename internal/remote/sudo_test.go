package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSudoArgv(t *testing.T) {
	r := Runner{As: "svc", Env: []string{"XDG_RUNTIME_DIR=/run/user/1001", "HOME=/home/svc"}}
	got := r.sudoArgv("systemctl", []string{"--user", "show", "web.service"})
	want := []string{"-n", "-u", "svc", "--", "env",
		"XDG_RUNTIME_DIR=/run/user/1001", "HOME=/home/svc",
		"systemctl", "--user", "show", "web.service"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv = %q", got)
	}
}

func TestSudoArgvNoEnv(t *testing.T) {
	r := Runner{As: "svc"}
	got := r.sudoArgv("true", nil)
	if len(got) != 5 || got[0] != "-n" || got[4] != "true" {
		t.Errorf("argv = %q", got)
	}
	for _, a := range got {
		if a == "env" {
			t.Error("env carrier must be omitted when Env is empty")
		}
	}
	if (Runner{}).Isolated() {
		t.Error("zero runner is not isolated")
	}
	if !(Runner{Target: "h"}).Isolated() || !(Runner{As: "u"}).Isolated() {
		t.Error("ssh and sudo runners are isolated")
	}
	if (Runner{As: "u"}).IsRemote() {
		t.Error("sudo runner is not an SSH remote")
	}
}

// fakeSudo puts a recording fake sudo first on PATH.
func fakeSudo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s ' \"$@\" > " + filepath.Join(dir, "argv") + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestSudoCommandRunsSudo(t *testing.T) {
	dir := fakeSudo(t)
	r := Runner{As: "svc", Env: []string{"HOME=/home/svc"}}
	if _, err := r.Output(context.Background(), "true"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(b))
	if !strings.HasPrefix(got, "-n -u svc -- env HOME=/home/svc true") {
		t.Errorf("sudo argv = %q", got)
	}
}

func TestSudoCommandDir(t *testing.T) {
	r := Runner{As: "svc"}
	if got := r.Command(context.Background(), "true").Dir; got != "/" {
		t.Errorf("empty Dir must fall back to /, got %q", got)
	}
	r.Dir = "/home/svc"
	if got := r.Command(context.Background(), "true").Dir; got != "/home/svc" {
		t.Errorf("Dir = %q", got)
	}
}

func TestSudoFailureSurfaces(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\necho 'a password is required' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := Runner{As: "svc"}
	if _, err := r.Output(context.Background(), "true"); err == nil {
		t.Error("expected error")
	} else if !strings.Contains(err.Error(), "sudo -u svc") || !strings.Contains(err.Error(), "password is required") {
		t.Errorf("err = %v", err)
	}
}
