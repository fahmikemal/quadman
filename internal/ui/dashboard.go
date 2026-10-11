package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// This file is the unit-list dashboard (DESIGN.md layout 2c): six summary
// tiles and the "needs attention" rows above the table, or one summary
// line when the terminal is narrow or short.

// dashCounts aggregates the unit list for the tiles and the summary line.
type dashCounts struct {
	units, running, exitedOK, failed, unhealthy, starting, notFound int
	pods, apps                                                      int
	pendingUpdates                                                  int
	updatesKnown                                                    bool
}

func (m Model) counts() dashCounts {
	var c dashCounts
	apps := map[string]bool{}
	for _, u := range m.units {
		c.units++
		if u.Kind == quadlet.KindPod {
			c.pods++
		}
		if e := m.podmanInfo[u.UnitName]; e.App != "" {
			apps[e.App] = true
		}
		st := m.stateOf(u)
		switch st.glyph {
		case gActive:
			c.running++
			if st.sub == "exited" {
				c.exitedOK++
			}
		case gFailed:
			c.failed++
		case gUnhealthy:
			c.unhealthy++
		case gStarting:
			c.starting++
		case gNotFound:
			c.notFound++
		}
	}
	c.apps = len(apps)
	if m.updateEntries != nil {
		c.updatesKnown = true
		for _, e := range m.updateEntries {
			if e.Updated == "pending" {
				c.pendingUpdates++
			}
		}
	}
	return c
}

// dashExpanded reports whether the full tiles + attention block fits:
// at least 100 columns and 30 rows, and not collapsed with z.
func (m Model) dashExpanded() bool {
	w, h := m.size()
	return !m.dashOff && w >= breakpointMid && h >= 30
}

// tile is one dashboard cell: a dim label, a colored value, a muted note.
type tile struct {
	label, value, note string
	valueStyle         lipgloss.Style
	tint               string // border color; "" = border token
}

