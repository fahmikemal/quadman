package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// fakeBin puts an executable shell script called name first on PATH.
func fakeBin(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGenerateLiveObject(t *testing.T) {
	fakeBin(t, "podlet", "echo '# FileName=live'\necho '[Container]'\n")
	out, err := generatePodlet(context.Background(), "container web")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "# FileName=live") {
		t.Errorf("output = %q", out)
	}
}

func TestGenerateLiveObjectNeedsName(t *testing.T) {
	for _, in := range []string{"container", "volume a b", "service web"} {
		if _, err := generatePodlet(context.Background(), in); err == nil {
			t.Errorf("%q should fail", in)
		} else if !strings.Contains(err.Error(), "kind") {
			t.Errorf("%q error should mention <kind>: %v", in, err)
		}
	}
}

func TestPrunePrompt(t *testing.T) {
	if got := prunePrompt(nil); !strings.Contains(got, "[y/N]") || strings.Contains(got, "inactive") {
		t.Errorf("empty prompt = %q", got)
	}
	got := prunePrompt([]string{"db.service"})
	if !strings.Contains(got, "1 inactive") || !strings.Contains(got, "db.service") {
		t.Errorf("single prompt = %q", got)
	}
	many := make([]string, 0, 10)
	for i := range 10 {
		many = append(many, string(rune('a'+i))+".service")
	}
	got = prunePrompt(many)
	if !strings.Contains(got, "10 inactive") || !strings.Contains(got, "+ 2 more") {
		t.Errorf("truncated prompt = %q", got)
	}
}

func TestInactiveQuadletUnits(t *testing.T) {
	m := withUnits(New(), "web", "db", "cache")
	m.status = map[string]systemd.Status{
		"web.service":   {ActiveState: "active"},
		"db.service":    {ActiveState: "inactive"},
		"cache.service": {ActiveState: "failed"},
	}
	got := m.inactiveQuadletUnits()
	if len(got) != 2 || got[0] != "cache.service" || got[1] != "db.service" {
		t.Errorf("inactive = %v, want sorted [cache db]", got)
	}
}

func TestCustomKeyConflicts(t *testing.T) {
	got := customKeyConflicts([]config.CustomCommand{
		{Name: "prune2", Key: "P"},
		{Name: "mine", Key: "G"},
	})
	if len(got) != 1 || !strings.Contains(got[0], "P") {
		t.Errorf("conflicts = %v, want [prune2 (P)]", got)
	}
	if len(customKeyConflicts(nil)) != 0 {
		t.Error("nil customs must not conflict")
	}
}

func TestInactiveSkipsUnknown(t *testing.T) {
	m := withUnits(New(), "cold")
	if got := m.inactiveQuadletUnits(); len(got) != 0 {
		t.Errorf("pre-refresh units must not count: %v", got)
	}
}

