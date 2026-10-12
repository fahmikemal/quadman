package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// This file is the shared skeleton of every secondary screen (DESIGN.md
// section 9): nav strip → summary tiles → optional sub-tabs and toolbar →
// a table with group headings, problems first → a detail panel for the
// selected row → the grouped footer. Each screen only describes itself
// as a screenSpec; layout, scrolling, filtering, and navigation live here
// so all screens behave the same.

// scrCol is one table column. A zero width makes the column flexible: it
// shares the width left after the fixed columns, between min and max.
type scrCol struct {
	title string
	w     int // fixed width; 0 = flex
	min   int // flex floor
	max   int // flex cap (0 = none)
	hide  int // hide the column when the terminal is narrower than this
	right bool
	src   int // index of the column in the spec (set by layoutCols)
}

// scrCell is one styled table cell.
type scrCell struct {
	text  string
	style lipgloss.Style
}

func cell(text string, st lipgloss.Style) scrCell { return scrCell{text: text, style: st} }

// scrRow is a table row, or a group heading when heading is set.
type scrRow struct {
	id      string // stable identity: the cursor stays on it across refreshes
	heading string
	cells   []scrCell
	search  string // filter text; defaults to the joined cells
	unit    string // generated unit name the row belongs to (↵ opens it)
	dim     bool
}

func (r scrRow) searchText() string {
	if r.search != "" {
		return r.search
	}
	parts := make([]string, 0, len(r.cells))
	for _, c := range r.cells {
		parts = append(parts, c.text)
	}
	return strings.Join(parts, " ")
}

// screenSpec describes one rendered state of a secondary screen.
type screenSpec struct {
	tiles   []tile
	summary string   // one-line stand-in for the tiles below 100×30
	tabs    []string // sub-tab labels; m.scrTab is the active one
	toolbar string   // one styled line under the tabs (live state, filters)
	cols    []scrCol
	rows    []scrRow
	empty   string // shown when there are no rows at all
	// detail returns the selected row's panel lines (nil = no panel).
	detail    func(r scrRow, width int) []string
	detailMax int    // panel line cap (default 6)
	info      string // status-line text while nothing else is reported
}

// screenState is the cursor and view state shared by the secondary
// screens; enterScreen resets it whenever a different screen opens.
type screenState struct {
	scrCur       int
	scrOff       int
	scrTab       int
	scrFull      bool // narrow terminals: the detail panel fills the screen
	scrFilter    string
	scrFiltering bool
	scrFilterIn  textinput.Model
	scrSort      int
	scrSortDesc  bool
	// secretIn is the secrets screen's create/rotate prompt.
	secretIn secretInput
	// probList caches the problems screen's findings.
	probList []problem
	// scrLoaded has one bit per async screen holding data from this
	// session; scrLoading is the screen whose load is in flight (modeList:
	// none); scrErr is why scrErrFor's first load failed.
	scrLoaded  uint32
	scrLoading mode
	scrErr     string
	scrErrFor  mode
	// scrLoadedAt is when each screen's data landed; scrSeq numbers the
	// loads (only the newest may land) and scrCancel ends the one in
	// flight. statsInFlight keeps the poll tick to one sample at a time.
	scrLoadedAt   [modeCount]time.Time
	scrSeq        uint64
	scrCancel     context.CancelFunc
	statsInFlight bool
}

// isScreenMode reports the modes rendered through the screen skeleton.
func isScreenMode(md mode) bool {
	switch md {
	case modeStorage, modeEvents, modeTimers, modeSecrets, modeRecent,
		modeStats, modeUpdates, modeValidate, modeTree:
		return true
	}
	return false
}

// enterScreen switches to md with a fresh cursor, filter and sub-tab.
func (m *Model) enterScreen(md mode) {
	if m.mode == md {
		return
	}
	// Leaving a screen abandons its load; leaving a live view ends its
	// follow process (palette and nav keys can open a screen straight from
	// the journal or the events view).
	m.cancelLoad()
	switch m.mode {
	case modeDetail:
		m.stopLogs()
	case modeEvents:
		m.stopEvents()
	}
	m.mode = md
	m.scrCur, m.scrOff, m.scrTab = 0, 0, 0
	m.scrFull = false
	m.scrFilter = ""
	m.scrFiltering = false
	m.scrFilterIn.SetValue("")
	m.scrFilterIn.Blur()
	m.scrSort, m.scrSortDesc = 0, false
	m.resize()
}

