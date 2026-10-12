package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
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
			if ok, why := m.allowed(capTargetFiles | capTerminal); !ok {
				return false, why
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
		description: "Add [Install] WantedBy= (default.target, multi-user.target for system units)",
		enabled: func(m Model) (bool, string) {
			if ok, why := m.allowed(capMutate | capTargetFiles); !ok {
				return false, why
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			if quadlet.BootSource(u) != "" {
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
			if ok, why := m.allowed(capMutate | capTargetFiles); !ok {
				return false, why
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			switch src := quadlet.BootSource(u); src {
			case "":
				return false, "already disabled"
			case u.Path:
			default:
				return false, "enabled by a drop-in"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.disableKeys(tea.KeyPressMsg{Code: 'd', Text: "d"})
		},
	})

	return actions
}
