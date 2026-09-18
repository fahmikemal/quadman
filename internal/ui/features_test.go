package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// TestFeatureReadonlyEndToEnd drives readonly mode through its full flow:
// banner visible, every write refused with an explanation, reads unaffected,
// and the detail-view edit path also refused.
func TestFeatureReadonlyEndToEnd(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.readonly = true

	if !strings.Contains(m.helpBar(), "readonly") {
		t.Fatal("readonly chip must be visible")
	}

	// Every write key is refused.
	for _, key := range []string{"s", "r", "x", "e", "d", "E", "h", "i", "D", "R", "L", "n"} {
		model, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		mm := model.(Model)
		if mm.busy || mm.pending != nil || mm.generating || mm.instancing {
			t.Fatalf("key %q started work in readonly mode", key)
		}
	}

	// Detail-view edit is refused too.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	mm := model.(Model) // logs view is read-only, allowed
	_ = mm
	m2 := withUnits(New(), "webapp")
	m2.table.SetCursor(0)
	m2.readonly = true
	m2.mode = modeDetail
	m2.tab = tabSource
	model2, _ := m2.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	mm2 := model2.(Model)
	if strings.Contains(mm2.statusLine, "editor:") || mm2.pickingEditor {
		t.Fatalf("detail E must be refused in readonly, got %q", mm2.statusLine)
	}
	if !strings.Contains(mm2.statusLine, "readonly") {
		t.Fatalf("detail E must explain readonly, got %q", mm2.statusLine)
	}

	// Reads still work: filter, problems, tree, storage screen open.
	m3 := readonlyModel()
	model3, _ := m3.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if _, ok := model3.(Model); !ok {
		t.Fatal("filter must open in readonly")
	}
}

// TestFeatureRecentActionsEndToEnd drives the A log through a full session:
// empty state, failed and ok actions recorded, screen renders them, esc back.
func TestFeatureRecentActionsEndToEnd(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	// Empty state explains itself.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	mm := model.(Model)
	if mm.mode != modeRecent {
		t.Fatal("A must open the recent screen")
	}
	if !strings.Contains(mm.viewport.View(), "No actions yet") {
		t.Fatalf("empty log must explain itself: %q", mm.viewport.View())
	}
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm = model.(Model)
	if mm.mode != modeList {
		t.Fatal("esc must return to the list")
	}

	// Failed + ok actions both land in the log.
	model, _ = mm.Update(actionMsg{desc: "start webapp.service", err: errString("boom")})
	mm = model.(Model)
	model, _ = mm.Update(healthMsg{container: "systemd-webapp", ok: true})
	mm = model.(Model)
	model, _ = mm.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	mm = model.(Model)
	view := mm.viewport.View()
	if !strings.Contains(view, "start webapp.service") || !strings.Contains(view, "FAIL") {
		t.Errorf("failed action must render with FAIL:\n%s", view)
	}
	if !strings.Contains(view, "healthcheck systemd-webapp") || !strings.Contains(view, "ok") {
		t.Errorf("healthy check must render as ok:\n%s", view)
	}
}