func TestPruneKeysArmsConfirmation(t *testing.T) {

	fakeBin(t, "podman", "exit 0\n")
	m := withUnits(New(), "db")
	m.status = map[string]systemd.Status{"db.service": {ActiveState: "inactive"}}
	model, _, ok := m.pruneKeys(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if !ok {
		t.Fatal("P should be handled")
	}
	mm := model.(Model)
	if mm.pending == nil || mm.pending.verb != "prune" {
		t.Fatalf("pending = %+v, want prune", mm.pending)
	}
	if !strings.Contains(mm.statusLine, "db.service") || !strings.Contains(mm.statusLine, "[y/N]") {
		t.Errorf("status = %q", mm.statusLine)
	}
}
func TestExecOpenVolumeNoPrefill(t *testing.T) {
	m := New()
	model, _ := m.Update(refreshMsg{units: []quadlet.Unit{{Name: "d", Kind: quadlet.KindVolume, UnitName: "d-volume.service"}}, images: []string{""}, lingerOK: true})
	mm := model.(Model)
	model, _, ok := mm.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if !ok {
		t.Fatal("X should be handled")
	}
	if got := model.(Model).execIn.Value(); got != "" {
		t.Errorf("non-container prefill = %q, want empty", got)
	}
}

func TestPruneKeysRefused(t *testing.T) {
	m := withUnits(New(), "db")
	m.readonly = true
	if _, _, ok := m.pruneKeys(tea.KeyPressMsg{Code: 'P', Text: "P"}); !ok {
		t.Fatal("P should be handled even when refused")
	} else if m.pending != nil {
		t.Error("readonly must not arm prune")
	}
	m2 := withUnits(New(), "db")
	m2.ssh = remote.Runner{Target: "host"}
	model, _, _ := m2.pruneKeys(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if model.(Model).pending != nil {
		t.Error("remote must not arm prune")
	}
}

func TestRunPendingPrune(t *testing.T) {
	fakeBin(t, "podman", "echo 'Total reclaimed space: 100MB'\n")
	m := New()
	_, cmd := m.runPending(&pendingAction{verb: "prune"})
	if cmd == nil {
		t.Fatal("prune should produce a command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("msg = %T, want tea.BatchMsg", msg)
	}
	found := false
	for _, c := range batch {
		if am, ok := c().(actionMsg); ok {
			found = true
			if am.err != nil || !strings.Contains(am.out, "100MB") {
				t.Errorf("result = %+v", am)
			}
		}
	}
	if !found {
		t.Error("batch should contain an actionMsg")
	}
}

func TestStatsMsgOpensMode(t *testing.T) {
	m := New()
	model, _ := m.Update(statsMsg{content: "NAME\nweb\n"})
	if model.(Model).mode != modeStats {
		t.Error("statsMsg should switch to modeStats")
	}
	model, _ = m.Update(statsMsg{err: context.DeadlineExceeded})
	if !strings.Contains(model.(Model).statusLine, "stats:") {
		t.Errorf("status = %q", model.(Model).statusLine)
	}
}

func TestExecOpenPrefillsBase(t *testing.T) {
	m := withUnits(New(), "webapp")
	model, _, ok := m.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if !ok {
		t.Fatal("X should be handled")
	}
	mm := model.(Model)
	if !mm.execing || mm.execIn.Value() != "systemd-webapp" {
		t.Errorf("execing=%v value=%q, want systemd-webapp", mm.execing, mm.execIn.Value())
	}
}

func TestExecOpenRefused(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.readonly = true
	model, _, _ := m.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if model.(Model).execing {
		t.Error("readonly must not arm exec")
	}
	m2 := withUnits(New(), "webapp")
	m2.ssh = remote.Runner{Target: "host"}
	model, _, _ = m2.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
	mm := model.(Model)
	if mm.execing || !strings.Contains(mm.statusLine, "SSH") {
		t.Errorf("remote must refuse with explanation: %q", mm.statusLine)
	}
	m3 := withUnits(New(), "webapp")
	m3.noEditor = true
	model, _, _ = m3.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
	if model.(Model).execing {
		t.Error("served sessions must not arm exec")
	}
}

func TestExecResolveNotFoundKeepsPrompt(t *testing.T) {
	m := New()
	model, cmd := m.Update(execResolveMsg{name: "ghost", found: false})
	mm := model.(Model)
	if !mm.execing || !strings.Contains(mm.statusLine, "not found") {
		t.Errorf("execing=%v status=%q", mm.execing, mm.statusLine)
	}
	if cmd != nil {
		t.Error("not-found must not hand over the terminal")
	}
}

func TestExecBaseName(t *testing.T) {
	if execBaseName("webapp.service") != "webapp" || execBaseName("a") != "a" {
		t.Error("base name derivation wrong")
	}
}

func TestKubeYamlMissingHint(t *testing.T) {
	dir := t.TempDir()
	kube := filepath.Join(dir, "stack.kube")
	if err := os.WriteFile(kube, []byte("[Kube]\nYaml=/nonexistent-quadman/missing.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.units = []quadlet.Unit{{Name: "stack", Kind: quadlet.KindKube, UnitName: "stack-kube.service", Path: kube}}
	found := false
	for _, h := range m.smartHints() {
		if strings.Contains(h, "does not exist") && strings.Contains(h, "stack-kube.service") {
			found = true
		}
	}
	if !found {
		t.Errorf("hints = %v, want missing-Yaml hint", m.smartHints())
	}
}

func TestKubeYamlPresentNoHint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pod.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	kube := filepath.Join(dir, "stack.kube")
	if err := os.WriteFile(kube, []byte("[Kube]\nYaml=pod.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.units = []quadlet.Unit{{Name: "stack", Kind: quadlet.KindKube, UnitName: "stack-kube.service", Path: kube}}
	for _, h := range m.smartHints() {
		if strings.Contains(h, "does not exist") {
			t.Errorf("present Yaml must not hint: %q", h)
		}
	}
}

func TestKubeYamlAccessor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	kube := filepath.Join(dir, "s.kube")
	if err := os.WriteFile(kube, []byte("[Kube]\nYaml=./p.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	u := quadlet.Unit{Name: "s", Kind: quadlet.KindKube, UnitName: "s-kube.service", Path: kube}
	f, err := m.parseUnitFile(u)
	if err != nil {
		t.Fatal(err)
	}
	if f.KubeYaml() != "./p.yaml" {
		t.Errorf("KubeYaml = %q", f.KubeYaml())
	}
	if !kubeYamlExists(u, "./p.yaml") {
		t.Error("relative Yaml should resolve against the unit dir")
	}
	if kubeYamlExists(u, "./missing.yaml") {
		t.Error("missing Yaml must not resolve")
	}
}

func TestPullImageAlreadyPresent(t *testing.T) {
	fakeBin(t, "podman", "if [ \"$1 $2\" = \"image exists\" ]; then exit 0; fi\necho 'must not pull'; exit 9\n")
	msg := pullImageCmd("busybox:latest")()
	am, ok := msg.(actionMsg)
	if !ok {
		t.Fatalf("msg = %T", msg)
	}
	if am.err != nil || !strings.Contains(am.out, "already in local storage") {
		t.Errorf("result = %+v", am)
	}
}

func TestPullImageFetches(t *testing.T) {
	fakeBin(t, "podman", "if [ \"$1 $2\" = \"image exists\" ]; then exit 1; fi\necho 'Copied blob sha256:abc'\n")
	msg := pullImageCmd("busybox:latest")()
	am, ok := msg.(actionMsg)
	if !ok {
		t.Fatalf("msg = %T", msg)
	}
	if am.err != nil || !strings.Contains(am.out, "sha256:abc") {
		t.Errorf("result = %+v", am)
	}
}

func TestUnitImage(t *testing.T) {
	m := withUnits(New(), "web")
	if got := m.unitImage(); got != "" {
		t.Errorf("pre-poll image = %q, want empty", got)
	}
	m.images["web.service"] = "busybox:latest"
	if got := m.unitImage(); got != "busybox:latest" {
		t.Errorf("image = %q", got)
	}
}

func TestPalettePullImageGating(t *testing.T) {
	m := withUnits(New(), "web")
	actions := m.buildPaletteActions()
	find := func(id string) *paletteAction {
		for i := range actions {
			if actions[i].id == id {
				return &actions[i]
			}
		}
		return nil
	}
	pull := find("pull-image")
	if pull == nil {
		t.Fatal("pull-image action missing")
	}
	if ok, _ := pull.enabled(m); ok {
		t.Error("unknown image must disable pull")
	}
	m.images["web.service"] = "localhost/app:latest"
	if ok, reason := pull.enabled(m); ok || reason != "locally built" {
		t.Errorf("local image must disable pull (ok=%v reason=%q)", ok, reason)
	}
	m.images["web.service"] = "docker.io/library/busybox:latest"
	if ok, reason := pull.enabled(m); !ok {
		t.Errorf("registry image must enable pull: %q", reason)
	}
	m.readonly = true
	if ok, _ := pull.enabled(m); ok {
		t.Error("readonly must disable pull")
	}
	for _, id := range []string{"exec", "stats", "prune"} {
		if find(id) == nil {
			t.Errorf("palette action %q missing", id)
		}
	}
}

func TestAutoUpdateLocalHint(t *testing.T) {
	dir := t.TempDir()
	kube := filepath.Join(dir, "c.container")
	if err := os.WriteFile(kube, []byte("[Container]\nImage=busybox\nAutoUpdate=local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.timerEnabled = "disabled"
	m.units = []quadlet.Unit{{Name: "c", Kind: quadlet.KindContainer, UnitName: "c.service", Path: kube}}
	found := false
	for _, h := range m.smartHints() {
		if strings.Contains(h, "AutoUpdate=local") {
			found = true
		}
	}
	if !found {
		t.Errorf("hints = %v, want local-policy hint", m.smartHints())
	}
}

func TestAutoUpdateKubeHint(t *testing.T) {
	dir := t.TempDir()
	kube := filepath.Join(dir, "s.kube")
	if err := os.WriteFile(kube, []byte("[Kube]\nYaml=pod.yaml\nAutoUpdate=registry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pod.yaml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.timerEnabled = ""
	m.units = []quadlet.Unit{{Name: "s", Kind: quadlet.KindKube, UnitName: "s-kube.service", Path: kube}}
	foundYaml, foundAuto := false, false
	for _, h := range m.smartHints() {
		if strings.Contains(h, "does not exist") {
			foundYaml = true
		}
		if strings.Contains(h, "AutoUpdate=registry") {
			foundAuto = true
		}
	}
	if foundYaml {
		t.Error("present Yaml must not hint")
	}
	if !foundAuto {
		t.Errorf("hints = %v, want kube AutoUpdate hint", m.smartHints())
	}
}
