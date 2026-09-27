package ui

import (
	tea "charm.land/bubbletea/v2"
)

// logsPaletteActions follows journals, exports/copies logs, cycles the
// priority filter, and opens the source/detail views.
func (m Model) logsPaletteActions() []paletteAction {
	var actions []paletteAction

	actions = append(actions, paletteAction{
		id:          "logs",
		title:       "Follow Journal Logs",
		shortcut:    "l",
		category:    "Logs",
		description: "Live journalctl -f streaming tail",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			u, ok := m.selected()
			if !ok {
				return m, nil, true
			}
			mm, cmd := m.startLogs(u)
			return mm, cmd, true
		},
	})

	actions = append(actions, paletteAction{
		id:          "export-logs-txt",
		title:       "Export Logs (Text)",
		shortcut:    "S",
		category:    "Logs",
		description: "Export unit logs to a .log file",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			mod, cmd := m.exportLogs("txt")
			return mod, cmd, true
		},
	})

	actions = append(actions, paletteAction{
		id:          "export-logs-json",
		title:       "Export Logs (JSONL)",
		shortcut:    "",
		category:    "Logs",
		description: "Export structured log entries to a .jsonl file",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			mod, cmd := m.exportLogs("json")
			return mod, cmd, true
		},
	})

	actions = append(actions, paletteAction{
		id:          "copy-logs",
		title:       "Copy Logs to Clipboard",
		shortcut:    "c",
		category:    "Logs",
		description: "Copy visible log lines to clipboard via OSC52",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			mod, cmd := m.exportLogs("clipboard")
			return mod, cmd, true
		},
	})

	actions = append(actions, paletteAction{
		id:          "cycle-log-prio",
		title:       "Cycle Log Priority",
		shortcut:    "p",
		category:    "Logs",
		description: "Filter logs by priority: all -> err -> warn -> info",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			mod, cmd := m.cycleLogPriority()
			return mod, cmd, true
		},
	})

	actions = append(actions, paletteAction{
		id:          "source",
		title:       "View Quadlet Source",
		shortcut:    "enter",
		category:    "View",
		description: "View Quadlet file source and effective drop-ins",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.openFileKeys(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "detail",
		title:       "Detail & Inspect Tabs",
		shortcut:    "]",
		category:    "View",
		description: "Tabbed detail pane (source / status / journal / inspect)",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			m.mode = modeDetail
			m.tab = tabStatus
			m.resize()
			return m, nil, true
		},
	})

	return actions
}
