package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeEventsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeEvents {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.stopEvents()
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "f":
		m.following = !m.following
		if m.following {
			m.viewport.GotoBottom()
		}
		return m, nil, true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true

}
