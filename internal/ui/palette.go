package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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
// Each section lives in its own builder in palette_actions.go; custom
// commands and compartment switchers append last.
func (m Model) buildPaletteActions() []paletteAction {
	var actions []paletteAction
	actions = append(actions, m.lifecyclePaletteActions()...)
	actions = append(actions, m.logsPaletteActions()...)
	actions = append(actions, m.configPaletteActions()...)
	actions = append(actions, m.hostPaletteActions()...)
	actions = append(actions, m.diagnosticsPaletteActions()...)
	actions = append(actions, m.customPaletteActions()...)
	actions = append(actions, m.compartmentPaletteActions()...)

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
