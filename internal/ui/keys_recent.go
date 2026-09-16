package ui

import (
	tea "charm.land/bubbletea/v2"
)

// Recent-actions screen (A): read-only log of finished actions.

func (m Model) modeRecentKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeRecent {
		return m, nil, false
	}
	if msg.String() == "esc" || msg.String() == "q" {
		m.mode = modeList
		m.resize()
		return m, nil, true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true
}

// openRecentKeys switches to the recent-actions screen.
func (m Model) openRecentKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "A" {
		return m, nil, false
	}
	m.mode = modeRecent
	m.viewport.SetContent(m.actionsView())
	m.viewport.GotoTop()
	m.resize()
	return m, nil, true
}
