package ui

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Breakpoints (DESIGN.md "Responsive"). Below minW×minH the UI shows only
// a "terminal too small" notice; below breakpointMid the chrome compacts;
// from breakpointWide a detail pane joins the unit list on the right.
const (
	minW           = 60
	minH           = 16
	breakpointMid  = 100
	breakpointWide = 160
	sidePaneW      = 58
)

// size is the terminal size, with a 100×30 default before the first
// WindowSizeMsg (tests, first frame).
func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	return w, h
}

// tooSmall reports a known terminal size below the supported minimum.
func (m Model) tooSmall() bool {
	return m.width > 0 && m.height > 0 && (m.width < minW || m.height < minH)
}

// sidePane reports whether the ≥160-column detail pane is shown.
func (m Model) sidePane() bool {
	w, _ := m.size()
	return m.mode == modeList && w >= breakpointWide
}

// tableWidth is the width the unit table gets (less the side pane).
func (m Model) tableWidth() int {
	w, _ := m.size()
	if m.sidePane() {
		return w - sidePaneW - 1
	}
	return w
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	if m.mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m Model) render() string {
	w, h := m.size()
	if m.tooSmall() {
		return tooSmallView(m.width, m.height)
	}
	// The palette floats over the screen it was opened from.
	base := m
	if m.mode == modePalette {
		base.mode = m.prevMode
	}
	var b strings.Builder
	b.WriteString(m.topBar(w))
	b.WriteString("\n")
	b.WriteString(m.navStrip(w))
	b.WriteString("\n")
	b.WriteString(base.body(w))
	b.WriteString("\n")
	b.WriteString(m.helpBar())
	b.WriteString("\n")
	b.WriteString(m.statusBar(w))
	frame := b.String()
	switch {
	case m.pending != nil:
		return overlay(frame, m.confirmView(w), w, h)
	case m.mode == modePalette:
		return overlay(frame, m.paletteView(), w, h)
	}
	return frame
}

// tooSmallView is the only thing drawn below the minimum size (8d).
func tooSmallView(w, h int) string {
	msg := []string{
		compactMark(),
		"",
		warnStyle.Bold(true).Render("Terminal too small"),
		mutedStyle.Render(fmt.Sprintf("current %d×%d · need %d×%d", w, h, minW, minH)),
		dimStyle.Render("resize the window, or q quit"),
	}
	block := strings.Join(msg, "\n")
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// body renders the mode's main area between the nav strip and the footer.
func (m Model) body(w int) string {
	var b strings.Builder
	switch m.mode {
	case modeList:
		b.WriteString(m.fillBody(m.listBody(w)))
	case modeDetail:
		b.WriteString(m.detailHeader(w))
		b.WriteString("\n")
		b.WriteString(ansi.Truncate(m.tabBar(""), w, "…"))
		b.WriteString("\n")
		if m.tab == tabJournal && (m.searching || m.searchStr != "") {
			info := ""
			if m.searchStr != "" {
				info = fmt.Sprintf("  (%d/%d matches)", m.matchPos, m.searchMatches)
				if m.searchMatches == 0 {
					info = "  (no matches)"
				}
			}
			b.WriteString(filterStyle.Render("/ " + m.searchIn.View() + info))
			b.WriteString("\n")
		}
		if m.tab == tabJournal && (m.filteringLogs || m.logFilter != "") {
			countInfo := fmt.Sprintf("  [%d/%d lines]", len(m.visibleLogLines()), len(m.logLines))
			b.WriteString(filterStyle.Render("Filter: / " + m.logFilterIn.View() + countInfo))
			b.WriteString("\n")
		}
		// Line numbers help in the source tab only; the other tabs are
		// prose, journal columns, or tool output.
		vp := m.viewport
		if m.tab != tabSource {
			vp.LeftGutterFunc = viewport.NoGutter
		}
		b.WriteString(vp.View())
	case modeGenerate:
		b.WriteString(m.generateView(w))
	case modePalette:
		// Rendered as an overlay by render(); nothing underneath.
	default:
		if isScreenMode(m.mode) {
			b.WriteString(m.renderScreen(w))
		} else {
			b.WriteString(m.viewport.View())
		}
	}
	return b.String()
}

// listPreamble is everything between the nav strip and the table header:
// the dashboard (tiles + attention, or one summary line) and any open
// input prompt. Mouse hit-testing and resize both measure it.
func (m Model) listPreamble(w int) string {
	var parts []string
	if m.dashExpanded() {
		parts = append(parts, m.tilesView(w))
		if att := m.attentionView(w); att != "" {
			parts = append(parts, att)
		}
	} else {
		parts = append(parts, m.summaryLine(w))
	}
	if m.generating {
		parts = append(parts, filterStyle.Render("generate from: "+m.genIn.View()))
	}
	if m.instancing {
		parts = append(parts, filterStyle.Render("instance name for "+m.selectedName()+" @: "+m.instanceIn.View()))
	}
	if m.execing {
		parts = append(parts, filterStyle.Render("exec shell in container: "+m.execIn.View()+"  (enter / esc)"))
	}
	if m.filtering || m.filterStr != "" {
		parts = append(parts, filterStyle.Render("/ "+m.filterIn.View()))
	}
	return strings.Join(parts, "\n")
}

func (m Model) listBody(w int) string {
	var b strings.Builder
	b.WriteString(m.listPreamble(w))
	b.WriteString("\n")
	tw := m.tableWidth()
	table := m.unitTableView(tw)
	if m.sidePane() {
		gap := strings.Repeat(" \n", strings.Count(table, "\n")+1)
		table = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(tw).Render(table),
			strings.TrimSuffix(gap, "\n"),
			m.sidePaneView(strings.Count(table, "\n")+1))
	}
	b.WriteString(table)
	if len(m.units) == 0 && !m.loading {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(
			"No quadlet units found. Drop *.container / *.pod / *.kube / *.volume / *.image / *.build files into"))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(strings.Join(m.searchDirs(), "  or  ")))
	}
	return b.String()
}

