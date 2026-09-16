package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// handleMouse processes mouse events. Clicks in the list move the cursor;
// clicks in views scroll. Wheel events are left to the widgets.
func (m Model) handleMouse(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if !m.mouse || m.mode != modeList || m.filtering || m.searching || m.pickingEditor || m.pending != nil {
		return m, nil
	}
	// Row geometry: 1 title line, an optional filter line, 1 table header.
	y := msg.Y - 2
	if m.filtering || m.filterStr != "" {
		y--
	}
	if y < 0 || y >= len(m.filtered) {
		return m, nil
	}
	m.table.SetCursor(y)
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
