package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

// hostPaletteActions covers generator reloads and the linger toggle.
func (m Model) hostPaletteActions() []paletteAction {
	var actions []paletteAction

	actions = append(actions, paletteAction{
		id:          "reload",
		title:       "Daemon Reload",
		shortcut:    "R",
		category:    "System",
		description: "Re-run systemd's Quadlet generator (systemctl --user daemon-reload)",
		enabled: func(m Model) (bool, string) {
			if len(m.stale) > 0 {
				return true, fmt.Sprintf("%d file(s) changed", len(m.stale))
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.reloadKeys(tea.KeyPressMsg{Code: 'R', Text: "R"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "linger",
		title:       "Toggle User Linger",
		shortcut:    "L",
		category:    "System",
		description: "Toggle loginctl enable-linger (keeps containers running after logout)",
		enabled: func(m Model) (bool, string) {
			if m.system {
				return false, "rootless only"
			}
			if m.readonly {
				return false, "readonly"
			}
			if m.lingerKnown {
				if m.linger {
					return true, "currently ON"
				}
				return true, "currently OFF"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.lingerKeys(tea.KeyPressMsg{Code: 'L', Text: "L"})
		},
	})

	return actions
}
