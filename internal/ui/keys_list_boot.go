package ui

import (
	"context"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Boot enablement, edit, updates, and health actions.

func (m Model) enableKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "e" {
		return m, nil, false
	}
	if m.refuseRemoteWrite() {
		return m, nil, true
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	// Boot start is declarative in quadlet: [Install] WantedBy= in the
	// file, and the generator wires it up on daemon-reload. Newer
	// systemd refuses `systemctl enable` for generated units.
	if f, err := m.parseUnitFile(u); err == nil && f.BootTarget() != "" {
		sys := m.sys
		return m, tea.Batch(m.setBusy("start "+u.UnitName),
			actionCmd("start "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "start", u.UnitName)
			})), true
	}
	m.pending = &pendingAction{verb: "enable", unit: u}
	m.setStatus("enable at boot: append [Install] WantedBy=default.target to "+filepath.Base(u.Path)+"? [y/N]", false)
	return m, nil, true

}

func (m Model) disableKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "d" {
		return m, nil, false
	}
	if m.refuseRemoteWrite() {
		return m, nil, true
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	f, err := m.parseUnitFile(u)
	if err != nil || f.BootTarget() == "" {
		m.setStatus(u.Name+" is not enabled at boot", false)
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
	if m.noEditor {
		m.setStatus("editor not available in SSH server session: edit Quadlet files directly on the host", true)
		return m, nil, true
	}
	if m.refuseRemoteWrite() {
		return m, nil, true
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	mm, cmd := m.editSelected()
	return mm, cmd, true
}

func (m Model) updatesOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "u" {
		return m, nil, false
	}
	return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys)), true

}

func (m Model) healthKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "h" {
		return m, nil, false
	}
	if m.refuseReadonly() {
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
	container := "systemd-" + u.Name
	return m, tea.Batch(m.setBusy("healthcheck "+container), func() tea.Msg {
		ok, err := podman.HealthcheckRun(context.Background(), container)
		return healthMsg{container: container, ok: ok, err: err}
	}), true

}
