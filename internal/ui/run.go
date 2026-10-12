package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
)

// This file holds program startup: process-wide presentation, CLI-level
// options, constructors with overrides, and the SSH/compartment wiring.

// Options are the CLI-level overrides applied on top of config.yaml.
type Options struct {
	Readonly    bool
	SSH         string
	Compartment string
	Mouse       bool
	// Theme and ASCII are process-wide (see Configure); NewWithOptions
	// ignores them, RunWithOptions applies them once.
	Theme       string
	ASCII       bool // ASCII glyph fallback (TERM=linux enables it too)
	QuadletDirs []string
	// ClientInfo identifies the connected client of a served session.
	ClientInfo string
	// NoEditor marks a served session: no editor, exec, secret copies,
	// local file writes, or session switches (see policy.go).
	NoEditor bool
	System   bool
}

// Configure sets the process-wide presentation: theme and glyph set, plus
// the extra Quadlet directories. Styles are shared by every session a
// process renders (quadman serve renders many at once), so they must be
// configured once, before the first session starts — never from a
// per-session constructor, where concurrent sessions would race on them.
// theme overrides config.yaml's; interactive false (a daemon without a
// terminal of its own) resolves "auto" to dark instead of probing stdio.
func Configure(theme string, ascii bool, extraDirs []string, cfg config.Config, interactive bool) {
	if theme == "" {
		theme = cfg.Settings.Theme
	}
	t := resolveTheme(theme)
	if t == ThemeAuto && !interactive {
		t = ThemeDark
	}
	applyTheme(t)
	if ascii {
		SetASCII(true)
	}
	applyExtraDirs(cfg.Settings.QuadletDirs)
	applyExtraDirs(extraDirs)
}

// Run starts the quadman TUI.
func Run() error {
	return RunWith(false)
}

// RunWith starts the quadman TUI, forcing readonly mode when readonly is
// true (from the --readonly flag).
func RunWith(readonly bool) error {
	return RunWithOptions(Options{Readonly: readonly})
}

// NewWithOptions returns a configured Model with CLI overrides applied.
// It only touches the new model: process-wide presentation is Configure's.
func NewWithOptions(o Options) Model {
	m := New()
	if o.System {
		m.system = true
		m.setSession(m.ssh, true, "")
	}
	if o.SSH != "" {
		m.applySSH(o.SSH)
	}
	if o.Compartment != "" {
		m.applyCompartment(o.Compartment)
	}
	if o.Readonly {
		m.readonly = true
	}
	if o.Mouse {
		m.mouse = true
	}
	if o.ClientInfo != "" {
		m.clientInfo = o.ClientInfo
	}
	if o.NoEditor {
		m.noEditor = true
	}
	return m
}

func (m Model) searchDirs() []string {
	if m.system {
		return quadlet.SearchDirsMode(true)
	}
	if m.compOn {
		return m.compDirs()
	}
	return quadlet.SearchDirsMode(false)
}

// RunWithOptions configures the process presentation and runs the TUI.
func RunWithOptions(o Options) error {
	cfg, _ := config.Load() // New reports config problems in the status line
	Configure(o.Theme, o.ASCII, o.QuadletDirs, cfg, true)
	m := NewWithOptions(o)
	_, err := tea.NewProgram(m).Run()
	return err
}

// applyExtraDirs appends user-configured Quadlet source directories to the
// discovery search path (config.yaml quadlet_dirs plus --quadlet-dir
// flags). Empty and duplicate entries are ignored, so calling it twice is
// safe.
func applyExtraDirs(dirs []string) {
	quadlet.AddExtraDirs(dirs)
}

// RunWithSSH starts the quadman TUI against a remote host: every CLI call
// (systemctl, journalctl, loginctl, podman, podlet) runs over SSH, and
// file-mutating actions are disabled with an explanation.
func RunWithSSH(target string, readonly bool) error {
	return RunWithOptions(Options{SSH: target, Readonly: readonly})
}

// localRunner is the operator's own session.
func localRunner() remote.Runner { return remote.Runner{} }