// screenSpec returns the spec of the current screen mode. An async screen
// without data yet keeps its skeleton but shows a loading or failed state
// instead of values derived from nothing.
func (m Model) screenSpec() screenSpec {
	s := m.specFor(m.mode)
	if !isAsyncScreen(m.mode) || m.screenLoaded(m.mode) {
		return s
	}
	switch {
	case m.scrErrFor == m.mode && m.scrErr != "":
		return placeholderSpec(s, "—", "not loaded",
			"could not load: "+m.scrErr+"  ·  "+retryKey(m.mode)+" retry  ·  esc back")
	case m.scrLoading == m.mode:
		return placeholderSpec(s, "…", "loading", "loading…")
	}
	return s
}

// placeholderSpec blanks every data-derived part of s (tile values, rows,
// toolbar, detail) and puts body where the rows would be.
func placeholderSpec(s screenSpec, value, note, body string) screenSpec {
	tiles := make([]tile, len(s.tiles))
	for i, t := range s.tiles {
		tiles[i] = tile{label: t.label, value: value, note: note, valueStyle: mutedStyle}
	}
	s.tiles = tiles
	tabs := make([]string, len(s.tabs))
	for i, t := range s.tabs {
		tabs[i] = withoutCount(t)
	}
	s.tabs = tabs
	if s.summary != "" {
		s.summary = mutedStyle.Render(body)
	}
	s.toolbar = ""
	s.rows = nil
	s.detail = nil
	s.empty = body
	return s
}

// withoutCount drops a trailing row count from a sub-tab label
// ("Images 0" → "Images"): there is nothing counted yet.
func withoutCount(label string) string {
	i := strings.LastIndexByte(label, ' ')
	if i < 0 || strings.TrimLeft(label[i+1:], "0123456789") != "" {
		return label
	}
	return label[:i]
}

// waitingHint answers a key pressed on an async screen that has no data.
func (m Model) waitingHint() string {
	if m.scrErrFor == m.mode && m.scrErr != "" {
		return "nothing loaded yet: " + retryKey(m.mode) + " retries, esc goes back"
	}
	return "still loading: actions wait for the data (esc goes back)"
}

// specFor builds the spec of screen md from the model's data.
func (m Model) specFor(md mode) screenSpec {
	switch md {
	case modeStorage:
		return m.storageSpec()
	case modeEvents:
		return m.eventsSpec()
	case modeTimers:
		return m.timersSpec()
	case modeSecrets:
		return m.secretsSpec()
	case modeRecent:
		return m.actionsSpec()
	case modeStats:
		return m.statsSpec()
	case modeUpdates:
		return m.updatesSpec()
	case modeValidate:
		return m.problemsSpec()
	case modeTree:
		return m.treeSpec()
	}
	return screenSpec{}
}

// visibleRows applies the screen filter. Headings survive only while a
// row under them matches.
func (m Model) visibleRows(rows []scrRow) []scrRow {
	if m.scrFilter == "" {
		return rows
	}
	var out []scrRow
	var pending *scrRow
	for i := range rows {
		r := rows[i]
		if r.heading != "" {
			pending = &rows[i]
			continue
		}
		if !fuzzyMatch(r.searchText(), m.scrFilter) {
			continue
		}
		if pending != nil {
			out = append(out, *pending)
			pending = nil
		}
		out = append(out, r)
	}
	return out
}

// selectable maps cursor positions to indices of non-heading rows.
func selectable(rows []scrRow) []int {
	var idx []int
	for i, r := range rows {
		if r.heading == "" {
			idx = append(idx, i)
		}
	}
	return idx
}

