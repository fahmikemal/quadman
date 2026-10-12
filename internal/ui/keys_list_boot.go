package ui

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Boot enablement, edit, updates, and health actions.

func (m Model) enableKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "e" {
		return m, nil, false
	}
	if m.refuse(capMutate | capTargetFiles) {
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	// Boot start is declarative in quadlet: [Install] WantedBy= in the
	// file, and the generator wires it up on daemon-reload. Newer
	// systemd refuses `systemctl enable` for generated units.
	if quadlet.BootSource(u) != "" {
		sys := m.sys
		return m, tea.Batch(m.setBusy("start "+u.UnitName),
			m.actionCmd("start "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "start", u.UnitName)
			})), true
	}
	m.pending = &pendingAction{verb: "enable", unit: u}
	m.setStatus("enable at boot: add [Install] WantedBy="+m.bootTarget()+" to "+filepath.Base(u.Path)+"? [y/N]", false)
	return m, nil, true

}

func (m Model) disableKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "d" {
		return m, nil, false
	}
	if m.refuse(capMutate | capTargetFiles) {
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	switch src := quadlet.BootSource(u); src {
	case "":
		m.setStatus(u.Name+" is not enabled at boot", false)
		return m, nil, true
	case u.Path:
	default:
		m.setStatus(quadlet.ErrBootInDropin{Path: src}.Error(), true)
		return m, nil, true
	}
	m.pending = &pendingAction{verb: "disable", unit: u}
	m.setStatus("disable at boot: remove [Install] from "+filepath.Base(u.Path)+"? [y/N]", false)
	return m, nil, true

}

func (m Model) editKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "E" {
		return m, nil, false
	}
	if m.refuse(capTargetFiles | capTerminal) {
		return m, nil, true
	}
	mm, cmd := m.editSelected()
	return mm, cmd, true
}

func (m Model) updatesOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "u" {
		return m, nil, false
	}
	return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys, m.pm)), true

}

func (m Model) healthKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "h" {
		return m, nil, false
	}
	if m.refuse(capMutate) {
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	if u.Kind != quadlet.KindContainer {
		m.setStatus("healthchecks apply to container units", false)
		return m, nil, true
	}
	container := m.containerName(u)
	pm := m.pm
	return m, tea.Batch(m.setBusy("healthcheck "+container), func() tea.Msg {
		ok, err := pm.HealthcheckRun(context.Background(), container)
		return healthMsg{container: container, ok: ok, err: err}
	}), true

}
