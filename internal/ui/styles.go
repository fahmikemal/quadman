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

// palette is one complete color scheme (DESIGN.md "Palette"). The main
// background is never painted: only surfaces (top bar, status line, tiles,
// modals) and the selected row get a background, so the user's terminal
// background and font stay in charge. The colorblind scheme avoids the
// red-green pairing (≈8% of men cannot tell them apart): ok is blue, warn
// yellow, err orange, and every status also carries a glyph and a word.
type palette struct {
	surface  string
	selBg    string
	border   string
	dim      string
	muted    string
	fg       string
	bright   string
	accent   string
	onAccent string // text drawn on an accent background
	ok       string
	warn     string
	err      string
	kind     string
}

var (
	darkPalette = palette{
		surface: "#161B24", selBg: "#1C2838", border: "#262E3B",
		dim: "#5D6878", muted: "#8B95A5", fg: "#C9D1DB", bright: "#EEF2F6",
		accent: "#52CFF3", onAccent: "#0E1117",
		ok: "#6ED889", warn: "#F3BD5C", err: "#F97770", kind: "#B6ADF3",
	}
	lightPalette = palette{
		surface: "#E9ECF0", selBg: "#D9E6F3", border: "#CFD5DD",
		dim: "#6B7584", muted: "#4F5867", fg: "#272D35", bright: "#0E1117",
		accent: "#0074A6", onAccent: "#FFFFFF",
		ok: "#0A7E3A", warn: "#AA6A00", err: "#C53637", kind: "#6451AC",
	}
	colorblindPalette = palette{
		surface: "#161B24", selBg: "#1C2838", border: "#262E3B",
		dim: "#5D6878", muted: "#8B95A5", fg: "#C9D1DB", bright: "#EEF2F6",
		accent: "#52CFF3", onAccent: "#0E1117",
		ok: "#609EFA", warn: "#F2E15F", err: "#F88A3D", kind: "#B6ADF3",
	}
)

// activePalette is the resolved scheme; layout code reads raw colors from
// it (tile borders, surface fills) where a prebuilt style does not fit.
var activePalette palette

var (
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

	// Token styles for the redesigned chrome.
	dimStyle     lipgloss.Style
	mutedStyle   lipgloss.Style
	fgStyle      lipgloss.Style
	brightStyle  lipgloss.Style
	accentStyle  lipgloss.Style
	kindStyle    lipgloss.Style
	surfaceStyle lipgloss.Style
	logoStyle    lipgloss.Style
)

// activeTheme is the resolved scheme name (for tests and the help text).
var activeTheme Theme

func init() {
	applyTheme(ThemeDark)
	if os.Getenv("TERM") == "linux" {
		asciiGlyphs = true
	}
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
	activePalette = p
	fg := func(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }

	dimStyle = fg(p.dim)
	mutedStyle = fg(p.muted)
	fgStyle = fg(p.fg)
	brightStyle = fg(p.bright).Bold(true)
	accentStyle = fg(p.accent)
	kindStyle = fg(p.kind)
	surfaceStyle = fg(p.fg).Background(lipgloss.Color(p.surface))
	logoStyle = fg(p.bright).Background(lipgloss.Color(p.accent)).Bold(true)

	helpStyle = mutedStyle
	okStyle = fg(p.ok)
	errStyle = fg(p.err)
	dimErrStyle = dimStyle
	warnStyle = fg(p.warn)
	filterStyle = accentStyle
	lingerOnStyle = fg(p.ok).Bold(true)
	lingerOffStyle = fg(p.err).Bold(true)
	lingerUnknownStyle = mutedStyle.Bold(true)
	tabActiveStyle = fg(p.onAccent).Background(lipgloss.Color(p.accent)).Bold(true)
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

// onSurface returns s painted on the surface background, so segments
// joined inside a surface bar keep the fill after their own ANSI resets.
func onSurface(s lipgloss.Style) lipgloss.Style {
	return s.Background(lipgloss.Color(activePalette.surface))
}

// onSel is onSurface for the selected-row background.
func onSel(s lipgloss.Style) lipgloss.Style {
	return s.Background(lipgloss.Color(activePalette.selBg))
}

// asciiGlyphs swaps every status glyph for its ASCII fallback (TERM=linux,
// --ascii), for consoles whose font lacks the geometric shapes block.
var asciiGlyphs bool

// SetASCII forces the ASCII glyph fallback on or off.
func SetASCII(on bool) { asciiGlyphs = on }

// glyphKind names one entry of the status glyph table (DESIGN.md "Glyphs").
type glyphKind int

const (
	gActive glyphKind = iota
	gInactive
	gStarting
	gFailed
	gUnhealthy
	gNotFound
	gTemplate
	gWarn
	gExternal
	gMarked
	gDone    // action succeeded (status line)
	gRefresh // refresh interval chip
)

var glyphTable = map[glyphKind][2]string{
	gActive:    {"●", "*"},
	gInactive:  {"○", "o"},
	gStarting:  {"◐", "~"},
	gFailed:    {"✗", "x"},
	gUnhealthy: {"▲", "!"},
	gNotFound:  {"◌", "?"},
	gTemplate:  {"◇", "<>"},
	gWarn:      {"⚠", "!"},
	gExternal:  {"~", "~"}, // units outside the generator search path (quadlet_dirs)
	gMarked:    {"✱", "+"},
	gDone:      {"✓", "ok"},
	gRefresh:   {"⟳", "refresh"},
}

// glyph returns the Unicode glyph, or its ASCII fallback when enabled.
func glyph(k glyphKind) string {
	g := glyphTable[k]
	if asciiGlyphs {
		return g[1]
	}
	return g[0]
}

// glyphStyle is the color that goes with each glyph.
func glyphStyle(k glyphKind) lipgloss.Style {
	switch k {
	case gActive:
		return okStyle
	case gStarting, gMarked:
		return accentStyle
	case gFailed:
		return errStyle
	case gUnhealthy, gWarn:
		return warnStyle
	case gNotFound, gExternal:
		return dimStyle
	}
	return mutedStyle
}

// logoMark is the 1-cell quadman mark: a 2×2 grid with the bottom-right
// cell in accent, drawn as ▛ (bright on accent).
func logoMark() string {
	if asciiGlyphs {
		return "[q]"
	}
	return logoStyle.Render("▛")
}