// selectedRow returns the row under the cursor of the current screen.
func (m Model) selectedRow() (scrRow, bool) {
	rows := m.visibleRows(m.screenSpec().rows)
	sel := selectable(rows)
	if len(sel) == 0 {
		return scrRow{}, false
	}
	c := min(max(m.scrCur, 0), len(sel)-1)
	return rows[sel[c]], true
}

// screenLayout is the vertical budget of a screen frame.
type screenLayout struct {
	tiles     bool // full tiles (else the summary line)
	detailH   int  // detail panel lines (0 = none), border excluded
	tableRows int  // data/heading rows that fit under the header
}

func (m Model) layoutFor(s screenSpec, rows []scrRow) screenLayout {
	w, h := m.size()
	avail := h - 2 - lineCount(m.helpBar()) - 1
	var l screenLayout
	l.tiles = len(s.tiles) > 0 && w >= breakpointMid && h >= 30
	switch {
	case l.tiles:
		avail -= 5
	case s.summary != "" || len(s.tiles) > 0:
		avail--
	}
	if len(s.tabs) > 0 {
		avail--
	}
	if s.toolbar != "" || m.scrFiltering || m.scrFilter != "" {
		avail--
	}
	if s.detail != nil && w >= breakpointMid {
		if r, ok := m.selectedRowIn(rows); ok {
			n := len(s.detail(r, w-4))
			capN := s.detailMax
			if capN == 0 {
				capN = 6
			}
			n = min(n, capN, max(avail/3, 2))
			if n > 0 {
				l.detailH = n
				avail -= n + 2
			}
		}
	}
	l.tableRows = max(avail-2, 1) // table header + "more" line
	return l
}

func (m Model) selectedRowIn(rows []scrRow) (scrRow, bool) {
	sel := selectable(rows)
	if len(sel) == 0 {
		return scrRow{}, false
	}
	return rows[sel[min(max(m.scrCur, 0), len(sel)-1)]], true
}

// screenWindow returns the visible line range, keeping the cursor row on
// screen and, when there is room, the heading right above it.
func (m Model) screenWindow(rows []scrRow, height int) (int, int) {
	sel := selectable(rows)
	off := m.scrOff
	if len(sel) > 0 {
		line := sel[min(max(m.scrCur, 0), len(sel)-1)]
		if line < off {
			off = line
			if line > 0 && rows[line-1].heading != "" {
				off = line - 1
			}
		}
		if line >= off+height {
			off = line - height + 1
		}
	}
	off = max(min(off, len(rows)-height), 0)
	return off, min(off+height, len(rows))
}

// syncScreenOffset persists the scroll window after the cursor moved.
func (m *Model) syncScreenOffset() {
	if !isScreenMode(m.mode) {
		return
	}
	s := m.screenSpec()
	rows := m.visibleRows(s.rows)
	if n := len(selectable(rows)); m.scrCur >= n {
		m.scrCur = max(n-1, 0)
	}
	l := m.layoutFor(s, rows)
	m.scrOff, _ = m.screenWindow(rows, l.tableRows)
}

// renderScreen draws the body of a secondary screen.
func (m Model) renderScreen(w int) string {
	s := m.screenSpec()
	rows := m.visibleRows(s.rows)
	l := m.layoutFor(s, rows)
	var parts []string
	switch {
	case l.tiles:
		parts = append(parts, tilesRow(s.tiles, w))
	case s.summary != "":
		parts = append(parts, ansi.Truncate(s.summary, w, "…"))
	case len(s.tiles) > 0:
		parts = append(parts, tilesSummary(s.tiles, w))
	}
	if len(s.tabs) > 0 {
		parts = append(parts, m.subTabs(s.tabs, w))
	}
	if m.scrFiltering || m.scrFilter != "" {
		parts = append(parts, filterStyle.Render("/ "+m.scrFilterIn.View()))
	} else if s.toolbar != "" {
		parts = append(parts, ansi.Truncate(s.toolbar, w, "…"))
	}

	sel, hasSel := m.selectedRowIn(rows)
	if w < breakpointMid && m.scrFull && hasSel && s.detail != nil {
		parts = append(parts, detailBox(s.detail(sel, w-4), w, -1))
		return m.fillBody(strings.Join(parts, "\n"))
	}

	parts = append(parts, m.screenTable(s, rows, w, l.tableRows))
	if l.detailH > 0 && hasSel {
		parts = append(parts, detailBox(s.detail(sel, w-4), w, l.detailH))
	}
	return m.fillBody(strings.Join(parts, "\n"))
}

