package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// This file holds the per-section builders for the command palette.
// buildPaletteActions (palette.go) only concatenates them, so a screen
// gains palette entries by extending its own builder here.

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

// customPaletteActions exposes one palette entry per user-defined command
// from config.yaml.
func (m Model) customPaletteActions() []paletteAction {
	var actions []paletteAction

	for _, cc := range m.custom {
		cmdObj := cc
		actions = append(actions, paletteAction{
			id:          "custom-" + cmdObj.Name,
			title:       "Custom: " + cmdObj.Name,
			shortcut:    cmdObj.Key,
			category:    "Custom",
			description: cmdObj.Run,
			enabled: func(m Model) (bool, string) {
				if m.readonly {
					return false, "readonly"
				}
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
				return m, tea.Batch(m.setBusy("custom "+cmdObj.Name), customCmd(cmdObj, u, m.images)), true
			},
		})
	}

	return actions
}
