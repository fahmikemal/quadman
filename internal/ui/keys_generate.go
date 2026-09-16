package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeGenerateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeGenerate {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "y":
		if m.refuseReadonly() {
			return m, nil, true
		}
		mm, cmd := m.installGenerated()
		return mm, cmd, true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true

}
