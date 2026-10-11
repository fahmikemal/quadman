package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) keys() keyMap {
	switch m.mode {
	case modeDetail:
		return detailKeys()
	case modeEvents:
		return detailKeys()
	case modeUpdates, modeValidate, modeTree, modeStorage, modeGenerate, modeRecent, modeTimers, modeSecrets:
		return updatesKeys()
	}
	return listKeys()
}

// helpBar is the footer: the grouped legend (3a), or the expanded help
// block while ? is toggled on.
func (m Model) helpBar() string {
	if m.showHelp {
		m.help.ShowAll = true
		systemOrLingerHelp := "Linger keeps rootless containers running after logout - enable it once on every quadlet host (loginctl enable-linger)."
		if m.system {
			systemOrLingerHelp = "System mode: managing system-wide Quadlet units in /run/containers/systemd, /etc/containers/systemd, /usr/share/containers/systemd."
		}
		lines := []string{
			m.help.View(m.keys()),
			systemOrLingerHelp,
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e adds [Install] WantedBy=default.target to the quadlet file so the unit starts at boot; d removes it (newer systemd refuses 'systemctl enable' on generated units).",
			"x stops the unit; quadlet runs containers with --rm, so stopping removes the container (state lives in volumes).",
			"E edits in your editor; the first use asks once and saves the choice to ~/.config/quadman/config.json (delete that file to re-pick).",
			"Markers after a name: " + glyph(gFailed) + " validation error · " + glyph(gWarn) + " warning · " + glyph(gExternal) + " file lives in a quadlet_dirs extra dir the generator cannot see.",
			"S cycles the list order (problems first / name / kind); z collapses the dashboard tiles.",
			"Quadlet search order: " + strings.Join(m.searchDirs(), " → "),
		}
		return helpStyle.Render(clampLines(strings.Join(lines, "\n"), m.help.Width()))
	}
	w := m.help.Width()
	if w <= 0 {
		w, _ = m.size()
	}
	groups := m.legendGroups()
	if w < breakpointMid {
		// Narrow: group labels go, the keys stay.
		for i := range groups {
			groups[i].label = ""
		}
	}
	return strings.Join(layoutLegend(groups, w), "\n")
}

// legendGroup is one labeled set of key hints in the footer.
type legendGroup struct {
	label string
	keys  [][2]string // {key, label}
}

// legendTail is always at the right end of the footer.
var legendTail = [][2]string{{"^P", "all commands"}, {"?", "help"}}

// maxLegendRows caps the footer height; groups that do not fit are left
// to ? and Ctrl+P (DESIGN.md principle 3).
const maxLegendRows = 2

