package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func clickModel() Model {
	m := withUnits(New(), "alpha", "bravo", "charlie")
	m.mouse = true
	m.table.SetCursor(0)
	return m
}

// Focused list inputs own the interaction while open: a click must not move
// the cursor behind the prompt the user is typing into.
func TestClickIgnoredWhileListInputFocused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Model)
	}{
		{"filtering", func(m *Model) { m.filtering = true }},
		{"generating", func(m *Model) { m.generating = true }},
		{"instancing", func(m *Model) { m.instancing = true }},
		{"execing", func(m *Model) { m.execing = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := clickModel()
			tc.setup(&m)
			model, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 4})
			if got := model.(Model).table.Cursor(); got != 0 {
				t.Errorf("cursor moved to %d while %s prompt is open", got, tc.name)
			}
		})
	}
}

// A settled filter line (input closed, text kept) still shifts the rows down
// by exactly one, so the click must compensate for it.
func TestClickCompensatesSettledFilterLine(t *testing.T) {
	m := clickModel()
	m.filterStr = "a"
	m.refilter()
	// Top bar, nav strip, the preamble (dashboard + settled filter line)
	// and the table header sit above the rows; +1 is the second row.
	w, _ := m.size()
	first := 2 + lineCount(m.listPreamble(w)) + 1
	if !strings.Contains(m.listPreamble(w), "/ ") {
		t.Fatal("the settled filter line must be part of the preamble")
	}
	model, _ := m.Update(tea.MouseClickMsg{X: 5, Y: first + 1})
	mm := model.(Model)
	u, ok := mm.selected()
	if !ok || u.Name != "bravo" {
		t.Errorf("click selected %+v, want bravo", u)
	}
}
