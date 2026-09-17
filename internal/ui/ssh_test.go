package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/podlet"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
)

func fakeSSHBin(t *testing.T, script string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func sshModel(t *testing.T) Model {
	t.Helper()
	// Answers podman quadlet list, systemctl show, and cat through one fake.
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q 'quadlet.*list'; then echo \"web.container\\tweb.service\\t/r/web.container\"; exit 0; fi\n" +
		"if echo \"$@\" | grep -q 'cat.*--'; then echo '[Container]'; echo 'Image=nginx'; exit 0; fi\n" +
		"echo 'Id=web.service'; echo 'LoadState=loaded'; echo 'ActiveState=active'; echo 'SubState=running'; echo 'Description=x'; echo ''\n"
	m := New()
	m.applySSH("user@remote")
	fake := fakeSSHBin(t, script)
	m.ssh.SSHBin = fake
	m.sys.Remote.SSHBin = fake
	m.lc.Remote.SSHBin = fake
	podman.DefaultRunner.SSHBin = fake
	podlet.DefaultRunner.SSHBin = fake
	t.Cleanup(func() {
		podman.DefaultRunner.SSHBin = ""
		podlet.DefaultRunner.SSHBin = ""
	})
	return m
}

func TestSSHApplyWiresClients(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	if !m.ssh.IsRemote() || m.sys.Remote.Target != "user@remote" || m.lc.Remote.Target != "user@remote" {
		t.Fatalf("clients not pointed at remote: %+v", m.ssh)
	}
	if podman.DefaultRunner.Target != "user@remote" || podlet.DefaultRunner.Target != "user@remote" {
		t.Error("podman/podlet runners must follow --ssh")
	}
	podman.DefaultRunner.Target = ""
	podlet.DefaultRunner.Target = ""
}

func TestSSHRefusesFileWrites(t *testing.T) {
	m := sshModel(t)
	m = withUnits(m, "webapp")
	m.table.SetCursor(0)
	for _, key := range []string{"e", "d", "E", "i", "D", "n"} {
		model, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		mm := model.(Model)
		if mm.pending != nil || mm.generating || mm.instancing || mm.pickingEditor {
			t.Errorf("key %q must not start file work over SSH", key)
		}
		if !strings.Contains(mm.statusLine, "SSH") {
			t.Errorf("key %q must explain SSH limits, got %q", key, mm.statusLine)
		}
	}
	podman.DefaultRunner.Target = ""
	podlet.DefaultRunner.Target = ""
}

func TestSSHReadsFileViaCat(t *testing.T) {
	m := sshModel(t)
	u := quadlet.Unit{Name: "web", Kind: quadlet.KindContainer, Path: "/r/web.container", UnitName: "web.service"}
	data, err := m.readUnitFile(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Image=nginx") {
		t.Errorf("content = %q, want cat output", data)
	}
	f, err := m.parseUnitFile(u)
	if err != nil {
		t.Fatal(err)
	}
	if f.Image() != "nginx" {
		t.Errorf("image = %q", f.Image())
	}
	podman.DefaultRunner.Target = ""
	podlet.DefaultRunner.Target = ""
}

func TestSSHTitleShowsTarget(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	if !strings.Contains(m.View().Content, "user@remote") {
		t.Errorf("title must show the remote target: %q", m.View().Content)
	}
	podman.DefaultRunner.Target = ""
	podlet.DefaultRunner.Target = ""
}

func TestSSHRefreshCmd(t *testing.T) {
	m := sshModel(t)
	cmd := refreshCmd(m.sys, m.lc, nil, m.ssh, enrichNone, false)
	msg := cmd()
	rm, ok := msg.(refreshMsg)
	if !ok {
		t.Fatalf("expected refreshMsg, got %T", msg)
	}
	if rm.err != nil {
		t.Fatalf("refreshCmd returned error over SSH: %v", rm.err)
	}
	if len(rm.units) != 1 || rm.units[0].Name != "web" {
		t.Errorf("expected 1 unit 'web', got %v", rm.units)
	}
	podman.DefaultRunner.Target = ""
	podlet.DefaultRunner.Target = ""
}
