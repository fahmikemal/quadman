package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// paletteAction represents a single executable action inside the Command Palette.
type paletteAction struct {
	id          string
	title       string
	shortcut    string
	category    string
	description string
	enabled     func(m Model) (bool, string)
	run         func(m Model) (tea.Model, tea.Cmd, bool)
}

// openPalette activates the Command Palette overlay.
func (m *Model) openPalette() {
	m.prevMode = m.mode
	m.mode = modePalette
	m.paletteIn.SetValue("")
	m.paletteIn.Focus()
	m.paletteCursor = 0
	m.paletteActions = m.buildPaletteActions()
	m.refilterPalette()
}

// closePalette closes the Command Palette and restores previous mode.
func (m *Model) closePalette() {
	m.mode = m.prevMode
	m.paletteIn.Blur()
}

// refilterPalette filters actions according to the search query.
func (m *Model) refilterPalette() {
	q := strings.TrimSpace(m.paletteIn.Value())
	if q == "" {
		m.paletteFiltered = m.paletteActions
	} else {
		var out []paletteAction
		for _, a := range m.paletteActions {
			searchTarget := a.title + " " + a.shortcut + " " + a.category + " " + a.description
			if fuzzyMatch(searchTarget, q) {
				out = append(out, a)
			}
		}
		m.paletteFiltered = out
	}
	if m.paletteCursor >= len(m.paletteFiltered) {
		m.paletteCursor = max(0, len(m.paletteFiltered)-1)
	}
}

// buildPaletteActions compiles the complete list of context-aware actions.
func (m Model) buildPaletteActions() []paletteAction {
	var actions []paletteAction

	// 1. Unit Lifecycle
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

	// 2. Navigation & Logs
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
			if m.ssh.IsRemote() {
				return false, "unavailable over SSH"
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
			if m.ssh.IsRemote() {
				return false, "unavailable over SSH"
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

	// 3. Configuration & Boot
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
			if m.ssh.IsRemote() {
				return false, "unavailable over SSH"
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
			if m.ssh.IsRemote() {
				return false, "unavailable over SSH"
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
			if m.ssh.IsRemote() {
				return false, "unavailable over SSH"
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

	// 4. System & Host
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

	// 5. Diagnostics & Screens
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

	// 6. User Custom Commands
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

// paletteView renders the Command Palette modal overlay.
func (m Model) paletteView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render(" COMMAND PALETTE (Ctrl+P / esc to close) "))
	b.WriteString("\n")
	b.WriteString(filterStyle.Render("Search: / " + m.paletteIn.View()))
	b.WriteString("\n\n")

	if len(m.paletteFiltered) == 0 {
		b.WriteString(helpStyle.Render("  No matching commands found.\n"))
		return b.String()
	}

	maxVisible := 12
	start := 0
	if m.paletteCursor >= maxVisible {
		start = m.paletteCursor - maxVisible + 1
	}
	end := min(start+maxVisible, len(m.paletteFiltered))

	for i := start; i < end; i++ {
		act := m.paletteFiltered[i]
		selected := (i == m.paletteCursor)
		enabled, reason := true, ""
		if act.enabled != nil {
			enabled, reason = act.enabled(m)
		}

		cursorPrefix := "  "
		if selected {
			cursorPrefix = "▶ "
		}

		badge := ""
		if act.shortcut != "" {
			badge = "[" + act.shortcut + "]"
		}

		catCol := fmt.Sprintf("%-13s", act.category)
		titleCol := fmt.Sprintf("%-25s", act.title)
		badgeCol := fmt.Sprintf("%-8s", badge)

		statusText := ""
		if !enabled {
			if reason != "" {
				statusText = "(" + reason + ") "
			} else {
				statusText = "(disabled) "
			}
		} else if reason != "" {
			statusText = "[" + reason + "] "
		}

		rowContent := fmt.Sprintf("%s %s %s %s%s", catCol, titleCol, badgeCol, statusText, act.description)
		rowContent = clampLines(rowContent, m.width-6)

		if selected {
			b.WriteString(tabActiveStyle.Render(cursorPrefix + rowContent))
		} else if !enabled {
			b.WriteString(dimErrStyle.Render(cursorPrefix + rowContent))
		} else {
			b.WriteString(cursorPrefix + rowContent)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(fmt.Sprintf("  %d/%d commands · ↑/↓ navigate · enter execute · esc close", m.paletteCursor+1, len(m.paletteFiltered))))
	return b.String()
}