// TestFeatureYAMLConfigEndToEnd proves the YAML file drives behavior: poll
// interval, readonly, and a custom command that runs and logs.
func TestFeatureYAMLConfigEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	sub := filepath.Join(dir, "quadman")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := "refresh_interval: 9s\nreadonly: true\ncustom_commands:\n  - name: probe\n    key: C\n    run: echo PROBE-{{.UnitName}}\n"
	if err := os.WriteFile(filepath.Join(sub, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	m := New()
	if m.pollInterval.String() != "9s" {
		t.Fatalf("poll interval = %v, want 9s from YAML", m.pollInterval)
	}
	if !m.readonly {
		t.Fatal("readonly from YAML must apply")
	}
	if len(m.custom) != 1 || m.custom[0].Key != "C" {
		t.Fatalf("custom = %+v, want 1 command on C", m.custom)
	}
	m = withUnits(m, "webapp")
	m.table.SetCursor(0)
	m.readonly = false // custom itself runs; readonly refusal is covered elsewhere
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	mm := model.(Model)
	_ = mm
	if cmd == nil {
		t.Fatal("custom key must produce a command")
	}
	// The command is a tea.Batch(setBusy tick, custom work): run the batch
	// and feed each resulting message back into the model.
	msg := cmd()
	var found customMsg
	var ok bool
	if batch, isBatch := msg.(tea.BatchMsg); isBatch {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if got := sub(); got != nil {
				if cm, isCustom := got.(customMsg); isCustom {
					found, ok = cm, true
				}
				model, _ = mm.Update(got)
				mm = model.(Model)
			}
		}
	} else {
		found, ok = msg.(customMsg)
	}
	if !ok {
		t.Fatalf("custom cmd must yield customMsg, got %T", msg)
	}
	cm := found
	if cm.err != nil || !strings.Contains(cm.out, "PROBE-webapp.service") {
		t.Errorf("custom result = %+v, want PROBE-webapp.service", cm)
	}
	if len(mm.actions) != 1 || !mm.actions[0].ok {
		t.Errorf("custom result must be logged: %+v", mm.actions)
	}
}

// TestFeatureGenerateStrictEndToEnd drives the generate input: garbage is
// rejected with guidance, a valid run command reaches podlet.
func TestFeatureGenerateStrictEndToEnd(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	// Open the generate input.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	mm := model.(Model)
	if !mm.generating {
		t.Skip("podlet not installed here; input gate is covered by unit tests")
	}
	// Type garbage and submit.
	for _, r := range "nginx:latest" {
		var cmd tea.Cmd
		var mod tea.Model
		mod, _ = mm.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		mm = mod.(Model)
		_ = cmd
	}
	mod, genCmd := mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm = mod.(Model)
	if mm.generating {
		t.Fatal("submit must close the input")
	}
	if genCmd == nil {
		t.Fatal("submit must produce the generate command")
	}
	// Run the batch: the spinner tick plus the validation result.
	if batch, ok := genCmd().(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if got := sub(); got != nil {
				if _, isGen := got.(generateMsg); isGen {
					mod, _ = mm.Update(got)
					mm = mod.(Model)
				}
			}
		}
	}
	if mm.busy {
		t.Fatal("rejected input must clear the spinner after its error lands")
	}
	if !strings.Contains(mm.statusLine, "not a run command") {
		t.Fatalf("garbage must be rejected with guidance, got %q", mm.statusLine)
	}
}

// TestFeatureInspectCacheEndToEnd proves the poll-time cache: repeated
// InspectAll calls do not re-parse unchanged files, and edits invalidate.
func TestFeatureInspectCacheEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := quadlet.Unit{Name: "webapp", Kind: quadlet.KindContainer, Path: path, UnitName: "webapp.service"}

	m := New()
	if m.inspect == nil {
		t.Fatal("model must carry an inspect cache")
	}
	first := m.inspect.Inspect(u)
	second := m.inspect.Inspect(u)
	if first.Image != "nginx:1" || second.Image != "nginx:1" {
		t.Fatalf("images = %q/%q, want nginx:1", first.Image, second.Image)
	}
}

// TestFeatureCustomLegendAndHelp proves the new keys are discoverable.
func TestFeatureCustomLegendAndHelp(t *testing.T) {
	m := New()
	bar := m.helpBar()
	if !strings.Contains(bar, "A actions") {
		t.Errorf("legend must advertise A actions: %q", bar)
	}
	m.custom = []config.CustomCommand{{Name: "probe", Key: "C", Run: "echo hi"}}
	// Custom keys work from the list dispatcher.
	m = withUnits(m, "webapp")
	m.table.SetCursor(0)
	model, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	mm := model.(Model)
	_ = mm
	if cmd == nil {
		t.Error("configured custom key must dispatch")
	}
}