func (m Model) tiles() []tile {
	c := m.counts()
	p := activePalette

	units := tile{label: "UNITS", value: fmt.Sprint(c.units), valueStyle: brightStyle}
	switch {
	case c.apps > 0 || c.pods > 0:
		units.note = fmt.Sprintf("%d app%s · %d pod%s", c.apps, plural(c.apps), c.pods, plural(c.pods))
	default:
		units.note = "quadlet files"
	}

	running := tile{label: "RUNNING", value: glyph(gActive) + " " + fmt.Sprint(c.running), valueStyle: okStyle,
		note: fmt.Sprintf("of %d", c.units)}
	if c.exitedOK > 0 {
		running.note = fmt.Sprintf("%d exited ok", c.exitedOK)
	}

	failed := tile{label: "FAILED", value: glyph(gFailed) + " " + fmt.Sprint(c.failed), valueStyle: errStyle,
		note: fmt.Sprintf("%d unhealthy", c.unhealthy)}
	switch {
	case c.failed > 0:
		failed.tint = p.err
	case c.unhealthy > 0:
		failed.tint = p.warn
	default:
		failed.valueStyle = mutedStyle
	}

	gen := tile{label: "GENERATOR", value: glyph(gActive) + " in sync", valueStyle: okStyle, note: "files loaded"}
	switch {
	case len(m.stale) > 0:
		gen = tile{label: "GENERATOR", value: glyph(gWarn) + fmt.Sprintf(" %d stale", len(m.stale)),
			valueStyle: warnStyle, note: "press R", tint: p.warn}
	case c.notFound > 0:
		gen = tile{label: "GENERATOR", value: glyph(gNotFound) + fmt.Sprintf(" %d not loaded", c.notFound),
			valueStyle: warnStyle, note: "press R", tint: p.warn}
	}

	linger := tile{label: "LINGER", value: "? unknown", valueStyle: mutedStyle, note: "loginctl"}
	switch {
	case m.system:
		linger = tile{label: "SCOPE", value: glyph(gActive) + " system", valueStyle: okStyle, note: "rootful units"}
	case m.lingerKnown && m.linger:
		linger = tile{label: "LINGER", value: glyph(gActive) + " on", valueStyle: okStyle, note: "survives logout"}
	case m.lingerKnown:
		linger = tile{label: "LINGER", value: glyph(gInactive) + " off", valueStyle: warnStyle,
			note: "stops at logout · L", tint: p.warn}
	}

	upd := tile{label: "AUTO-UPDATE", value: "—", valueStyle: mutedStyle, note: "press u to check"}
	if c.updatesKnown {
		if c.pendingUpdates > 0 {
			upd.value = fmt.Sprintf("%d pending", c.pendingUpdates)
			upd.valueStyle = accentStyle
		} else {
			upd.value = "up to date"
			upd.valueStyle = okStyle
		}
		upd.note = "timer " + orDash(m.timerActive)
	}
	return []tile{units, running, failed, gen, linger, upd}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// tilesView lays the six dashboard tiles side by side across width.
func (m Model) tilesView(width int) string {
	if (width-5)/6 < 12 {
		return m.summaryLine(width)
	}
	return tilesRow(m.tiles(), width)
}

// tilesRow lays tiles side by side across width: a dim label, a bold
// colored value, a muted note, and a border tinted when it needs a look.
func tilesRow(ts []tile, width int) string {
	n := len(ts)
	if n == 0 {
		return ""
	}
	gap := 1
	tw := (width - gap*(n-1)) / n
	if tw < 8 {
		return tilesSummary(ts, width)
	}
	var cells []string
	for i, t := range ts {
		w := tw
		if i == n-1 { // the last tile absorbs the rounding remainder
			w = width - (tw+gap)*(n-1)
		}
		border := activePalette.border
		if t.tint != "" {
			border = t.tint
		}
		inner := w - 4 // border + 1 padding each side
		label := dimStyle.Render(ansi.Truncate(t.label, inner, "…"))
		value := t.valueStyle.Bold(true).Render(ansi.Truncate(t.value, inner, "…"))
		note := mutedStyle.Render(ansi.Truncate(t.note, inner, "…"))
		cells = append(cells, lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(border)).
			Padding(0, 1).
			Width(w).
			Render(label+"\n"+value+"\n"+note))
		if i < n-1 {
			cells = append(cells, strings.Repeat(" ", gap))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

// summaryLine is the one-line dashboard for narrow or short terminals:
// "16 units · ● 11 · ✗ 2 · ▲ 1 · ⚠ 2 stale".
func (m Model) summaryLine(width int) string {
	c := m.counts()
	parts := []string{fgStyle.Render(fmt.Sprintf("%d units", c.units)),
		okStyle.Render(fmt.Sprintf("%s %d", glyph(gActive), c.running))}
	if c.failed > 0 {
		parts = append(parts, errStyle.Render(fmt.Sprintf("%s %d", glyph(gFailed), c.failed)))
	}
	if c.unhealthy > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%s %d", glyph(gUnhealthy), c.unhealthy)))
	}
	if len(m.stale) > 0 {
		parts = append(parts, warnStyle.Render(fmt.Sprintf("%s %d stale · R reload (daemon-reload)", glyph(gWarn), len(m.stale))))
	}
	if c.pendingUpdates > 0 {
		parts = append(parts, accentStyle.Render(fmt.Sprintf("%d update%s", c.pendingUpdates, plural(c.pendingUpdates))))
	}
	sep := dimStyle.Render(" · ")
	hint := ""
	if m.dashOff {
		hint = dimStyle.Render("   z expand")
	}
	return ansi.Truncate(" "+strings.Join(parts, sep)+hint, width, "…")
}

// attentionItem is one "needs attention" row: what is wrong and the keys
// that fix it (DESIGN.md principle 4: problems show the next action).
type attentionItem struct {
	glyph  glyphKind
	name   string
	reason string
	keys   [][2]string
}

// maxAttention caps the block; the rest stays one keypress away (v).
const maxAttention = 4

func (m Model) attention() []attentionItem {
	var items []attentionItem
	for _, u := range m.sortUnits(m.units) {
		st := m.stateOf(u)
		firstErr := ""
		for _, is := range m.issues[u.Name] {
			if is.Severity == quadlet.SeverityError {
				firstErr = is.Message
				break
			}
		}
		switch {
		case st.glyph == gFailed:
			if firstErr != "" { // the generator rejected the file: fix it first
				items = append(items, attentionItem{gFailed, u.Name, "failed · " + firstErr,
					[][2]string{{"v", "explain"}, {"E", "edit file"}}})
				continue
			}
			reason := "failed"
			if st.sub != "" && st.sub != "-" && st.sub != "failed" {
				reason += " · " + st.sub
			}
			items = append(items, attentionItem{gFailed, u.Name, reason,
				[][2]string{{"l", "logs"}, {"v", "explain"}, {"r", "restart"}}})
		case firstErr != "":
			items = append(items, attentionItem{gFailed, u.Name, firstErr,
				[][2]string{{"v", "explain"}, {"E", "edit file"}}})
		case st.glyph == gUnhealthy:
			items = append(items, attentionItem{gUnhealthy, u.Name, "unhealthy · healthcheck failing",
				[][2]string{{"h", "run healthcheck"}, {"l", "logs"}}})
		case st.glyph == gNotFound && !isExternalUnit(u.Path):
			items = append(items, attentionItem{gNotFound, u.Name, "not loaded · generator has not seen it",
				[][2]string{{"R", "reload generator"}}})
		}
	}
	if n := len(m.stale); n > 0 {
		items = append(items, attentionItem{gWarn, "",
			fmt.Sprintf("%d file%s changed since last daemon-reload (e.g. %s)", n, plural(n), m.stale[0].Name),
			[][2]string{{"R", "reload generator"}}})
	}
	return items
}

// attentionView renders the block; "" when nothing needs attention.
func (m Model) attentionView(width int) string {
	items := m.attention()
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(dimStyle.Bold(true).Render(" NEEDS ATTENTION"))
	shown := items
	if len(items) > maxAttention {
		shown = items[:maxAttention-1]
	}
	for _, it := range shown {
		b.WriteString("\n")
		b.WriteString(m.attentionRow(it, width))
	}
	if len(shown) < len(items) {
		b.WriteString("\n")
		more := fmt.Sprintf("   … %d more problems", len(items)-len(shown))
		b.WriteString(mutedStyle.Render(more) + "  " + keyHint("v", "problems"))
	}
	return b.String()
}

func (m Model) attentionRow(it attentionItem, width int) string {
	var keys []string
	for _, k := range it.keys {
		keys = append(keys, keyHint(k[0], k[1]))
	}
	right := strings.Join(keys, "   ")
	rightW := ansi.StringWidth(right)
	left := " " + glyphStyle(it.glyph).Render(glyph(it.glyph)) + " "
	text := it.reason
	if it.name != "" {
		text = it.name + " " + it.reason
	}
	room := width - 3 - rightW - 2
	if room < 10 { // too narrow for the keys: drop them
		room = width - 3
		right, rightW = "", 0
	}
	text = ansi.Truncate(text, room, "…")
	if it.name != "" && len(it.name) < len(text) {
		left += fgStyle.Bold(true).Render(it.name) + mutedStyle.Render(text[len(it.name):])
	} else {
		left += mutedStyle.Render(text)
	}
	pad := width - ansi.StringWidth(left) - rightW
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// keyHint renders "k label" with the key in accent and the label muted.
func keyHint(k, label string) string {
	return accentStyle.Render(k) + " " + mutedStyle.Render(label)
}
