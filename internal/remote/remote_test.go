package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSSH writes a script that records its argv and echoes a marker.
func fakeSSH(t *testing.T) (bin, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "argv")
	bin = filepath.Join(dir, "ssh")
	script := "#!/bin/sh\necho \"$@\" > " + record + "\necho FAKE-OUT\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, record
}

// shellSSH is a fake ssh that behaves like OpenSSH on the wire: it drops
// the options and the destination, joins what is left with spaces, and
// hands that one string to a POSIX shell — exactly what sshd does with
// the remote login shell.
func shellSSH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ssh")
	script := `#!/bin/sh
while [ $# -gt 0 ]; do
  case "$1" in
    -o) shift 2 ;;
    --) shift; shift; break ;;
    *) shift ;;
  esac
done
exec /bin/sh -c "$*"
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestLocalCommand(t *testing.T) {
	var r Runner
	out, err := r.Output(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "hi" {
		t.Fatalf("out = %q, want hi", out)
	}
}

func TestRemoteArgv(t *testing.T) {
	bin, record := fakeSSH(t)
	r := Runner{Target: "root@remote", SSHBin: bin}
	out, err := r.Output(context.Background(), "systemctl", "--user", "show", "--", "web.service")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "FAKE-OUT") {
		t.Fatalf("out = %q", out)
	}
	raw, _ := os.ReadFile(record)
	argv := string(raw)
	for _, want := range []string{"BatchMode=yes", "root@remote", "systemctl --user show -- web.service"} {
		if !strings.Contains(argv, want) {
			t.Errorf("ssh argv missing %q: %s", want, argv)
		}
	}
}

// TestRemoteArgvRoundTrip proves the remote shell rebuilds the exact argv:
// tabs, spaces, quotes, globs, $ and backslashes must all survive.
func TestRemoteArgvRoundTrip(t *testing.T) {
	r := Runner{Target: "h", SSHBin: shellSSH(t)}
	args := []string{
		"{{.Name}}\t{{.UnitName}}\t{{.Path}}",
		"two words",
		"it's",
		`"double"`,
		"$HOME",
		"*",
		`back\slash`,
		"~",
		"",
		"web@foo\\x2dbar.service",
		"a;rm -rf /",
	}
	out, err := r.Output(context.Background(), "printf", append([]string{"<%s>\\n"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	var want strings.Builder
	for _, a := range args {
		want.WriteString("<" + a + ">\n")
	}
	if string(out) != want.String() {
		t.Errorf("remote argv mangled:\n got %q\nwant %q", out, want.String())
	}
}

func TestRemoteCommandEnv(t *testing.T) {
	r := Runner{Target: "h", SSHBin: shellSSH(t)}
	out, err := r.OutputEnv(context.Background(), []string{"QM_TEST=a b", "bad key=x", "=nokey"}, "sh", "-c", `printf %s "$QM_TEST"`)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a b" {
		t.Errorf("env value = %q, want %q", out, "a b")
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "''",
		"web.service":      "web.service",
		"--property=Id,X":  "--property=Id,X",
		"/a/b-c_d@e%f+g:h": "/a/b-c_d@e%f+g:h",
		"two words":        "'two words'",
		"it's":             `'it'\''s'`,
		"$x":               "'$x'",
		"~":                "'~'",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemoteCommandQuotesAssignmentLikeName(t *testing.T) {
	if got := RemoteCommand(nil, "A=b", []string{"x"}); got != "'A=b' x" {
		t.Errorf("RemoteCommand = %q", got)
	}
	if got := RemoteCommand([]string{"LC_ALL=C"}, "systemctl", []string{"list-timers"}); got != "LC_ALL=C systemctl list-timers" {
		t.Errorf("RemoteCommand = %q", got)
	}
}

func TestLocalCommandEnv(t *testing.T) {
	var r Runner
	out, err := r.OutputEnv(context.Background(), []string{"QM_LOCAL=yes"}, "sh", "-c", `printf %s "$QM_LOCAL"`)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "yes" {
		t.Errorf("local env = %q", out)
	}
	if cmd := r.Command(context.Background(), "true"); cmd.Env != nil {
		t.Error("no extra env must keep the inherited environment untouched")
	}
}

func TestCatRemote(t *testing.T) {
	bin, record := fakeSSH(t)
	r := Runner{Target: "h", SSHBin: bin}
	if _, err := r.Cat(context.Background(), "/x/my unit.container"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(record)
	if !strings.Contains(string(raw), "cat -- '/x/my unit.container'") {
		t.Errorf("cat argv = %s", raw)
	}
}

func TestIsRemote(t *testing.T) {
	if (Runner{}).IsRemote() {
		t.Error("zero runner must be local")
	}
	if !(Runner{Target: "h"}).IsRemote() {
		t.Error("target must be remote")
	}
}

func TestRemoteTargetAfterDashDash(t *testing.T) {
	r := Runner{Target: "-oProxyCommand=evil"}
	argv := r.sshArgv(nil, "systemctl", []string{"--user", "show", "web.service"})
	for i, a := range argv {
		if a == r.Target {
			if i == 0 || argv[i-1] != "--" {
				t.Fatalf("target must follow --: %v", argv)
			}
			if i+1 >= len(argv) || argv[i+1] != "systemctl --user show web.service" {
				t.Fatalf("command must be one quoted word after the target: %v", argv)
			}
			return
		}
	}
	t.Fatalf("target not found in argv: %v", argv)
}

func TestMultiplexOptions(t *testing.T) {
	r := Runner{Target: "h"}
	if opts := r.muxOptions(); opts != nil {
		t.Errorf("multiplexing must be opt-in, got %v", opts)
	}
	r.Multiplex = true
	opts := strings.Join(r.muxOptions(), " ")
	for _, want := range []string{"ControlMaster=auto", "ControlPath=", "%C", "ControlPersist=60"} {
		if !strings.Contains(opts, want) {
			t.Errorf("mux options missing %q: %s", want, opts)
		}
	}
	dir, err := controlDir()
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("control dir must exist with mode 0700: %v %v", fi, err)
	}
	Shutdown(context.Background())
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Shutdown must remove the control dir, stat err = %v", err)
	}
	if opts := r.muxOptions(); opts != nil {
		t.Errorf("after Shutdown multiplexing must degrade to plain connections, got %v", opts)
	}
}

func TestLabel(t *testing.T) {
	if (Runner{}).Label() != "" || (Runner{Target: "h"}).Label() != "h" || (Runner{As: "svc"}).Label() != "sudo -u svc" {
		t.Error("labels")
	}
}
