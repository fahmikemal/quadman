package ui

import (
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeSecretsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeSecrets {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "r":
		return m, tea.Batch(m.setBusy("podman secrets"), secretsCmd()), true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true
}
