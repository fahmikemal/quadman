package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeTreeKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeTree {
		return m, nil, false
	}
	if msg.String() == "esc" || msg.String() == "q" {
		m.mode = modeList
		m.resize()
		return m, nil, true
	}
	var cmd tea.Cmd
	m.treeModel, cmd = m.treeModel.Update(msg)
	return m, cmd, true

}
