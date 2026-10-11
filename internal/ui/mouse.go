package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// handleMouse processes mouse events. Clicks in the list move the cursor;
// clicks in views scroll. Wheel events are left to the widgets.
// A focused prompt owns the interaction while open, so clicks are ignored
// until it closes; otherwise the prompt's own line would also shift the
// row geometry below it.
func (m Model) handleMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.mouse || m.mode != modeList || m.filtering || m.searching || m.pickingEditor || m.pending != nil ||
		m.generating || m.instancing || m.execing {
		return m, nil
	}
	// Row geometry: top bar, nav strip, the dashboard preamble (which
	// carries any settled filter line), then the table header.
	w, _ := m.size()
	if m.sidePane() && msg.X >= m.tableWidth() {
		return m, nil
	}
	start, end := m.visibleWindow()
	row := start + msg.Y - (2 + lineCount(m.listPreamble(w)) + 1)
	if row < start || row >= end {
		return m, nil
	}
	m.table.SetCursor(row)
	return m, nil
}

// copySelection copies a piece of the selected unit to the clipboard via
// OSC52 (works over SSH).
func (m Model) copySelection(what string) (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	var text string
	switch what {
	case "name":
		text = u.UnitName
	case "image":
		text = m.images[u.UnitName]
	}
	if text == "" {
		m.setStatus("nothing to copy for "+what, false)
		return m, nil
	}
	desc := text
	if len(desc) > 40 {
		desc = desc[:40] + "…"
	}
	m.setStatus(fmt.Sprintf("copied %s", desc), false)
	return m, tea.SetClipboard(text)
}