// sidePaneView is the ≥160-column overview of the selected unit (2b).
func (m Model) sidePaneView(height int) string {
	inner := sidePaneW - 4
	var lines []string
	u, ok := m.selected()
	if !ok {
		lines = append(lines, mutedStyle.Render("no unit selected"))
	} else {
		st := m.stateOf(u)
		lines = append(lines,
			brightStyle.Render(ansi.Truncate(u.Name, inner, "…")),
			glyphStyle(st.glyph).Render(glyph(st.glyph)+" "+st.word)+mutedStyle.Render(subNote(st)),
			"",
		)
		field := func(k, v string) {
			if v == "" {
				v = "—"
			}
			lines = append(lines, dimStyle.Render(padCell(k, 9))+fgStyle.Render(ansi.Truncate(v, inner-9, "…")))
		}
		field("unit", u.UnitName)
		field("kind", string(u.Kind))
		field("uptime", uptimeOf(m.status[u.UnitName], time.Now()))
		field("image", m.images[u.UnitName])
		if e := m.podmanInfo[u.UnitName]; e.App != "" || e.Pod != "" {
			field("app", e.App)
			field("pod", e.Pod)
		}
		field("file", u.Path)
		if issues := m.issues[u.Name]; len(issues) > 0 {
			lines = append(lines, "", dimStyle.Bold(true).Render("PROBLEMS"))
			for _, is := range issues {
				g := gWarn
				if is.Severity == quadlet.SeverityError {
					g = gFailed
				}
				lines = append(lines, glyphStyle(g).Render(glyph(g))+" "+mutedStyle.Render(ansi.Truncate(is.Message, inner-2, "…")))
			}
		}
		lines = append(lines, "",
			keyHint("enter", "file")+"  "+keyHint("l", "logs")+"  "+keyHint("r", "restart"))
	}
	if len(lines) > height-2 {
		lines = lines[:max(height-2, 0)]
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(activePalette.border)).
		Padding(0, 1).
		Width(sidePaneW).
		Height(height).
		Render(strings.Join(lines, "\n"))
}

// topBar is the surface strip: "▛ quadman › scope › target" with refresh
// and version chips on the right.
func (m Model) topBar(w int) string {
	sep := onSurface(dimStyle).Render(" › ")
	scope := "rootless"
	if m.system {
		scope = "system"
	}
	left := onSurface(lipgloss.NewStyle()).Render(" ") + logoMark() +
		onSurface(brightStyle).Render(" quadman") + sep +
		onSurface(fgStyle).Render(scope) + sep +
		onSurface(fgStyle).Render(m.target())
	interval := m.pollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	right := glyph(gRefresh) + " " + interval.String()
	if m.podmanVersion != "" {
		right += " · podman " + m.podmanVersion
	}
	right = onSurface(mutedStyle).Render(right + " ")
	return surfaceLine(left, right, w)
}

// target names whose units are shown: SSH host, sudo compartment, the
// SSH-served client, or the local user@host.
func (m Model) target() string {
	switch {
	case m.ssh.IsRemote():
		return m.ssh.Target
	case m.compOn:
		return "as " + m.comp.User
	case m.clientInfo != "":
		return "ssh: " + m.clientInfo
	case m.whoami != "":
		return m.whoami
	}
	return "local"
}

// localWhoami returns "user@host" for the top bar.
func localWhoami() string {
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	host, _ := os.Hostname()
	if host == "" {
		return name
	}
	return name + "@" + host
}

// surfaceLine joins left and right segments into one full-width surface
// row, truncating left first so the right chips stay readable.
func surfaceLine(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if ansi.StringWidth(left)+rw > w {
		left = ansi.Truncate(left, max(w-rw, 0), "…")
	}
	gap := w - ansi.StringWidth(left) - rw
	if gap < 0 {
		return ansi.Truncate(left+right, w, "")
	}
	return left + surfaceStyle.Render(strings.Repeat(" ", gap)) + right
}

// navItem is one entry of the nav strip: every view, its key and badge.
type navItem struct {
	label, key string
	mode       mode
}

