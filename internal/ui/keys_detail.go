package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeDetailKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeDetail {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.stopLogs()
		m.clearSearch()
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "[":
		mm, cmd := m.cycleTab(-1)
		return mm, cmd, true
	case "]":
		mm, cmd := m.cycleTab(1)
		return mm, cmd, true
	case "l":
		if m.tab != tabJournal {
			mm, cmd := m.cycleTab(tabJournal - m.tab)
			return mm, cmd, true
		}
		return m, nil, true
	case "f":
		if m.tab == tabJournal && m.sess != nil {
			m.following = !m.following
			if m.following {
				m.viewport.GotoBottom()
			}
		}
		return m, nil, true
	case "/":
		if m.tab == tabJournal {
			m.searching = true
			m.searchIn.Focus()
			return m, textinput.Blink, true
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd, true
	case "n":
		if m.tab == tabJournal {
			m.nextMatch()
		}
		return m, nil, true
	case "N":
		if m.tab == tabJournal {
			m.prevMatch()
		}
		return m, nil, true
	case "E":
		if m.refuseRemoteWrite() {
			return m, nil, true
		}
		if m.refuseReadonly() {
			return m, nil, true
		}
		mm, cmd := m.editSelected()
		return mm, cmd, true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true
}
