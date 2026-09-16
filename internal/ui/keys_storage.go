package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeStorageKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeStorage {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "r":
		return m, tea.Batch(m.setBusy("system df"), storageCmd()), true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true

}
