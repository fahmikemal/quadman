package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/systemd"
)

func TestPaletteOpenAndClose(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.table.SetCursor(0)

	// Press Ctrl+P to open
	model, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm := model.(Model)
	if mm.mode != modePalette {
		t.Fatalf("mode = %d, want modePalette (%d)", mm.mode, modePalette)
	}

	// Verify rendering contains Command Palette header
	view := mm.View().Content
	if !strings.Contains(view, "COMMAND PALETTE") {
		t.Errorf("view missing palette header: %q", view)
	}

	// Press Esc to close
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	mm = model.(Model)
	if mm.mode != modeList {
		t.Fatalf("mode = %d, want modeList (%d) after esc", mm.mode, modeList)
	}

	// Press Ctrl+P to toggle open then toggle close
	model, _ = mm.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm = model.(Model)
	if mm.mode != modePalette {
		t.Fatalf("mode = %d, want modePalette", mm.mode)
	}
	model, _ = mm.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm = model.(Model)
	if mm.mode != modeList {
		t.Fatalf("mode = %d, want modeList after toggling ctrl+p", mm.mode)
	}
}

func TestPaletteFuzzyFilter(t *testing.T) {
	m := withUnits(New(), "web-app")
	model, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm := model.(Model)

	// Initially all actions are listed
	if len(mm.paletteFiltered) < 15 {
		t.Fatalf("initial actions count = %d, want >= 15", len(mm.paletteFiltered))
	}

	// Type "reload" to filter
	for _, ch := range "reload" {
		model, _ = mm.Update(tea.KeyPressMsg{Code: rune(ch), Text: string(ch)})
		mm = model.(Model)
	}

	if len(mm.paletteFiltered) == 0 {
		t.Fatal("filtering 'reload' gave 0 actions")
	}

	found := false
	for _, act := range mm.paletteFiltered {
		if act.id == "reload" {
			found = true
			break
		}
	}
	if !found {
		t.Error("filtered actions missing 'reload'")
	}
}

func TestPaletteExecuteAction(t *testing.T) {
	m := withUnits(New(), "web-app")
	model, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm := model.(Model)

	// Filter down to reload
	for _, ch := range "reload" {
		model, _ = mm.Update(tea.KeyPressMsg{Code: rune(ch), Text: string(ch)})
		mm = model.(Model)
	}

	// Press Enter to execute the selected action
	model, cmd := mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	mm = model.(Model)

	if mm.mode != modeList {
		t.Errorf("executing action should return to previous mode, got %d", mm.mode)
	}
	if cmd == nil && !mm.busy {
		t.Error("executing reload should dispatch cmd or set busy")
	}
}

func TestContextSensitiveStartStop(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.table.SetCursor(0)
	m.status = make(map[string]systemd.Status)

	// Case 1: Unit is already active -> 's' warns instead of running start
	m.status["web-app.service"] = systemd.Status{
		Id:          "web-app.service",
		ActiveState: "active",
		SubState:    "running",
	}

	model, _ := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	mm := model.(Model)
	if !strings.Contains(mm.statusLine, "already active") {
		t.Errorf("pressing s on active unit should report already active: %q", mm.statusLine)
	}

	// Case 2: Unit is already inactive -> 'x' warns instead of arming stop
	m.status["web-app.service"] = systemd.Status{
		Id:          "web-app.service",
		ActiveState: "inactive",
		SubState:    "dead",
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	mm = model.(Model)
	if !strings.Contains(mm.statusLine, "already stopped") {
		t.Errorf("pressing x on inactive unit should report already stopped: %q", mm.statusLine)
	}
	if mm.pending != nil {
		t.Error("stopping inactive unit should not arm pending action")
	}

	// Case 3: Unit not-found -> 's' warns to daemon-reload
	m.status["web-app.service"] = systemd.Status{
		Id:        "web-app.service",
		LoadState: "not-found",
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	mm = model.(Model)
	if !strings.Contains(mm.statusLine, "not found") || !strings.Contains(mm.statusLine, "daemon-reload") {
		t.Errorf("starting not-found unit should hint at daemon-reload: %q", mm.statusLine)
	}
}

func TestPaletteDisabledActionRefusal(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.readonly = true
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "ctrl+p"})
	mm := model.(Model)

	// Filter down to start
	for _, ch := range "start unit" {
		model, _ = mm.Update(tea.KeyPressMsg{Code: rune(ch), Text: string(ch)})
		mm = model.(Model)
	}

	if len(mm.paletteFiltered) == 0 {
		t.Fatal("filtering 'start unit' gave 0 results")
	}

	// Press Enter to execute
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	mm = model.(Model)

	// In readonly mode, it should refuse with explanation
	if !strings.Contains(mm.statusLine, "cannot run") || !strings.Contains(mm.statusLine, "readonly") {
		t.Errorf("executing disabled action in palette should report reason: %q", mm.statusLine)
	}
}
