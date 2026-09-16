package ui

import (
	"charm.land/bubbles/v2/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

// refilter recomputes the visible unit list after the filter changed,
// keeping the cursor pinned on the same unit when it survives the filter.
func (m *Model) refilter() {
	pin := ""
	if c := m.table.Cursor(); c >= 0 && c < len(m.filtered) {
		pin = m.filtered[c].UnitName
	}
	m.filtered = filterUnits(m.units, m.filterStr)
	m.buildRows()
	for i, u := range m.filtered {
		if u.UnitName == pin {
			m.table.SetCursor(i)
			break
		}
	}
}

func filterUnits(units []quadlet.Unit, pattern string) []quadlet.Unit {
	if pattern == "" {
		return units
	}
	var out []quadlet.Unit
	for _, u := range units {
		if fuzzyMatch(u.Name+" "+u.UnitName+" "+string(u.Kind), pattern) {
			out = append(out, u)
		}
	}
	return out
}

// buildRows renders the filtered unit list into the table. A unit whose
// container podman reports as unhealthy surfaces that in the STATE column.
func (m *Model) buildRows() {
	rows := make([]table.Row, 0, len(m.filtered))
	nameW, kindW, unitW, stateW, subW := len("QUADLET"), len("KIND"), len("SYSTEMD UNIT"), len("STATE"), len("SUB")
	for _, u := range m.filtered {
		state, sub := m.status[u.UnitName].Display()
		if m.health["systemd-"+u.Name] == "unhealthy" {
			state = "unhealthy"
			sub = "health"
		}
		name := u.Name + m.issues.marker(u.Name)
		if isExternalUnit(u.Path) {
			name = externalSuffix(name)
		}
		if m.marks[u.UnitName] {
			name = "* " + name
		}
		rows = append(rows, table.Row{name, string(u.Kind), u.UnitName, state, sub, m.images[u.UnitName]})
		nameW = max(nameW, ansi.StringWidth(name))
		kindW = max(kindW, len(u.Kind))
		unitW = max(unitW, len(u.UnitName))
		stateW = max(stateW, len(state))
		subW = max(subW, len(sub))
	}
	m.table.SetColumns(adaptiveColumns(m.width, nameW, kindW, unitW, stateW, subW))
	m.table.SetRows(rows)
}

// adaptiveColumns sizes the table columns from the widest content per column
// (plus padding) and gives the leftover width to IMAGE, so short values no
// longer leave huge empty gaps. Caps keep one long value from eating the table.
func adaptiveColumns(total, nameW, kindW, unitW, stateW, subW int) []table.Column {
	if total <= 0 {
		total = 100
	}
	const pad = 2
	nameW = clampW(nameW+pad, 9, 34)
	kindW = clampW(kindW+pad, 6, 12)
	unitW = clampW(unitW+pad, 14, 44)
	stateW = clampW(stateW+pad, 7, 14)
	subW = clampW(subW+pad, 5, 13)
	imageW := total - nameW - kindW - unitW - stateW - subW - 8 // separators
	if imageW < 16 {
		imageW = 16
	}
	return []table.Column{
		{Title: "QUADLET", Width: nameW},
		{Title: "KIND", Width: kindW},
		{Title: "SYSTEMD UNIT", Width: unitW},
		{Title: "STATE", Width: stateW},
		{Title: "SUB", Width: subW},
		{Title: "IMAGE", Width: imageW},
	}
}

func clampW(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m Model) selected() (quadlet.Unit, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.filtered) {
		return quadlet.Unit{}, false
	}
	return m.filtered[c], true
}