var navItems = []navItem{
	{"Units", "", modeList},
	{"Stats", "o", modeStats},
	{"Tree", "t", modeTree},
	{"Problems", "v", modeValidate},
	{"Updates", "u", modeUpdates},
	{"Storage", "g", modeStorage},
	{"Events", "w", modeEvents},
	{"Timers", "T", modeTimers},
	{"Secrets", "K", modeSecrets},
	{"Actions", "A", modeRecent},
}

// navMode is the nav entry to highlight: detail belongs to Units, the
// palette to whatever it was opened from.
func (m Model) navMode() mode {
	switch m.mode {
	case modeDetail:
		return modeList
	case modePalette:
		if m.prevMode == modeDetail {
			return modeList
		}
		return m.prevMode
	}
	return m.mode
}

// navBadge returns the count shown next to a nav entry and its style.
func (m Model) navBadge(md mode) (string, lipgloss.Style) {
	switch md {
	case modeValidate:
		n, worst := 0, gWarn
		for name := range m.issues {
			n++
			if m.issues.marker(name) == " ✗" {
				worst = gFailed
			}
		}
		if n > 0 {
			return fmt.Sprint(n), glyphStyle(worst)
		}
	case modeUpdates:
		if c := m.counts(); c.pendingUpdates > 0 {
			return fmt.Sprint(c.pendingUpdates), accentStyle
		}
	}
	return "", mutedStyle
}

// navStrip renders every view with its key and badge. Below 100 columns
// labels drop to keys only (Units keeps its name). The active view is
// drawn on accent.
func (m Model) navStrip(w int) string {
	// Normal spacing first; tighten when the labels overflow by a little
	// (around 100 columns) before falling back to truncation.
	s := m.navStripSpaced(w, " ")
	if ansi.StringWidth(s) > w {
		s = m.navStripSpaced(w, "")
	}
	return ansi.Truncate(s, w, "…")
}

func (m Model) navStripSpaced(w int, pad string) string {
	narrow := w < breakpointMid
	active := m.navMode()
	var b strings.Builder
	for _, it := range navItems {
		badge, badgeStyle := m.navBadge(it.mode)
		label := it.label
		if narrow && it.key != "" {
			label = ""
		}
		if it.mode == active {
			text := strings.TrimSpace(strings.Join([]string{label, badge, it.key}, " "))
			text = strings.Join(strings.Fields(text), " ")
			b.WriteString(tabActiveStyle.Render(" " + text + " "))
			continue
		}
		b.WriteString(pad)
		if label != "" {
			b.WriteString(fgStyle.Render(label))
			if badge != "" || it.key != "" {
				b.WriteString(" ")
			}
		}
		if badge != "" {
			b.WriteString(badgeStyle.Bold(true).Render(badge))
			if it.key != "" {
				b.WriteString(" ")
			}
		}
		if it.key != "" {
			b.WriteString(accentStyle.Render(it.key))
		}
		b.WriteString(" ")
	}
	return b.String()
}

// statusBar is the last row (surface): the action result or spinner on
// the left, position and session chips on the right.
func (m Model) statusBar(w int) string {
	var left string
	switch {
	case m.pickingEditor:
		left = onSurface(warnStyle).Render(" " + m.pickerLine())
	case m.busy:
		left = onSurface(mutedStyle).Render(" " + m.spinner.View() + " " + m.busyText)
	case m.statusLine != "" && m.statusErr:
		left = onSurface(errStyle).Render(" " + glyph(gFailed) + " " + m.statusLine)
	case m.statusLine != "":
		left = onSurface(okStyle).Render(" " + glyph(gDone) + " " + m.statusLine)
	case m.loading:
		left = onSurface(mutedStyle).Render(" loading…")
	default:
		left = onSurface(dimStyle).Render(" ready")
	}
	return surfaceLine(left, m.chips(), w)
}

// chips are the right-hand status chips: list position, marks, linger or
// system scope, readonly.
func (m Model) chips() string {
	sep := onSurface(dimStyle).Render("  ")
	var cs []string
	if m.mode == modeList && len(m.filtered) > 0 {
		cs = append(cs, onSurface(mutedStyle).Render(fmt.Sprintf("%d of %d", m.table.Cursor()+1, len(m.filtered))))
	}
	if l := m.bulkLabel(); l != "" {
		cs = append(cs, onSurface(accentStyle).Render(glyph(gMarked)+" "+l))
	}
	switch {
	case m.system:
		cs = append(cs, onSurface(lingerOnStyle).Render("system"))
	case !m.lingerKnown:
		cs = append(cs, onSurface(lingerUnknownStyle).Render("linger: ?"))
	case m.linger:
		cs = append(cs, onSurface(lingerOnStyle).Render(glyph(gActive)+" linger: on"))
	default:
		cs = append(cs, onSurface(lingerOffStyle).Render(glyph(gInactive)+" linger: off"))
	}
	if m.readonly {
		cs = append(cs, onSurface(warnStyle.Bold(true)).Render("readonly"))
	}
	return strings.Join(cs, sep) + onSurface(lipgloss.NewStyle()).Render(" ")
}

// subNote is the sub-state after the state word, unless it repeats it.
func subNote(st unitState) string {
	if st.sub == "" || st.sub == "-" || st.sub == st.word {
		return ""
	}
	return "  " + st.sub
}
