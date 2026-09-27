package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
)

// lifecyclePaletteActions starts, stops, and restarts units, plus the
// container extras (exec shell, stats, prune, image pull).
func (m Model) lifecyclePaletteActions() []paletteAction {
	var actions []paletteAction

	actions = append(actions, paletteAction{
		id:          "start",
		title:       "Start Unit",
		shortcut:    "s",
		category:    "Lifecycle",
		description: "Start the selected unit (or all marked units)",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if len(m.markedUnits()) > 0 {
				return true, fmt.Sprintf("%d marked", len(m.markedUnits()))
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			st := m.status[u.UnitName]
			if st.LoadState == "not-found" {
				return false, "not found (reload needed)"
			}
			if st.ActiveState == "active" {
				return false, "already active"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.startRestartKeys(tea.KeyPressMsg{Code: 's', Text: "s"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "stop",
		title:       "Stop Unit",
		shortcut:    "x",
		category:    "Lifecycle",
		description: "Stop unit (container will be removed by --rm)",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if len(m.markedUnits()) > 0 {
				return true, fmt.Sprintf("%d marked", len(m.markedUnits()))
			}
			u, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			st := m.status[u.UnitName]
			if st.ActiveState == "inactive" || st.ActiveState == "failed" {
				return false, "already stopped"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.stopKeys(tea.KeyPressMsg{Code: 'x', Text: "x"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "restart",
		title:       "Restart Unit",
		shortcut:    "r",
		category:    "Lifecycle",
		description: "Restart the selected unit (or all marked units)",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if len(m.markedUnits()) > 0 {
				return true, fmt.Sprintf("%d marked", len(m.markedUnits()))
			}
			_, ok := m.selected()
			if !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.startRestartKeys(tea.KeyPressMsg{Code: 'r', Text: "r"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "exec",
		title:       "Exec Shell in Container",
		shortcut:    "X",
		category:    "Lifecycle",
		description: "Open /bin/sh in the selected unit's container",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			if m.noEditor {
				return false, "unavailable in served sessions"
			}
			if _, ok := m.selected(); !ok {
				return false, "no selection"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.execOpenKeys(tea.KeyPressMsg{Code: 'X', Text: "X"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "stats",
		title:       "Resource Stats",
		shortcut:    "o",
		category:    "Views",
		description: "Live CPU/memory/network per container (podman stats)",
		enabled: func(m Model) (bool, string) {
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.statsOpenKeys(tea.KeyPressMsg{Code: 'o', Text: "o"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "prune",
		title:       "System Prune",
		shortcut:    "P",
		category:    "Host",
		description: "Remove stopped containers + unused data, warning for inactive Quadlet units",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			if bad, reason := m.isolatedReason(); bad {
				return false, reason
			}
			if !podman.Available() {
				return false, "podman not found"
			}
			return true, ""
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			return m.pruneKeys(tea.KeyPressMsg{Code: 'P', Text: "P"})
		},
	})

	actions = append(actions, paletteAction{
		id:          "pull-image",
		title:       "Pull Unit Image",
		shortcut:    "",
		category:    "Lifecycle",
		description: "Pull the selected unit's image now (avoids a cold pull at start)",
		enabled: func(m Model) (bool, string) {
			if m.readonly {
				return false, "readonly"
			}
			img := m.unitImage()
			if img == "" {
				return false, "unknown image"
			}
			if podman.IsLocalImage(img) {
				return false, "locally built"
			}
			return true, img
		},
		run: func(m Model) (tea.Model, tea.Cmd, bool) {
			img := m.unitImage()
			if img == "" {
				return m, nil, true
			}
			return m, tea.Batch(m.setBusy("pull "+img), pullImageCmd(img)), true
		},
	})

	return actions
}
