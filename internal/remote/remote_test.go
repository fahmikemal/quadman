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
	for _, want := range []string{"BatchMode=yes", "root@remote", "systemctl", "--user", "show", "--", "web.service"} {
		if !strings.Contains(argv, want) {
			t.Errorf("ssh argv missing %q: %s", want, argv)
		}
	}
}

func TestCatRemote(t *testing.T) {
	bin, _ := fakeSSH(t)
	r := Runner{Target: "h", SSHBin: bin}
	if _, err := r.Cat(context.Background(), "/x/y.container"); err != nil {
		t.Fatal(err)
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

func TestRemoteNoLeadingDashDashToRemoteShell(t *testing.T) {
	r := Runner{Target: "ubuntu@remote"}
	argv := r.argv("systemctl", []string{"--user", "show", "web.service"})
	var targetIdx = -1
	for i, a := range argv {
		if a == "ubuntu@remote" {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		t.Fatalf("target not found in argv: %v", argv)
	}
	if targetIdx+1 >= len(argv) || argv[targetIdx+1] != "systemctl" {
		t.Fatalf("command following target must be 'systemctl', not %q (argv: %v)", argv[targetIdx+1], argv)
	}
}
