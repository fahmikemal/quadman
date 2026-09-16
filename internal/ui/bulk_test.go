package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMarkToggle(t *testing.T) {
	m := withUnits(New(), "alpha", "bravo")
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	mm := model.(Model)
	if !mm.marks["alpha.service"] {
		t.Fatal("space must mark the cursor row")
	}
	if !strings.Contains(mm.table.Rows()[0][0], "*") {
		t.Errorf("marked row must show *: %q", mm.table.Rows()[0][0])
	}

	model, _ = mm.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if len(model.(Model).marks) != 0 {
		t.Error("second space must unmark")
	}
}

func TestBulkStartFlow(t *testing.T) {
	m := withUnits(New(), "alpha", "bravo")
	m.table.SetCursor(0)
	model, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	mm := model.(Model)
	model, _ = mm.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	mm = model.(Model)
	model, _ = mm.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	mm = model.(Model)
	if len(mm.markedUnits()) != 2 {
		t.Fatalf("marked = %d, want 2", len(mm.markedUnits()))
	}

	model, _ = mm.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	mm = model.(Model)
	if mm.pending == nil || mm.pending.verb != "bulk-start" {
		t.Fatalf("s with marks must arm bulk-start, got %+v", mm.pending)
	}
	if !strings.Contains(mm.statusLine, "2 units") {
		t.Errorf("prompt must count units: %q", mm.statusLine)
	}

	model, _ = mm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mm = model.(Model)
	if mm.pending != nil || !mm.busy {
		t.Error("confirming bulk must start the mass action")
	}
}

func TestBulkResultClearsMarks(t *testing.T) {
	m := withUnits(New(), "alpha")
	m.marks = map[string]bool{"alpha.service": true}
	model, _ := m.Update(bulkMsg{desc: "bulk start 1 unit(s)", done: 1, total: 1})
	mm := model.(Model)
	if len(mm.marks) != 0 {
		t.Error("successful bulk must clear marks")
	}
	if !strings.Contains(mm.statusLine, "1/1 ok") {
		t.Errorf("status = %q", mm.statusLine)
	}
	if len(mm.actions) != 1 || !mm.actions[0].ok {
		t.Errorf("bulk must be logged: %+v", mm.actions)
	}
}

func TestBulkResultFailure(t *testing.T) {
	m := withUnits(New(), "alpha", "bravo")
	m.marks = map[string]bool{"alpha.service": true, "bravo.service": true}
	model, _ := m.Update(bulkMsg{desc: "bulk stop 2 unit(s)", done: 1, total: 2, err: errString("failed: bravo.service")})
	mm := model.(Model)
	if len(mm.marks) != 2 {
		t.Error("failed bulk must keep marks for retry")
	}
	if !strings.Contains(mm.statusLine, "1/2") {
		t.Errorf("status must show the count: %q", mm.statusLine)
	}
}

func TestEscClearsMarks(t *testing.T) {
	m := withUnits(New(), "alpha")
	m.marks = map[string]bool{"alpha.service": true}
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm := model.(Model)
	if len(mm.marks) != 0 {
		t.Error("esc must clear marks when no filter is set")
	}
}

func TestMarkReadonly(t *testing.T) {
	m := readonlyModel()
	model, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	mm := model.(Model)
	if len(mm.marks) != 0 {
		t.Error("space must not mark in readonly mode")
	}
}