// legendGroups returns the footer groups for the current mode. Each mode
// defines its own set; the unit list adds a SELECT group from 160 columns
// and collapses to unlabeled UNIT keys below 100.
func (m Model) legendGroups() []legendGroup {
	back := [2]string{"esc", "back"}
	scroll := [2]string{"↑/↓", "scroll"}
	switch m.mode {
	case modeDetail:
		if m.tab == tabJournal {
			return []legendGroup{
				{"JOURNAL", [][2]string{{"f", "pause/resume"}, {"/", "search"}, {"F", "grep"}, {"p", "prio"}, {"S", "export"}, {"c", "copy"}}},
				{"NAV", [][2]string{{"[/]", "tabs"}, back}},
			}
		}
		return []legendGroup{{"NAV", [][2]string{{"[/]", "tabs"}, scroll, back}}, {"FILE", [][2]string{{"E", "edit"}}}}
	case modeStorage:
		return []legendGroup{
			{"STORAGE", [][2]string{{"p", "pull"}, {"D", "remove"}, {"P", "prune unused"}, {"y", "copy ref"}, {"/", "filter"}, {"S", "sort"}}},
			{"NAV", [][2]string{{"[ ]", "tabs"}, {"↵", "open unit"}, {"r", "refresh"}, back}},
		}
	case modeEvents:
		return []legendGroup{
			{"EVENTS", [][2]string{{"f", "pause"}, {"1-6", "type"}, {"u", "this unit only"}, {"s", "since"}, {"/", "filter"}, {"S", "export"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeTimers:
		return []legendGroup{
			{"TIMER", [][2]string{{"n", "run now"}, {"s", "start"}, {"x", "stop"}, {"e/d", "boot"}, {"l", "last run log"}, {"E", "edit"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, {"r", "refresh"}, back}},
		}
	case modeSecrets:
		return []legendGroup{
			{"SECRET", [][2]string{{"n", "create"}, {"r", "rotate"}, {"c", "copy value"}, {"D", "remove"}, {"/", "filter"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, {"^R", "refresh"}, back}},
		}
	case modeRecent:
		return []legendGroup{
			{"HISTORY", [][2]string{{".", "repeat"}, {"c", "copy command"}, {"v", "related problem"}, {"l", "logs"}, {"S", "export"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeStats:
		return []legendGroup{
			{"STATS", [][2]string{{"←→", "sort"}, {"i", "idle"}, {"r", "refresh"}, {"l", "logs"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeUpdates:
		return []legendGroup{
			{"UPDATES", [][2]string{{"a", "apply now"}, {"r", "re-check"}, {"U", "toggle timer"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeValidate:
		return []legendGroup{
			{"PROBLEMS", [][2]string{{"E", "edit at line"}, {"R", "reload"}, {"y", "copy"}, {"/", "filter"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeTree:
		return []legendGroup{
			{"TREE", [][2]string{{"←→", "fold"}, {"z", "all"}, {"f", "problems only"}, {"i", "impact"}}},
			{"NAV", [][2]string{{"↵", "open unit"}, back}},
		}
	case modeGenerate:
		return []legendGroup{{"PREVIEW", [][2]string{{"y", "write & reload"}, {"b", "boot"}, {"space", "start"}, {"tab", "next file"}, {"esc", "cancel"}}}}
	case modePalette:
		return []legendGroup{{"PALETTE", [][2]string{{"↑/↓", "move"}, {"enter", "run"}, {"esc", "close"}}}}
	}
	unit := legendGroup{"UNIT", [][2]string{{"s", "start"}, {"x", "stop"}, {"r", "restart"}, {"e/d", "boot"}, {"E", "edit"}, {"D", "delete"}}}
	w, _ := m.size()
	if w < breakpointMid {
		unit.label = ""
		unit.keys = [][2]string{{"s", "start"}, {"x", "stop"}, {"r", "restart"}, {"l", "logs"}, {"/", "filter"}}
		return []legendGroup{unit}
	}
	groups := []legendGroup{
		unit,
		{"INSPECT", [][2]string{{"enter", "file"}, {"l", "logs"}, {"h", "health"}, {"X", "shell"}}},
		{"VIEWS", [][2]string{{"o", "stats"}, {"t", "tree"}, {"v", "problems"}, {"u", "updates"}, {"g", "storage"}}},
		{"HOST", [][2]string{{"R", "reload"}, {"L", "linger"}}},
	}
	if w >= breakpointWide {
		groups = append(groups, legendGroup{"SELECT", [][2]string{{"/", "filter"}, {"space", "mark"}, {"y/Y", "copy"}, {"i", "instantiate"}, {"S", "sort"}}})
	}
	return groups
}

// renderGroup draws "LABEL    k label  k label"; label dim, key accent,
// description muted.
func renderGroup(g legendGroup) string {
	var parts []string
	for _, k := range g.keys {
		parts = append(parts, keyHint(k[0], k[1]))
	}
	s := strings.Join(parts, "  ")
	if g.label != "" {
		s = dimStyle.Render(padCell(g.label, 9)) + s
	}
	return s
}

// layoutLegend flows the groups into at most maxLegendRows rows of width
// w, with the ^P/? tail right-aligned on the last row. When the tail does
// not fit after the last group, that group moves down a row first; only
// when no arrangement fits are groups dropped whole, from the end (they
// stay reachable through ? and Ctrl+P). A single group wider than the
// terminal is truncated.
func layoutLegend(groups []legendGroup, w int) []string {
	tail := renderGroup(legendGroup{keys: legendTail})
	tailW := ansi.StringWidth(tail)
	const sep = "   "
	width := func(row []string) int {
		return ansi.StringWidth(strings.Join(row, sep))
	}
	for n := len(groups); n > 0; n-- {
		var rows [][]string
		var cur []string
		for _, g := range groups[:n] {
			r := renderGroup(g)
			if len(cur) > 0 && width(append(slices.Clone(cur), r)) > w {
				rows = append(rows, cur)
				cur = nil
			}
			cur = append(cur, r)
		}
		rows = append(rows, cur)
		last := len(rows) - 1
		if width(rows[last])+2+tailW > w && len(rows[last]) > 1 && len(rows) < maxLegendRows {
			moved := rows[last][len(rows[last])-1]
			rows[last] = rows[last][:len(rows[last])-1]
			rows = append(rows, []string{moved})
			last++
		}
		fits := len(rows) <= maxLegendRows && width(rows[last])+2+tailW <= w
		if !fits && n > 1 {
			continue
		}
		out := make([]string, len(rows))
		for i, row := range rows {
			out[i] = strings.Join(row, sep)
		}
		pad := max(w-ansi.StringWidth(out[last])-tailW, 2)
		out[last] += strings.Repeat(" ", pad) + tail
		if len(out) > maxLegendRows {
			out = out[len(out)-maxLegendRows:]
		}
		for i, r := range out {
			out[i] = ansi.Truncate(r, w, "…")
		}
		return out
	}
	return []string{strings.Repeat(" ", max(w-tailW, 0)) + tail}
}

// clampLines truncates every line to w columns; bubbles/help only truncates
// while an ellipsis still fits and then happily overflows, so we clamp here.
func clampLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		if ansi.StringWidth(l) > w {
			ls[i] = ansi.Truncate(l, w, "…")
		}
	}
	return strings.Join(ls, "\n")
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
