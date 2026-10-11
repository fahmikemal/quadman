package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// refilter recomputes the visible unit list after the filter changed,
// keeping the cursor pinned on the same unit when it survives the filter.
func (m *Model) refilter() {
	pin := ""
	if c := m.table.Cursor(); c >= 0 && c < len(m.filtered) {
		pin = m.filtered[c].UnitName
	}
	m.filtered = m.sortUnits(filterUnits(m.units, m.filterStr))
	m.buildRows()
	m.pinCursor(pin)
}

// pinCursor moves the cursor back onto the named unit when it is visible.
func (m *Model) pinCursor(unitName string) {
	if unitName == "" {
		return
	}
	for i, u := range m.filtered {
		if u.UnitName == unitName {
			m.table.SetCursor(i)
			break
		}
	}
	m.syncListOffset()
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

// unitState is one unit's display state: glyph + word + color, never
// color alone (DESIGN.md principle 1).
type unitState struct {
	glyph glyphKind
	word  string
	sub   string
}

// stateOf classifies a unit from systemd status, podman health and its
// name (template units end in "@").
func (m Model) stateOf(u quadlet.Unit) unitState {
	st := m.status[u.UnitName]
	state, sub := st.Display()
	switch {
	case strings.HasSuffix(u.Name, "@"):
		return unitState{gTemplate, "template", "-"}
	case m.health["systemd-"+u.Name] == "unhealthy":
		return unitState{gUnhealthy, "unhealthy", "health"}
	case state == "failed":
		return unitState{gFailed, "failed", sub}
	case state == "activating" || state == "reloading":
		return unitState{gStarting, "starting", sub}
	case state == "deactivating":
		return unitState{gStarting, "stopping", sub}
	case state == "not-found":
		return unitState{gNotFound, "not-found", sub}
	case state == "active":
		return unitState{gActive, "active", sub}
	case state == "inactive":
		return unitState{gInactive, "inactive", sub}
	case state == "-":
		return unitState{gInactive, "unknown", sub}
	}
	return unitState{gInactive, state, sub}
}

// stateRank orders the problems-first sort: failed → unhealthy → starting
// → not-found → active → inactive.
func stateRank(g glyphKind) int {
	switch g {
	case gFailed:
		return 0
	case gUnhealthy:
		return 1
	case gStarting:
		return 2
	case gNotFound:
		return 3
	case gActive:
		return 4
	case gInactive:
		return 5
	}
	return 6
}

// listSort is the S-cycled table order.
type listSort int

const (
	sortProblems listSort = iota
	sortName
	sortKind
	sortCount
)

func (s listSort) String() string {
	switch s {
	case sortName:
		return "name"
	case sortKind:
		return "kind"
	}
	return "problems first"
}

// sortUnits returns a sorted copy of units in the active sort order. Units
// with validation errors rank with failed ones.
func (m Model) sortUnits(units []quadlet.Unit) []quadlet.Unit {
	out := slices.Clone(units)
	rank := func(u quadlet.Unit) int {
		r := stateRank(m.stateOf(u).glyph)
		if m.issues.marker(u.Name) == " ✗" && r > 0 {
			r = 0
		}
		return r
	}
	slices.SortStableFunc(out, func(a, b quadlet.Unit) int {
		switch m.sortBy {
		case sortProblems:
			if d := rank(a) - rank(b); d != 0 {
				return d
			}
		case sortKind:
			if c := strings.Compare(string(a.Kind), string(b.Kind)); c != 0 {
				return c
			}
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// unitRow is the render model of one table row; table.Row keeps the same
// cells as plain text for the widget's cursor bookkeeping and for tests.
type unitRow struct {
	state   unitState
	name    string
	markers []glyphKind // ✗ ⚠ ~ after the name
	marked  bool
	kind    string
	uptime  string
	image   string
}

func (r unitRow) stateText() string { return glyph(r.state.glyph) + " " + r.state.word }

func (r unitRow) nameText() string {
	s := r.name
	for _, g := range r.markers {
		if g == gExternal {
			s += glyph(g)
		} else {
			s += " " + glyph(g)
		}
	}
	if r.marked {
		s = glyph(gMarked) + " " + s
	}
	return s
}

// buildRows renders the filtered unit list into the table. A unit whose
// container podman reports as unhealthy surfaces that in the STATE column.
func (m *Model) buildRows() {
	rows := make([]table.Row, 0, len(m.filtered))
	m.rowView = make([]unitRow, 0, len(m.filtered))
	nameW := len("QUADLET")
	now := time.Now()
	for _, u := range m.filtered {
		r := unitRow{
			state:  m.stateOf(u),
			name:   u.Name,
			marked: m.marks[u.UnitName],
			kind:   string(u.Kind),
			uptime: "—",
			image:  m.images[u.UnitName],
		}
		switch m.issues.marker(u.Name) {
		case " ✗":
			r.markers = append(r.markers, gFailed)
		case " ⚠":
			r.markers = append(r.markers, gWarn)
		}
		if isExternalUnit(u.Path) {
			r.markers = append(r.markers, gExternal)
		}
		if st := m.status[u.UnitName]; r.state.glyph == gActive || r.state.glyph == gUnhealthy || r.state.glyph == gStarting {
			r.uptime = uptimeOf(st, now)
		}
		if r.image == "" {
			r.image = "—"
		}
		m.rowView = append(m.rowView, r)
		rows = append(rows, table.Row{r.stateText(), r.nameText(), r.kind, r.state.sub, r.uptime, r.image})
		nameW = max(nameW, ansi.StringWidth(r.nameText()))
	}
	// The widget renders rows cell-by-column and panics on a row wider
	// than its columns, so clear rows before narrowing and trim each row.
	cols := adaptiveColumns(m.tableWidth(), nameW)
	cursor := m.table.Cursor()
	m.table.SetRows(nil)
	m.table.SetColumns(cols)
	for i := range rows {
		rows[i] = rows[i][:len(cols)]
	}
	m.table.SetRows(rows)
	if len(rows) > 0 {
		m.table.SetCursor(min(max(cursor, 0), len(rows)-1))
	}
}

// uptimeOf formats how long a unit has been active: "2d 1h", "19h",
// "2h 11m", "4s". Unknown start time renders as "—".
func uptimeOf(st systemd.Status, now time.Time) string {
	if st.ActiveSince.IsZero() {
		return "—"
	}
	return humanDuration(now.Sub(st.ActiveSince))
}

func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0 && hours < 10:
		return fmt.Sprintf("%dh %dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	case mins > 0:
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// Column widths (DESIGN.md "Columns"). STATE fits the longest word
// ("▲ unhealthy"); QUADLET clamps between 14 and 34.
const (
	colStateW  = 13
	colKindW   = 11
	colSubW    = 9
	colUptimeW = 10
	colGap     = 1
	rowPrefixW = 2 // selection edge + cursor
	imageMinW  = 8
)

// adaptiveColumns sizes the table for a total width. Below 100 columns the
// table keeps STATE QUADLET KIND SUB; UPTIME and IMAGE join from 100 up,
// and IMAGE takes whatever width remains.
func adaptiveColumns(total, nameW int) []table.Column {
	if total <= 0 {
		total = 100
	}
	nameW = clampW(nameW+1, 14, 34)
	cols := []table.Column{
		{Title: "STATE", Width: colStateW},
		{Title: "QUADLET", Width: nameW},
		{Title: "KIND", Width: colKindW},
		{Title: "SUB", Width: colSubW},
	}
	used := rowPrefixW + colStateW + nameW + colKindW + colSubW + 3*colGap
	if total < breakpointMid {
		// Narrow: QUADLET absorbs the slack so long names survive.
		if extra := total - used; extra > 0 {
			cols[1].Width += extra
		}
		return cols
	}
	imageW := max(total-used-colUptimeW-2*colGap, imageMinW)
	return append(cols,
		table.Column{Title: "UPTIME", Width: colUptimeW},
		table.Column{Title: "IMAGE", Width: imageW},
	)
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

// syncListOffset scrolls the visible row window just enough to keep the
// cursor on screen (the widget's own offset is private, so the list
// renderer keeps its own).
func (m *Model) syncListOffset() {
	m.listOff, _ = m.visibleWindow()
}

// listRows is how many unit rows fit under the table header.
func (m Model) listRows() int {
	return m.listRowsAvail()
}

// visibleWindow returns the [start, end) row range to draw: listOff,
// nudged so the cursor is always inside even if the height changed since
// the offset was last synced.
func (m Model) visibleWindow() (int, int) {
	rows := m.listRows()
	off := m.listOff
	c := m.table.Cursor()
	if c < off {
		off = c
	}
	if c >= off+rows {
		off = c - rows + 1
	}
	off = max(min(off, len(m.rowView)-rows), 0)
	return off, min(off+rows, len(m.rowView))
}

// unitTableView renders the header and the visible window of rows. Cells
// are styled per column; the selected row gets the selection background
// and a 1-column accent edge.
func (m Model) unitTableView(width int) string {
	cols := m.table.Columns()
	var b strings.Builder
	head := strings.Repeat(" ", rowPrefixW)
	for i, c := range cols {
		if i > 0 {
			head += " "
		}
		head += padCell(c.Title, c.Width)
	}
	b.WriteString(dimStyle.Bold(true).Render(ansi.Truncate(head, width, "")))

	start, end := m.visibleWindow()
	cursor := m.table.Cursor()
	for i := start; i < end; i++ {
		b.WriteString("\n")
		b.WriteString(m.renderUnitRow(m.rowView[i], cols, width, i == cursor))
	}
	// One hint line under the rows: what is hidden and how it is sorted.
	hint := "sorted: " + m.sortBy.String() + " · S cycle sort"
	if hidden := len(m.rowView) - end; hidden > 0 {
		hint = fmt.Sprintf("… %d more  (%s)", hidden, hint)
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(ansi.Truncate("  "+hint, width, "…")))
	return b.String()
}

// renderUnitRow draws one row. Selected rows paint every cell on selBg so
// the fill survives each cell's own color reset.
func (m Model) renderUnitRow(r unitRow, cols []table.Column, width int, sel bool) string {
	st := func(s lipgloss.Style) lipgloss.Style {
		if sel {
			return onSel(s)
		}
		return s
	}
	var b strings.Builder
	if sel {
		b.WriteString(lipgloss.NewStyle().Background(lipgloss.Color(activePalette.accent)).Render(" "))
		b.WriteString(st(accentStyle).Render(cursorGlyph()))
	} else {
		b.WriteString("  ")
	}
	gap := st(lipgloss.NewStyle()).Render(" ")
	nameStyle := fgStyle
	if sel {
		nameStyle = brightStyle
	}
	for i, c := range cols {
		if i > 0 {
			b.WriteString(gap)
		}
		switch c.Title {
		case "STATE":
			b.WriteString(st(glyphStyle(r.state.glyph)).Render(padCell(r.stateText(), c.Width)))
		case "QUADLET":
			b.WriteString(m.renderNameCell(r, c.Width, st, nameStyle))
		case "KIND":
			b.WriteString(st(kindStyle).Render(padCell(r.kind, c.Width)))
		case "SUB":
			b.WriteString(st(mutedStyle).Render(padCell(r.state.sub, c.Width)))
		case "UPTIME":
			b.WriteString(st(mutedStyle).Render(padCell(r.uptime, c.Width)))
		case "IMAGE":
			b.WriteString(st(mutedStyle).Render(padCell(r.image, c.Width)))
		}
	}
	line := b.String()
	if w := ansi.StringWidth(line); w < width && sel {
		line += onSel(lipgloss.NewStyle()).Render(strings.Repeat(" ", width-w))
	}
	return ansi.Truncate(line, width, "")
}

// renderNameCell draws QUADLET with its colored markers, truncating the
// name (never the markers) to fit.
func (m Model) renderNameCell(r unitRow, w int, st func(lipgloss.Style) lipgloss.Style, nameStyle lipgloss.Style) string {
	var suffix []string
	sufW := 0
	for _, g := range r.markers {
		s := glyph(g)
		if g != gExternal {
			s = " " + s
		}
		suffix = append(suffix, st(glyphStyle(g)).Render(s))
		sufW += ansi.StringWidth(s)
	}
	prefix := ""
	if r.marked {
		prefix = glyph(gMarked) + " "
	}
	nameW := max(w-sufW-ansi.StringWidth(prefix), 1)
	name := ansi.Truncate(r.name, nameW, "…")
	var b strings.Builder
	if prefix != "" {
		b.WriteString(st(glyphStyle(gMarked)).Render(prefix))
	}
	b.WriteString(st(nameStyle).Render(name))
	b.WriteString(strings.Join(suffix, ""))
	if pad := w - ansi.StringWidth(prefix) - ansi.StringWidth(name) - sufW; pad > 0 {
		b.WriteString(st(lipgloss.NewStyle()).Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

func cursorGlyph() string {
	if asciiGlyphs {
		return ">"
	}
	return "▸"
}

// padCell truncates s to w columns ("…" on the right) and pads it out.
func padCell(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
