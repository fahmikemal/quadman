package ui

import (
	tea "charm.land/bubbletea/v2"
)

// configPaletteActions edits the Quadlet source and manages boot state
// (enable/disable at boot).
func (m Model) configPaletteActions() []paletteAction {
	var actions []paletteAction

	actions = append(actions, paletteAction{
		id:          "edit",
		title:       "Edit Quadlet File",
		shortcut:    "E",
		category:    "Config",
		description: "Edit source file in $EDITOR and prompt to reload",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if m.noEditor {
				return false, "disabled in SSH server"
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
			return m.editKeys(tea.KeyPressMsg{Code: 'E', Text: "E"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "enable",
		title:       "Enable at Boot",
		shortcut:    "e",
		category:    "Boot",
		description: "Append [Install] WantedBy=default.target",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			if f, err := m.parseUnitFile(u); err == nil && f.BootTarget() != "" {
				return false, "already enabled"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.enableKeys(tea.KeyPressMsg{Code: 'e', Text: "e"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "disable",
		title:       "Disable from Boot",
		shortcut:    "d",
		category:    "Boot",
		description: "Remove [Install] to disable start at boot",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			if f, err := m.parseUnitFile(u); err == nil && f.BootTarget() == "" {
				return false, "already disabled"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.disableKeys(tea.KeyPressMsg{Code: 'd', Text: "d"})
		},
	})

	return actions
}
