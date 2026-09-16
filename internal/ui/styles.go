package ui

import (
	"os"

	"charm.land/lipgloss/v2"
)

// Theme selects the UI color scheme. "auto" follows the terminal background
// (light terminals get the light scheme, everything else dark).
type Theme string

const (
	ThemeAuto       Theme = "auto"
	ThemeDark       Theme = "dark"
	ThemeLight      Theme = "light"
	ThemeColorblind Theme = "colorblind"
)

// palette is one complete color scheme. The colorblind scheme avoids the
// red-green pairing (≈8% of men cannot tell them apart): success is blue,
// failure is vermilion/yellow, each always paired with a text glyph
// (ok/FAIL/✗/⚠) that carries the meaning without color.
type palette struct {
	titleFg, titleBg string
	headerFg         string
	help             string
	ok               string
	err              string
	dimErr           string
	warn             string
	filter           string
	lingerOn         string
	lingerOff        string
	lingerUnknown    string
	tabActiveFg      string
	tabActiveBg      string
	tabInactive      string
}

var (
	darkPalette = palette{
		titleFg: "15", titleBg: "62", headerFg: "62", help: "241",
		ok: "42", err: "203", dimErr: "174", warn: "214", filter: "39",
		lingerOn: "42", lingerOff: "203", lingerUnknown: "245",
		tabActiveFg: "15", tabActiveBg: "62", tabInactive: "245",
	}
	lightPalette = palette{
		titleFg: "15", titleBg: "62", headerFg: "62", help: "240",
		ok: "28", err: "160", dimErr: "131", warn: "172", filter: "25",
		lingerOn: "28", lingerOff: "160", lingerUnknown: "242",
		tabActiveFg: "15", tabActiveBg: "62", tabInactive: "242",
	}
	colorblindPalette = palette{
		titleFg: "15", titleBg: "62", headerFg: "62", help: "241",
		ok: "33", err: "214", dimErr: "180", warn: "226", filter: "39",
		lingerOn: "33", lingerOff: "214", lingerUnknown: "245",
		tabActiveFg: "15", tabActiveBg: "62", tabInactive: "245",
	}
)

var (
	titleStyle         lipgloss.Style
	headerStyle        lipgloss.Style
	helpStyle          lipgloss.Style
	okStyle            lipgloss.Style
	errStyle           lipgloss.Style
	dimErrStyle        lipgloss.Style
	warnStyle          lipgloss.Style
	filterStyle        lipgloss.Style
	lingerOnStyle      lipgloss.Style
	lingerOffStyle     lipgloss.Style
	lingerUnknownStyle lipgloss.Style
	tabActiveStyle     lipgloss.Style
	tabInactiveStyle   lipgloss.Style
)

// activeTheme is the resolved scheme name (for tests and the help text).
var activeTheme Theme

func init() {
	applyTheme(ThemeDark)
}

// applyTheme rebuilds every style from the named palette.
func applyTheme(t Theme) {
	if t == ThemeAuto {
		t = ThemeDark
		if !lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
			t = ThemeLight
		}
	}
	var p palette
	switch t {
	case ThemeLight:
		p = lightPalette
	case ThemeColorblind:
		p = colorblindPalette
	default:
		p = darkPalette
		t = ThemeDark
	}
	activeTheme = t
	titleStyle = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(p.titleFg)).
		Background(lipgloss.Color(p.titleBg)).
		Padding(0, 1)
	headerStyle = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(p.headerFg)).
		Padding(0, 1)
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.help))
	okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.ok))
	errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.err))
	dimErrStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.dimErr))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.warn))
	filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.filter))
	lingerOnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.lingerOn)).Bold(true)
	lingerOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.lingerOff)).Bold(true)
	lingerUnknownStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.lingerUnknown)).Bold(true)
	tabActiveStyle = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(p.tabActiveFg)).
		Background(lipgloss.Color(p.tabActiveBg))
	tabInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(p.tabInactive))
}

// resolveTheme maps the config string to a Theme, defaulting to dark.
func resolveTheme(s string) Theme {
	switch Theme(s) {
	case ThemeAuto, ThemeDark, ThemeLight, ThemeColorblind:
		return Theme(s)
	default:
		return ThemeDark
	}
}
