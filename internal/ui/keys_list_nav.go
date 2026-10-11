package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// List navigation keys: quit, help, filter, file view, logs.

func (m Model) quitKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "q" {
		return m, nil, false
	}
	return m, tea.Quit, true

}

func (m Model) escKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "esc" {
		return m, nil, false
	}
	if m.filterStr != "" {
		m.filterStr = ""
		m.filterIn.SetValue("")
		m.refilter()
		return m, nil, true
	}
	if len(m.marks) > 0 {
		m.clearMarks()
		m.setStatus("marks cleared", false)
		m.buildRows()
	}
	return m, nil, true

}

func (m Model) helpKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "?" {
		return m, nil, false
	}
	m.showHelp = !m.showHelp
	m.resize()
	return m, nil, true

}

func (m Model) filterOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "/" {
		return m, nil, false
	}
	m.filtering = true
	m.filterIn.Focus()
	return m, textinput.Blink, true

}

func (m Model) openLogsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "l" {
		return m, nil, false
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	mm, cmd := m.startLogs(u)
	return mm, cmd, true

}

func (m Model) openFileKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "enter" {
		return m, nil, false
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	content, err := m.readUnitFile(u)
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil, true
	}
	m.viewport.SetContent(m.fileContent(u, content))
	m.viewport.GotoTop()
	m.mode = modeDetail
	m.tab = tabSource
	m.resize()
	return m, nil, true
}

// sortKeys cycles the list order: problems first → name → kind.
func (m Model) sortKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "S" || m.mode != modeList {
		return m, nil, false
	}
	m.sortBy = (m.sortBy + 1) % sortCount
	m.refilter()
	m.setStatus("sorted: "+m.sortBy.String(), false)
	return m, nil, true
}

// dashboardKeys toggles the dashboard tiles and attention block (z).
func (m Model) dashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "z" || m.mode != modeList {
		return m, nil, false
	}
	m.dashOff = !m.dashOff
	m.resize()
	return m, nil, true
}
