package ui

import (
	tea "charm.land/bubbletea/v2"
)

// statsOpenKeys opens the resource screen (o). Read-only by nature: no
// readonly/remote gates, like storage (g) and events (w).
func (m Model) statsOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "o" {
		return m, nil, false
	}
	return m, tea.Batch(m.setBusy("stats"), statsCmd()), true
}
