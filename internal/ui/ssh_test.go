package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// useFakeSSH rebuilds the model's session around a fake ssh binary, so
// every client (systemctl, loginctl, podman, podlet) goes through it.
func useFakeSSH(m *Model, fake string) {
	r := m.ssh
	r.SSHBin = fake
	m.setSession(r, m.system, m.sys.GenDir)
}

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
	useFakeSSH(&m, fakeSSHBin(t, script))
	return m
}

func TestSSHApplyWiresClients(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	if !m.ssh.IsRemote() || m.sys.Remote.Target != "user@remote" || m.lc.Remote.Target != "user@remote" {
		t.Fatalf("clients not pointed at remote: %+v", m.ssh)
	}
	if m.pm.Runner.Target != "user@remote" || m.pl.Runner.Target != "user@remote" {
		t.Error("podman/podlet clients must follow --ssh")
	}
	if !m.ssh.Multiplex {
		t.Error("--ssh sessions share one connection by default")
	}
}

// TestSessionSwitchKeepsInFlightClients: switching targets builds new
// clients; commands that captured the old ones keep their session.
func TestSessionSwitchKeepsInFlightClients(t *testing.T) {
	m := New()
	before, epoch := m.sys, m.epoch
	m.applySSH("user@remote")
	if before.Remote.Target != "" {
		t.Error("the previous session's systemd client must not be re-pointed")
	}
	if m.sys == before || m.epoch == epoch {
		t.Error("a switch must build new clients and bump the epoch")
	}
	stale := refreshMsg{units: []quadlet.Unit{{Name: "old", UnitName: "old.service"}}, images: []string{""}, epoch: epoch}
	model, _ := m.Update(stale)
	if len(model.(Model).units) != 0 {
		t.Error("a refresh of the previous session must be dropped")
	}
}

func TestDropinNamesParse(t *testing.T) {
	got := dropinNames([]byte("zz.conf\nnotes.txt\nsubdir/\n10-a.conf\n"))
	want := []string{"10-a.conf", "zz.conf"}
	if len(got) != len(want) {
		t.Fatalf("dropinNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dropinNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(dropinNames(nil)) != 0 {
		t.Error("empty listing must yield none")
	}
}

func dropinFakeBin(t *testing.T, lsOut, lsExit string) string {
	t.Helper()
	script := "#!/bin/sh\n" +
		"for last; do :; done\n" +
		"cmd=${last%% *}\n" +
		"if [ \"$cmd\" = \"ls\" ]; then printf '" + lsOut + "'; exit " + lsExit + "; fi\n" +
		"if [ \"$cmd\" = \"cat\" ]; then printf '[Container]\\nEnvironment=A=1\\n'; exit 0; fi\n" +
		"exit 1\n"
	return fakeSSHBin(t, script)
}

func TestFileContentRemoteDropins(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	m.ssh.SSHBin = dropinFakeBin(t, "10-a.conf\\nnotes.txt\\nsubdir/\\n", "0")
	u := quadlet.Unit{Name: "web-api", Kind: quadlet.KindContainer, Path: "/r/web-api.container"}
	out := m.fileContent(u, []byte("[Container]\nImage=nginx"))
	if !strings.Contains(out, "Image=nginx") {
		t.Errorf("base content lost: %q", out)
	}
	if !strings.Contains(out, "drop-in: container.d/10-a.conf") || !strings.Contains(out, "Environment=A=1") {
		t.Errorf("remote drop-ins not merged: %q", out)
	}
	if strings.Contains(out, "notes.txt") || strings.Contains(out, "subdir") {
		t.Errorf("non-conf entries must be filtered: %q", out)
	}
}

func TestFileContentRemoteDropinsNone(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	m.ssh.SSHBin = dropinFakeBin(t, "", "0")
	u := quadlet.Unit{Name: "web", Kind: quadlet.KindContainer, Path: "/r/web.container"}
	if out := m.fileContent(u, []byte("base")); out != "base" {
		t.Errorf("listed-but-empty must render base only, got %q", out)
	}
}

func TestFileContentRemoteDropinsFailure(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	m.ssh.SSHBin = dropinFakeBin(t, "", "1")
	u := quadlet.Unit{Name: "web", Kind: quadlet.KindContainer, Path: "/r/web.container"}
	out := m.fileContent(u, []byte("base"))
	if !strings.Contains(out, "drop-ins not listed") {
		t.Errorf("failed enumeration must keep the note, got %q", out)
	}
}

func TestSSHRefusesFileWrites(t *testing.T) {
	m := sshModel(t)
	m = withUnits(m, "webapp")
	m.table.SetCursor(0)
	for _, key := range []string{"e", "d", "E", "D", "n"} {
		model, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		mm := model.(Model)
		if mm.pending != nil || mm.generating || mm.instancing || mm.pickingEditor {
			t.Errorf("key %q must not start file work over SSH", key)
		}
		if !strings.Contains(mm.statusLine, "SSH") {
			t.Errorf("key %q must explain SSH limits, got %q", key, mm.statusLine)
		}
	}
}

// TestSSHInstantiateAllowed: instantiating a template only starts a unit
// (no file is written), so it works over SSH like any lifecycle action.
func TestSSHInstantiateAllowed(t *testing.T) {
	m := sshModel(t)
	m = withUnits(m, "web@")
	m.table.SetCursor(0)
	model, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if !model.(Model).instancing {
		t.Errorf("instantiate must open its prompt over SSH: %q", model.(Model).statusLine)
	}
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
}

func TestSSHTitleShowsTarget(t *testing.T) {
	m := New()
	m.applySSH("user@remote")
	if !strings.Contains(m.View().Content, "user@remote") {
		t.Errorf("title must show the remote target: %q", m.View().Content)
	}
}

func TestUpdatesCmdUsesRemotePodman(t *testing.T) {
	script := "#!/bin/sh\n" +
		"if echo \"$@\" | grep -q 'is-enabled'; then echo 'enabled'; exit 0; fi\n" +
		"if echo \"$@\" | grep -q 'is-active'; then echo 'active'; exit 0; fi\n" +
		"if echo \"$@\" | grep -q 'auto-update --dry-run'; then echo '[{\"Container\":\"c\",\"Image\":\"img\",\"Policy\":\"registry\",\"Unit\":\"fake-test.service\",\"Updated\":\"true\"}]'; exit 0; fi\n" +
		"echo ok; exit 0\n"
	m := New()
	m.applySSH("user@remote")
	useFakeSSH(&m, fakeSSHBin(t, script))
	msg := updatesCmd(m.sys, m.pm)()
	um, ok := msg.(updatesMsg)
	if !ok {
		t.Fatalf("expected updatesMsg, got %T", msg)
	}
	if um.err != nil {
		t.Fatalf("updatesCmd error: %v", um.err)
	}
	if len(um.entries) != 1 || um.entries[0].Unit != "fake-test.service" {
		t.Errorf("entries must come from remote podman, got %+v", um.entries)
	}
	if um.timerEnabled != "enabled" || um.timerActive != "active" {
		t.Errorf("timer = %q/%q, want enabled/active", um.timerEnabled, um.timerActive)
	}
}

func TestSSHRefreshCmd(t *testing.T) {
	m := sshModel(t)
	msg := m.refreshReq(enrichNone).cmd()()
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
}
