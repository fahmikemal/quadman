package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func (m Model) modeDetailKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeDetail {
		return m, nil, false
	}

	// Active log filtering input mode
	if m.tab == tabJournal && m.filteringLogs {
		switch msg.String() {
		case "enter":
			m.filteringLogs = false
			m.logFilterIn.Blur()
			m.logFilter = strings.TrimSpace(m.logFilterIn.Value())
			m.refreshLogContent()
			return m, nil, true
		case "esc":
			m.filteringLogs = false
			m.logFilterIn.Blur()
			m.logFilterIn.SetValue("")
			m.logFilter = ""
			m.refreshLogContent()
			return m, nil, true
		default:
			var cmd tea.Cmd
			m.logFilterIn, cmd = m.logFilterIn.Update(msg)
			m.logFilter = strings.TrimSpace(m.logFilterIn.Value())
			m.refreshLogContent()
			return m, cmd, true
		}
	}

	switch msg.String() {
	case "esc", "q":
		if m.tab == tabJournal && m.logFilter != "" {
			m.logFilter = ""
			m.logFilterIn.SetValue("")
			m.refreshLogContent()
			m.setStatus("log filter cleared", false)
			return m, nil, true
		}
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
	case "F":
		if m.tab == tabJournal {
			m.filteringLogs = true
			m.logFilterIn.Focus()
			return m, textinput.Blink, true
		}
		return m, nil, true
	case "p":
		if m.tab == tabJournal {
			mm, cmd := m.cycleLogPriority()
			return mm, cmd, true
		}
		return m, nil, true
	case "S", "ctrl+s":
		if m.tab == tabJournal {
			mm, cmd := m.exportLogs("txt")
			return mm, cmd, true
		}
		return m, nil, true
	case "c":
		if m.tab == tabJournal {
			mm, cmd := m.exportLogs("clipboard")
			return mm, cmd, true
		}
		return m, nil, true
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
