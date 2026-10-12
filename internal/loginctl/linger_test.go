package loginctl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
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

// TestResolveCompartmentUser: an empty user name in a compartment means the
// sudo target, never the operator running quadman.
func TestResolveCompartmentUser(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "argv")
	sudo := "#!/bin/sh\nprintf '%s ' \"$@\" > " + record + "\necho yes\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(sudo), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	l := &Loginctl{Bin: "loginctl", Remote: remote.Runner{As: "svc-web"}}
	on, err := l.Enabled(context.Background(), "")
	if err != nil || !on {
		t.Fatalf("Enabled = %v, %v", on, err)
	}
	raw, _ := os.ReadFile(record)
	if !strings.HasSuffix(strings.TrimSpace(string(raw)), "show-user --property=Linger --value -- svc-web") {
		t.Errorf("compartment linger must query the target user, sudo argv = %q", raw)
	}
}

// TestResolveRemoteUser: over SSH the remote login name comes from the
// remote host (id -un), once, then every call reuses it.
func TestResolveRemoteUser(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	ssh := filepath.Join(dir, "ssh")
	script := `#!/bin/sh
for last; do :; done
echo "$last" >> ` + calls + `
case "$last" in
  "id -un") echo ubuntu ;;
  *) echo no ;;
esac
`
	if err := os.WriteFile(ssh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	l := &Loginctl{Bin: "loginctl", Remote: remote.Runner{Target: "ubuntu@host", SSHBin: ssh}}
	for range 2 {
		if _, err := l.Enabled(context.Background(), ""); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(calls)
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"id -un", "loginctl show-user --property=Linger --value -- ubuntu", "loginctl show-user --property=Linger --value -- ubuntu"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("remote calls = %q, want %q", got, want)
	}
}

func TestResolvePinnedUser(t *testing.T) {
	l := &Loginctl{User: "bob", Remote: remote.Runner{As: "svc"}}
	if got, err := l.resolve(context.Background(), ""); err != nil || got != "bob" {
		t.Errorf("pinned user = %q, %v", got, err)
	}
	if got, _ := l.resolve(context.Background(), "carol"); got != "carol" {
		t.Errorf("explicit user must win, got %q", got)
	}
}
