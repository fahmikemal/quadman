package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kemal-labs/quadman/internal/quadlet"
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
	linger := lingerUnknownStyle.Render("linger: ?")
	if m.lingerKnown {
		if m.linger {
			linger = lingerOnStyle.Render("linger: on")
		} else {
			linger = lingerOffStyle.Render("linger: off")
		}
	}
	if m.showHelp {
		m.help.ShowAll = true
		lines := []string{
			m.help.View(m.keys()) + "  ·  " + linger,
			"Linger keeps rootless containers running after logout - enable it once on every quadlet host (loginctl enable-linger).",
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e adds [Install] WantedBy=default.target to the quadlet file so the unit starts at boot; d removes it (newer systemd refuses 'systemctl enable' on generated units).",
			"x stops the unit; quadlet runs containers with --rm, so stopping removes the container (state lives in volumes).",
			"E edits in your editor; the first use asks once and saves the choice to ~/.config/quadman/config.json (delete that file to re-pick).",
			"Quadlet search order: " + strings.Join(quadlet.SearchDirs(), " → "),
		}
		return helpStyle.Render(clampLines(strings.Join(lines, "\n"), m.help.Width()))
	}

	// Compact legend: full words; one line when it fits, two when narrow.
	bar := strings.Join(m.legend(), "\n")
	ls := strings.Split(bar, "\n")
	ls[len(ls)-1] += "  ·  " + linger
	if m.readonly {
		ls[len(ls)-1] += "  ·  " + lingerOffStyle.Render("readonly")
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
			return []string{"[/] tabs · f pause/resume · / search · n/N match · E edit · esc/q back"}
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
	full := strings.Join(keys, " · ")
	if w <= 0 || ansi.StringWidth(full)+legendChipReserve <= w {
		return []string{full}
	}
	return splitLegend(keys, w-legendChipReserve)
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
		"s start",
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
		"s start",
		"t tree",
		"u updates",
		"v problems",
		"x stop",
		"enter file",
	}
}

// splitLegend breaks the sorted key list into two lines, filling the first
// line up to the target width and putting the rest on the second.
func splitLegend(keys []string, target int) []string {
	var first []string
	w := 0
	i := 0
	for ; i < len(keys); i++ {
		kw := ansi.StringWidth(keys[i]) + 3 // separator
		if w > 0 && w+kw > target {
			break
		}
		w += kw
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
