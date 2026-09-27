package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// diagnosticsPaletteActions opens every diagnostic screen (health,
// updates, problems, tree, storage, events, timers, secrets, generate,
// recent audit) plus the clipboard copies.
func (m Model) diagnosticsPaletteActions() []paletteAction {
	var actions []paletteAction

	actions = append(actions, paletteAction{
		id:          "health",
		title:       "Run Healthcheck",
		shortcut:    "h",
		category:    "Diagnostics",
		description: "Execute podman healthcheck run on the container",
		enabled: func(m Model) (bool, string) {
			u, ok := m.selected()
			if !ok || u.Kind != quadlet.KindContainer {
				return false, "containers only"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.healthKeys(tea.KeyPressMsg{Code: 'h', Text: "h"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "updates",
		title:       "Auto-Updates Screen",
		shortcut:    "u",
		category:    "Maintenance",
		description: "Preview podman auto-update --dry-run and timer status",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.updatesOpenKeys(tea.KeyPressMsg{Code: 'u', Text: "u"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "validate",
		title:       "Problems & Validation",
		shortcut:    "v",
		category:    "Diagnostics",
		description: "Inspect Quadlet generator errors, warnings, and hints",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.problemsKeys(tea.KeyPressMsg{Code: 'v', Text: "v"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "tree",
		title:       "Dependency Tree",
		shortcut:    "t",
		category:    "Topology",
		description: "View Quadlet relationship graph (pods, volumes, networks, units)",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.treeKeysOpen(tea.KeyPressMsg{Code: 't', Text: "t"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "storage",
		title:       "Storage Disk Usage",
		shortcut:    "g",
		category:    "Podman",
		description: "View podman system df --verbose disk consumption",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.storageKeys(tea.KeyPressMsg{Code: 'g', Text: "g"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "events",
		title:       "Live Events Stream",
		shortcut:    "w",
		category:    "Podman",
		description: "Stream live podman events",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.eventsKeys(tea.KeyPressMsg{Code: 'w', Text: "w"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "timers",
		title:       "Systemd Timers & Calendar Schedules",
		shortcut:    "T",
		category:    "Systemd",
		description: "View systemd timers, triggers, and calendar schedules",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.timersKeys(tea.KeyPressMsg{Code: 'T', Text: "T"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "secrets",
		title:       "Podman Secret Store",
		shortcut:    "K",
		category:    "Podman",
		description: "Inspect configured Podman secrets and drivers",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.secretsKeys(tea.KeyPressMsg{Code: 'K', Text: "K"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "generate",
		title:       "Generate from Run/Compose",
		shortcut:    "n",
		category:    "Creation",
		description: "Convert docker run or compose to Quadlet via podlet",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.generateOpenKeys(tea.KeyPressMsg{Code: 'n', Text: "n"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "recent",
		title:       "Recent Actions Audit",
		shortcut:    "A",
		category:    "Audit",
		description: "View log of actions executed this session and results",
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.openRecentKeys(tea.KeyPressMsg{Code: 'A', Text: "A"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "copy-name",
		title:       "Copy Unit Name",
		shortcut:    "y",
		category:    "Clipboard",
		description: "Copy unit name to clipboard via OSC52",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			return ok, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.copyNameKeys(tea.KeyPressMsg{Code: 'y', Text: "y"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "copy-image",
		title:       "Copy Image Ref",
		shortcut:    "Y",
		category:    "Clipboard",
		description: "Copy container image to clipboard via OSC52",
		enabled: func(m Model) (bool, string) {
			_, ok := m.selected()
			return ok, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.copyImageKeys(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
		},
	})

	return actions
}