// fillBody pads the body with blank lines (or clips it) to exactly the
// height between the nav strip and the footer, so the footer never moves.
func (m Model) fillBody(body string) string {
	_, h := m.size()
	avail := h - 2 - lineCount(m.helpBar()) - 1
	lines := strings.Split(body, "\n")
	if len(lines) > avail {
		lines = lines[:max(avail, 1)]
	}
	for len(lines) < avail {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// screenTable renders the header, the row window and a "more" line.
func (m Model) screenTable(s screenSpec, rows []scrRow, w, height int) string {
	cols, widths := layoutCols(s.cols, w)
	var b strings.Builder
	head := strings.Repeat(" ", rowPrefixW)
	for i, c := range cols {
		if i > 0 {
			head += " "
		}
		t := padCell(c.title, widths[i])
		if c.right {
			t = padLeft(c.title, widths[i])
		}
		head += t
	}
	b.WriteString(dimStyle.Bold(true).Render(ansi.Truncate(head, w, "")))
	if len(rows) == 0 {
		b.WriteString("\n")
		empty := s.empty
		if m.scrFilter != "" {
			empty = fmt.Sprintf("no rows match %q (esc clears the filter)", m.scrFilter)
		}
		if empty == "" {
			empty = "nothing here"
		}
		b.WriteString(mutedStyle.Render(ansi.Truncate("  "+empty, w, "…")))
		return b.String()
	}
	start, end := m.screenWindow(rows, height)
	selLine := -1
	if sl := selectable(rows); len(sl) > 0 {
		selLine = sl[min(max(m.scrCur, 0), len(sl)-1)]
	}
	for i := start; i < end; i++ {
		b.WriteString("\n")
		r := rows[i]
		if r.heading != "" {
			b.WriteString(dimStyle.Bold(true).Render(ansi.Truncate(" "+r.heading, w, "…")))
			continue
		}
		b.WriteString(renderScrRow(r, cols, widths, w, i == selLine))
	}
	if below := len(rows) - end; below > 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more", below)))
	}
	return b.String()
}

// layoutCols drops columns hidden at width w and sizes flex columns.
// Each kept column records its index in all, so rows keep reading the
// right cell when a middle column is hidden.
func layoutCols(all []scrCol, w int) ([]scrCol, []int) {
	var cols []scrCol
	for i, c := range all {
		if c.hide > 0 && w < c.hide {
			continue
		}
		c.src = i
		cols = append(cols, c)
	}
	widths := make([]int, len(cols))
	fixed, flex := rowPrefixW+max(len(cols)-1, 0), 0
	for i, c := range cols {
		if c.w > 0 {
			widths[i] = c.w
			fixed += c.w
		} else {
			flex++
		}
	}
	left := w - fixed
	// Hand out the remaining width evenly, honoring caps; a capped
	// column's surplus goes back to the others.
	for flex > 0 {
		share := left / flex
		changed := false
		for i, c := range cols {
			if c.w > 0 || widths[i] > 0 {
				continue
			}
			if c.max > 0 && share >= c.max {
				widths[i] = c.max
				left -= c.max
				flex--
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	if flex > 0 {
		share := max(left/flex, 1)
		extra := left - share*flex
		for i, c := range cols {
			if c.w > 0 || widths[i] > 0 {
				continue
			}
			widths[i] = max(share, c.min)
			if extra > 0 {
				widths[i]++
				extra--
			}
		}
	}
	return cols, widths
}

// renderScrRow draws one data row; the selected row is painted on selBg
// with the accent edge, like the unit list.
func renderScrRow(r scrRow, cols []scrCol, widths []int, w int, sel bool) string {
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
	for i := range cols {
		if i > 0 {
			b.WriteString(gap)
		}
		c := scrCell{}
		if src := cols[i].src; src < len(r.cells) {
			c = r.cells[src]
		}
		style := c.style
		if r.dim {
			style = dimStyle
		}
		text := padCell(c.text, widths[i])
		if cols[i].right {
			text = padLeft(c.text, widths[i])
		}
		b.WriteString(st(style).Render(text))
	}
	line := b.String()
	if lw := ansi.StringWidth(line); lw < w && sel {
		line += onSel(lipgloss.NewStyle()).Render(strings.Repeat(" ", w-lw))
	}
	return ansi.Truncate(line, w, "")
}

// padLeft right-aligns s in w columns.
func padLeft(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// detailBox frames the detail panel; height < 0 means as tall as needed.
func detailBox(lines []string, w, height int) string {
	inner := w - 4
	if height >= 0 && len(lines) > height {
		lines = lines[:height]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, inner, "…")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(activePalette.border)).
		Padding(0, 1).
		Width(w).
		Render(strings.Join(lines, "\n"))
}

// subTabs renders the screen's sub-tabs like the nav strip.
func (m Model) subTabs(tabs []string, w int) string {
	var b strings.Builder
	for i, t := range tabs {
		if i == m.scrTab {
			b.WriteString(tabActiveStyle.Render(" " + t + " "))
		} else {
			b.WriteString(" " + fgStyle.Render(t) + " ")
		}
	}
	b.WriteString("  " + keyHint("[ ]", "switch"))
	return ansi.Truncate(b.String(), w, "…")
}

// tilesSummary folds tiles into one line for narrow or short terminals.
func tilesSummary(ts []tile, w int) string {
	var parts []string
	for _, t := range ts {
		parts = append(parts, dimStyle.Render(strings.ToLower(t.label))+" "+t.valueStyle.Render(t.value))
	}
	return ansi.Truncate(" "+strings.Join(parts, dimStyle.Render(" · ")), w, "…")
}

// detailField renders "label  value" for detail panels.
func detailField(label, value string) string {
	return dimStyle.Render(padCell(label, 12)) + fgStyle.Render(value)
}

// screenKeys is the shared navigation of every secondary screen. It runs
// after the screen's own keys and always consumes the key, so nothing
// leaks into the unit-list actions behind the screen.
func (m Model) screenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.screenSpec()
	rows := m.visibleRows(s.rows)
	n := len(selectable(rows))
	page := max(m.layoutFor(s, rows).tableRows-1, 1)
	switch msg.String() {
	case "up", "k":
		m.scrCur = max(m.scrCur-1, 0)
	case "down", "j":
		m.scrCur = min(m.scrCur+1, max(n-1, 0))
	case "pgup", "ctrl+u":
		m.scrCur = max(m.scrCur-page, 0)
	case "pgdown", "ctrl+d":
		m.scrCur = min(m.scrCur+page, max(n-1, 0))
	case "home":
		m.scrCur = 0
	case "end":
		m.scrCur = max(n-1, 0)
	case "[", "]":
		if len(s.tabs) > 0 {
			d := 1
			if msg.String() == "[" {
				d = len(s.tabs) - 1
			}
			m.scrTab = (m.scrTab + d) % len(s.tabs)
			m.scrCur, m.scrOff = 0, 0
		}
	case "/":
		m.scrFiltering = true
		m.scrFilterIn.Focus()
		m.resize()
		return m, textinput.Blink
	case "enter":
		w, _ := m.size()
		if w < breakpointMid && s.detail != nil && n > 0 {
			m.scrFull = !m.scrFull
			return m, nil
		}
		if r, ok := m.selectedRowIn(rows); ok && r.unit != "" {
			return m.openUnitInList(r.unit)
		}
	case "esc", "q":
		switch {
		case m.scrFull:
			m.scrFull = false
		case m.scrFilter != "":
			m.scrFilter = ""
			m.scrFilterIn.SetValue("")
			m.resize()
		default:
			return m.leaveScreen()
		}
	default:
		if md, ok := navKeyMode(msg.String()); ok && md != m.mode {
			return m.openView(md)
		}
		if isAsyncScreen(m.mode) && !m.screenLoaded(m.mode) {
			m.setStatus(m.waitingHint(), false)
		}
	}
	return m, nil
}

// screenFilterKeys owns the keyboard while the screen filter is open.
func (m Model) screenFilterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.scrFiltering {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		m.scrFiltering = false
		m.scrFilterIn.Blur()
		m.scrFilter = strings.TrimSpace(m.scrFilterIn.Value())
		m.resize()
		return m, nil, true
	case "esc":
		m.scrFiltering = false
		m.scrFilterIn.Blur()
		m.scrFilterIn.SetValue("")
		m.scrFilter = ""
		m.resize()
		return m, nil, true
	}
	var cmd tea.Cmd
	m.scrFilterIn, cmd = m.scrFilterIn.Update(msg)
	m.scrFilter = strings.TrimSpace(m.scrFilterIn.Value())
	m.scrCur, m.scrOff = 0, 0
	return m, cmd, true
}

// navKeyMode maps a nav-strip key to its view.
func navKeyMode(k string) (mode, bool) {
	for _, it := range navItems {
		if it.key != "" && it.key == k {
			return it.mode, true
		}
	}
	return 0, false
}

// openView opens a nav-strip view from anywhere, exactly as its key does
// on the unit list.
func (m Model) openView(md mode) (tea.Model, tea.Cmd) {
	mm, cmd := m.leaveScreen()
	base := mm.(Model)
	for _, it := range navItems {
		if it.mode == md && it.key != "" {
			r := []rune(it.key)
			if model, c, ok := base.navOpen(tea.KeyPressMsg{Code: r[0], Text: it.key}); ok {
				return model, tea.Batch(cmd, c)
			}
		}
	}
	return base, cmd
}

// navOpen dispatches a nav key to its list-mode opener.
func (m Model) navOpen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	for _, h := range []func(Model, tea.KeyPressMsg) (tea.Model, tea.Cmd, bool){
		Model.statsOpenKeys, Model.treeKeysOpen, Model.problemsKeys, Model.updatesOpenKeys,
		Model.storageKeys, Model.eventsKeys, Model.timersKeys, Model.secretsKeys, Model.openRecentKeys,
	} {
		if mm, cmd, ok := h(m, msg); ok {
			return mm, cmd, true
		}
	}
	return m, nil, false
}

// leaveScreen returns to the unit list, stopping any live stream.
func (m Model) leaveScreen() (tea.Model, tea.Cmd) {
	if m.mode == modeEvents {
		m.stopEvents()
	}
	m.cancelLoad()
	m.scrFiltering = false
	m.scrFilter = ""
	m.scrFilterIn.SetValue("")
	m.scrFull = false
	m.mode = modeList
	m.resize()
	return m, nil
}

// openUnitInList jumps to the unit list with unitName selected.
func (m Model) openUnitInList(unitName string) (tea.Model, tea.Cmd) {
	mm, cmd := m.leaveScreen()
	base := mm.(Model)
	if base.filterStr != "" {
		base.filterStr = ""
		base.filterIn.SetValue("")
		base.refilter()
	}
	for i, u := range base.filtered {
		if u.UnitName == unitName || u.Name == unitName {
			base.table.SetCursor(i)
			base.syncListOffset()
			return base, cmd
		}
	}
	base.setStatus(unitName+" is not a quadlet unit in this list", false)
	return base, cmd
}

// unitByName finds a quadlet unit by base name or generated unit name.
func (m Model) unitByName(name string) (int, bool) {
	for i, u := range m.units {
		if u.UnitName == name || u.Name == name {
			return i, true
		}
	}
	return 0, false
}

// unitGlyph renders "● name" for a unit reference in USED BY columns.
func (m Model) unitGlyph(name string) string {
	if i, ok := m.unitByName(name); ok {
		st := m.stateOf(m.units[i])
		return glyph(st.glyph) + " " + m.units[i].Name
	}
	return glyph(gNotFound) + " " + name
}
