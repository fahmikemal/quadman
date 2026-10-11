package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	m.paletteActions = groupByCategory(m.buildPaletteActions())
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

// groupByCategory makes each category contiguous (in order of first
// appearance, stable within a category) so the palette can draw one
// heading per group.
func groupByCategory(actions []paletteAction) []paletteAction {
	order := map[string]int{}
	for _, a := range actions {
		if _, ok := order[a.category]; !ok {
			order[a.category] = len(order)
		}
	}
	out := slices.Clone(actions)
	slices.SortStableFunc(out, func(a, b paletteAction) int {
		return order[a.category] - order[b.category]
	})
	return out
}

// paletteView renders the Command Palette: a centered 84-column box
// (a full-width sheet below 100 columns) with the actions grouped under
// their category and the target unit shown beside the input.
func (m Model) paletteView() string {
	w, h := m.size()
	boxW := min(84, w-4)
	if w < breakpointMid {
		boxW = w // a full-width sheet on narrow terminals
	}
	inner := boxW - 4
	// Body height minus the box border and its title, input, rule and
	// count lines.
	avail := h - 2 - lineCount(m.helpBar()) - 1
	maxRows := max(avail-2-4, 3)

	var b strings.Builder
	title := accentStyle.Bold(true).Render("COMMAND PALETTE")
	closeHint := dimStyle.Render("esc close")
	b.WriteString(title + strings.Repeat(" ", max(inner-ansi.StringWidth(title)-ansi.StringWidth(closeHint), 1)) + closeHint)
	b.WriteString("\n")
	input := accentStyle.Render("› ") + m.paletteIn.View()
	target := ""
	if u, ok := m.selected(); ok && m.prevMode != modeTree {
		target = mutedStyle.Render("on ") + fgStyle.Render(ansi.Truncate(u.UnitName, 30, "…"))
	}
	b.WriteString(input + strings.Repeat(" ", max(inner-ansi.StringWidth(input)-ansi.StringWidth(target), 1)) + target)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", inner)))
	b.WriteString("\n")

	if len(m.paletteFiltered) == 0 {
		b.WriteString(mutedStyle.Render("No matching commands found."))
	} else {
		lines, cursorLine := m.paletteLines(inner)
		start := 0
		if cursorLine >= maxRows {
			start = cursorLine - maxRows + 1
		}
		end := min(start+maxRows, len(lines))
		b.WriteString(strings.Join(lines[start:end], "\n"))
	}
	b.WriteString("\n")
	count := fmt.Sprintf("%d commands", len(m.paletteFiltered))
	if q := strings.TrimSpace(m.paletteIn.Value()); q != "" {
		count = fmt.Sprintf("%d of %d match %q", len(m.paletteFiltered), len(m.paletteActions), q)
	}
	pos := ""
	if len(m.paletteFiltered) > 0 {
		pos = fmt.Sprintf("%d/%d", m.paletteCursor+1, len(m.paletteFiltered))
	}
	foot := dimStyle.Render(count + " · ↑/↓ move · ↵ run")
	b.WriteString(foot + strings.Repeat(" ", max(inner-ansi.StringWidth(foot)-len(pos), 1)) + mutedStyle.Render(pos))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(activePalette.accent)).
		Padding(0, 1).
		Width(boxW).
		Render(b.String())
}

// paletteLines renders the filtered actions with a category heading each
// time the category changes, and returns the cursor's line index.
func (m Model) paletteLines(width int) ([]string, int) {
	var lines []string
	cursorLine := 0
	lastCat := ""
	for i, act := range m.paletteFiltered {
		if act.category != lastCat {
			lines = append(lines, dimStyle.Bold(true).Render(strings.ToUpper(act.category)))
			lastCat = act.category
		}
		if i == m.paletteCursor {
			cursorLine = len(lines)
		}
		lines = append(lines, m.paletteRow(act, width, i == m.paletteCursor))
	}
	return lines, cursorLine
}

// paletteRow draws one action: title, description, then the disabled
// reason or the shortcut key right-aligned.
func (m Model) paletteRow(act paletteAction, width int, sel bool) string {
	enabled, reason := true, ""
	if act.enabled != nil {
		enabled, reason = act.enabled(m)
	}
	st := func(s lipgloss.Style) lipgloss.Style {
		if sel {
			return onSel(s)
		}
		return s
	}
	tStyle, descStyle := fgStyle, mutedStyle
	if sel {
		tStyle = brightStyle
	}
	if !enabled {
		tStyle, descStyle = dimStyle, dimStyle
	}
	right := ""
	switch {
	case !enabled && reason != "":
		right = "(" + reason + ")"
	case !enabled:
		right = "(disabled)"
	case reason != "":
		right = "[" + reason + "]"
	}
	if act.shortcut != "" {
		right = strings.TrimSpace(right + "  " + act.shortcut)
	}
	prefix := "  "
	if sel {
		prefix = cursorGlyph() + " "
	}
	titleCol := padCell(act.title, 26)
	descW := max(width-2-26-1-ansi.StringWidth(right)-1, 0)
	desc := padCell(act.description, descW)
	rightStyle := accentStyle
	if !enabled || reason != "" {
		rightStyle = dimStyle
	}
	return st(accentStyle).Render(prefix) +
		st(tStyle).Render(titleCol) +
		st(lipgloss.NewStyle()).Render(" ") +
		st(descStyle).Render(desc) +
		st(lipgloss.NewStyle()).Render(" ") +
		st(rightStyle).Render(right)
}
