package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Logo 1a ("Quad Grid"): four units, the one you manage in accent. The
// block-art splash is what `quadman -version` prints on a terminal; below
// 80 columns it falls back to the two-line compact mark.

// splashMark is the 2×2 grid; the bottom-right cell is the accent one.
var splashMark = [5][2]string{
	{"████", "████"},
	{"████", "████"},
	{"    ", "    "},
	{"████", "████"},
	{"████", "████"},
}

// splashWord is the "quadman" wordmark in half blocks, one row per mark row.
var splashWord = [5]string{
	"                         ██",
	"▄█▀▀██ ██  ██ ▀▀▀▀█▄ ▄█▀▀██ ██▀██▀█▄ ▀▀▀▀█▄ ██▀▀█▄",
	"██  ██ ██  ██ ▄█▀▀██ ██  ██ ██ ██ ██ ▄█▀▀██ ██  ██",
	" ▀▀▀██  ▀▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀▀ ▀▀ ▀▀ ▀▀  ▀▀▀▀▀ ▀▀  ▀▀",
	"    ▀▀",
}

// Splash renders the version splash. info is the "podman … · systemd …"
// line; width < 80 selects the compact form.
func Splash(version, info string, width int) string {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(activePalette.accent))
	fg := lipgloss.NewStyle().Foreground(lipgloss.Color(activePalette.fg))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color(activePalette.bright)).Bold(true)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color(activePalette.muted))
	tagline := "rootless quadlet manager for systemd + podman"
	if width > 0 && width < 80 {
		return strings.Join([]string{
			fg.Render("▀▀ ") + fg.Render("▀▀") + "  " + bright.Render("quadman "+version),
			fg.Render("▀▀ ") + accent.Render("▀▀") + "  " + muted.Render("rootless quadlet manager"),
		}, "\n")
	}
	var lines []string
	for i, row := range splashMark {
		right := fg.Render(row[1])
		if i >= 3 {
			right = accent.Render(row[1])
		}
		lines = append(lines, fg.Render(row[0])+"  "+right+"   "+bright.Render(splashWord[i]))
	}
	lines = append(lines,
		bright.Render("quadman "+version)+muted.Render("  ·  "+tagline),
	)
	if info != "" {
		lines = append(lines, muted.Render(info))
	}
	return strings.Join(lines, "\n")
}

// compactMark is the 2-row mark used by the too-small screen.
func compactMark() string {
	if asciiGlyphs {
		return "[q] quadman"
	}
	return fgStyle.Render("▀▀ ▀▀") + "  " + brightStyle.Render("quadman") + "\n" + fgStyle.Render("▀▀ ") + accentStyle.Render("▀▀")
}
