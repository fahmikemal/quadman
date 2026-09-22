package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// execFinishedMsg reports the outcome of an interactive `podman exec`
// session after the terminal is handed back.
type execFinishedMsg struct {
	err error
}

// execBaseName is the prefilled container guess for a unit: Quadlet names
// the container after the service (webapp.service -> webapp), but pods,
// volumes, and renamed services differ, so the user can always correct it.
func execBaseName(unit string) string {
	return strings.TrimSuffix(unit, ".service")
}

// execOpenKeys arms the exec prompt (X) on the selected unit.
func (m Model) execOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "X" {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	if m.ssh.IsRemote() {
		m.setStatus("exec unavailable over SSH (an interactive TTY cannot cross the batch runner)", true)
		return m, nil, true
	}
	if m.noEditor {
		m.setStatus("exec unavailable in served sessions (connect with ssh and run podman exec yourself)", true)
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		m.setStatus("no unit selected", true)
		return m, nil, true
	}
	prefill := ""
	if u.Kind == quadlet.KindContainer {
		// Quadlet names the container systemd-<name> (same convention
		// healthcheck uses); other kinds have no single container, so
		// the prompt stays empty for the user to fill.
		prefill = "systemd-" + execBaseName(u.UnitName)
	}
	m.execing = true
	m.execIn.SetValue(prefill)
	m.execIn.Focus()
	return m, nil, true
}

// execKeys handles the exec-name input: enter resolves the name against
// `podman ps -a` (exact-only, never guessed) and hands the terminal to
// `podman exec -it <name> /bin/sh`; esc cancels.
func (m Model) execKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.execing {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		name := strings.TrimSpace(m.execIn.Value())
		if name == "" || strings.ContainsAny(name, " \t\n") {
			m.setStatus("exec cancelled (empty container name)", false)
			m.execing = false
			m.execIn.Blur()
			return m, nil, true
		}
		m.execing = false
		m.execIn.Blur()
		return m, execResolveCmd(name), true
	case "esc":
		m.execing = false
		m.execIn.SetValue("")
		m.execIn.Blur()
		m.setStatus("exec cancelled", false)
		return m, nil, true
	}
	var cmd tea.Cmd
	m.execIn, cmd = m.execIn.Update(msg)
	return m, cmd, true
}

type execResolveMsg struct {
	name  string
	found bool
	err   error
}

// execResolveCmd confirms the container exists (through the session
// runner, so compartments resolve the target's containers) before the
// terminal handover, so a typo fails with a message instead of a dead
// shell.
func execResolveCmd(name string) tea.Cmd {
	return func() tea.Msg {
		names, err := podman.ContainerNames(context.Background())
		if err != nil {
			return execResolveMsg{name: name, err: err}
		}
		return execResolveMsg{name: name, found: podman.ResolveExecTarget(names, name)}
	}
}

// execShell hands the terminal to podman exec through the session runner:
// locally direct, or under sudo for compartments (non-interactive sudo
// fails fast instead of prompting mid-handover). Argv is fixed and the
// name was exact-matched (or typed as one word), so no shell is ever
// involved.
func (m Model) execShell(name string) tea.Cmd {
	cmd := m.ssh.Command(context.Background(), "podman", "exec", "-it", name, "/bin/sh") // #nosec G204 -- fixed argv via session runner, no shell
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return execFinishedMsg{err} })
}
