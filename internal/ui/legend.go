package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m Model) keys() keyMap {
	switch m.mode {
	case modeDetail:
		return detailKeys()
	case modeEvents:
		return detailKeys()
	case modeUpdates, modeValidate, modeTree, modeStorage, modeGenerate, modeRecent:
		return updatesKeys()
	}
	return listKeys()
}

func (m Model) helpBar() string {
	chip := lingerUnknownStyle.Render("linger: ?")
	if m.system {
		chip = lingerOnStyle.Render("system")
	} else if m.lingerKnown {
		if m.linger {
			chip = lingerOnStyle.Render("linger: on")
		} else {
			chip = lingerOffStyle.Render("linger: off")
		}
	}
	if m.showHelp {
		m.help.ShowAll = true
		systemOrLingerHelp := "Linger keeps rootless containers running after logout - enable it once on every quadlet host (loginctl enable-linger)."
		if m.system {
			systemOrLingerHelp = "System mode: managing system-wide Quadlet units in /run/containers/systemd, /etc/containers/systemd, /usr/share/containers/systemd."
		}
		lines := []string{
			m.help.View(m.keys()) + "  ·  " + chip,
			systemOrLingerHelp,
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e adds [Install] WantedBy=default.target to the quadlet file so the unit starts at boot; d removes it (newer systemd refuses 'systemctl enable' on generated units).",
			"x stops the unit; quadlet runs containers with --rm, so stopping removes the container (state lives in volumes).",
			"E edits in your editor; the first use asks once and saves the choice to ~/.config/quadman/config.json (delete that file to re-pick).",
			"A ~ after a name means the file lives in a quadlet_dirs extra dir the generator cannot see.",
			"Quadlet search order: " + strings.Join(m.searchDirs(), " → "),
		}
		return helpStyle.Render(clampLines(strings.Join(lines, "\n"), m.help.Width()))
	}

	// Compact legend: full words; one line when it fits, two when narrow.
	bar := strings.Join(m.legend(), "\n")
	ls := strings.Split(bar, "\n")
	chips := "  ·  " + chip
	if m.readonly {
		chips += "  ·  " + lingerOffStyle.Render("readonly")
	}
	if w := m.help.Width(); w > 0 && ansi.StringWidth(ls[len(ls)-1])+ansi.StringWidth(chips) > w {
		ls = append(ls, strings.TrimPrefix(chips, "  ·  "))
	} else {
		ls[len(ls)-1] += chips
	}
	return helpStyle.Render(clampLines(strings.Join(ls, "\n"), m.help.Width()))
}

// legendChipReserve reserves room for the " · linger: on/off" chip when
// deciding whether the full legend fits on one line.
const legendChipReserve = 16

// legend returns the compact key hints for the current mode. In list mode it
// prefers a single line, wrapping to two lines (views, then unit actions)
// only when the terminal is too narrow for the full legend.
func (m Model) legend() []string {
	switch m.mode {
	case modeDetail:
		if m.tab == tabJournal {
			return []string{"[/] tabs · f pause/resume · / search · F grep · p prio · S export · c copy · esc/q back"}
		}
		return []string{"[/] tabs · E edit · ↑/↓ scroll · esc/q back"}
	case modeStorage:
		return []string{"r refresh · ↑/↓ scroll · esc/q back"}
	case modeEvents:
		return []string{"f pause/resume · ↑/↓ scroll · esc/q back"}
	case modeGenerate:
		return []string{"y write & reload · esc cancel · ↑/↓ scroll"}
	case modeUpdates:
		return []string{"U toggle timer · r refresh · esc/q back"}
	case modeRecent:
		return []string{"esc/q back"}
	}
	w := m.help.Width()
	keys := legendKeys()
	if w > 0 && w < legendFullWidth {
		keys = legendKeysCompact()
	}
	reserve := legendChipReserve
	if m.readonly {
		reserve += 14
	}
	full := strings.Join(keys, " · ")
	if w <= 0 || ansi.StringWidth(full)+reserve <= w {
		return []string{full}
	}
	return splitLegend(keys, w, reserve)
}

// legendFullWidth is the terminal width below which the legend falls back
// to the compact subset so nothing is truncated.
const legendFullWidth = 170

// legendKeys returns every list-mode key hint sorted alphabetically by key
// (symbols first, lowercase before uppercase within a letter).
func legendKeys() []string {
	return []string{
		"/ filter", "? all keys",
		"A actions", "d disable", "D delete",
		"e enable", "E edit",
		"g storage", "h healthcheck",
		"i instantiate", "I install",
		"l logs", "L linger",
		"n generate",
		"q quit",
		"r restart", "R reload",
		"s start", "space mark",
		"t tree",
		"u updates",
		"v problems",
		"w events",
		"x stop",
		"y/Y copy",
		"enter file",
	}
}

// legendKeysCompact is the essential subset for narrow terminals, also
// alphabetically sorted.
func legendKeysCompact() []string {
	return []string{
		"/ filter", "? all keys",
		"d disable",
		"e enable", "E edit",
		"l logs", "L linger",
		"q quit",
		"r restart", "R reload",
		"s start", "space mark",
		"t tree",
		"u updates",
		"v problems",
		"x stop",
		"enter file",
	}
}

// splitLegend breaks the sorted key list into two lines, balancing keys
// across lines so that the second line leaves room for chips (reserve)
// without overflowing the terminal width.
func splitLegend(keys []string, width, reserve int) []string {
	if width <= 0 {
		return []string{strings.Join(keys, " · ")}
	}
	bestSplit := -1
	for i := 1; i < len(keys); i++ {
		first := strings.Join(keys[:i], " · ")
		second := strings.Join(keys[i:], " · ")
		if ansi.StringWidth(first) <= width && ansi.StringWidth(second)+reserve <= width {
			bestSplit = i
		}
	}
	if bestSplit > 0 {
		return []string{strings.Join(keys[:bestSplit], " · "), strings.Join(keys[bestSplit:], " · ")}
	}
	var first []string
	curW := 0
	i := 0
	for ; i < len(keys); i++ {
		kw := ansi.StringWidth(keys[i])
		if curW > 0 {
			kw += 3 // separator " · "
		}
		if curW > 0 && curW+kw > width {
			break
		}
		curW += kw
		first = append(first, keys[i])
	}
	if i == 0 || i >= len(keys) {
		return []string{strings.Join(keys, " · ")}
	}
	return []string{strings.Join(first, " · "), strings.Join(keys[i:], " · ")}
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
