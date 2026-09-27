package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
)

// This file holds program startup: CLI-level options, constructors with
// overrides, and the SSH/compartment wiring.

// Options are the CLI-level overrides applied on top of config.yaml.
type Options struct {
	Readonly    bool
	SSH         string
	Compartment string
	Mouse       bool
	Theme       string
	QuadletDirs []string
	ClientInfo  string
	NoEditor    bool
	System      bool
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
func NewWithOptions(o Options) Model {
	m := New()
	if o.SSH != "" {
		m.applySSH(o.SSH)
	}
	if o.Compartment != "" {
		m.applyCompartment(o.Compartment)
	}
	if o.Readonly {
		m.readonly = true
	}
	if o.System {
		m.system = true
		m.sys.User = false
	}
	if o.Mouse {
		m.mouse = true
	}
	if o.Theme != "" {
		applyTheme(resolveTheme(o.Theme))
	}
	if len(o.QuadletDirs) > 0 {
		applyExtraDirs(o.QuadletDirs)
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

// RunWithOptions starts the quadman TUI with CLI overrides.
func RunWithOptions(o Options) error {
	m := NewWithOptions(o)
	_, err := tea.NewProgram(m).Run()
	return err
}

// applyExtraDirs appends user-configured Quadlet source directories to the
// discovery search path (config.yaml quadlet_dirs plus --quadlet-dir
// flags). Empty and duplicate entries are ignored, so calling it twice
// (once from main, once from New) is safe.
func applyExtraDirs(dirs []string) {
	seen := map[string]bool{}
	for _, d := range quadlet.ExtraDirs {
		seen[d] = true
	}
	for _, d := range dirs {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		quadlet.ExtraDirs = append(quadlet.ExtraDirs, d)
	}
}

// RunWithSSH starts the quadman TUI against a remote host: every CLI call
// (systemctl, journalctl, loginctl, podman, podlet) runs over SSH, and
// file-mutating actions are disabled with an explanation.
func RunWithSSH(target string, readonly bool) error {
	return RunWithOptions(Options{SSH: target, Readonly: readonly})
}

// applySSH points every CLI client at the remote target.
func (m *Model) applySSH(target string) {
	m.ssh = remote.Runner{Target: target}
	m.sys.Remote = m.ssh
	m.lc.Remote = m.ssh
	podman.DefaultRunner = m.ssh
	podlet.DefaultRunner = m.ssh
}
