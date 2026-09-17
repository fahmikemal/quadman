package ui

import (
	"context"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/tree"
	tea "charm.land/bubbletea/v2"
)

// Operational screens and unit management: storage, events, generate, problems, tree, instantiate, delete, copy, reload, linger.

func (m Model) storageKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "g" {
		return m, nil, false
	}
	return m, tea.Batch(m.setBusy("system df"), storageCmd()), true

}

func (m Model) eventsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "w" {
		return m, nil, false
	}
	mm, cmd := m.startEvents()
	return mm, cmd, true

}

func (m Model) generateOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "n" {
		return m, nil, false
	}
	if m.refuseRemoteWrite() {
		return m, nil, true
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	if !podletAvailable() {
		m.setStatus("podlet not found — install it to generate quadlets (github.com/containers/podlet)", false)
		return m, nil, true
	}
	m.generating = true
	m.genIn.Focus()
	return m, textinput.Blink, true

}

func (m Model) problemsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "v" {
		return m, nil, false
	}
	m.mode = modeValidate
	m.viewport.SetContent(m.validateView())
	m.viewport.GotoTop()
	m.resize()
	return m, nil, true

}

func (m Model) treeKeysOpen(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "t" {
		return m, nil, false
	}
	m.treeModel = tree.New(m.buildTreeModel(), 80, 20)
	m.mode = modeTree
	m.resize()
	return m, nil, true

}

func (m Model) instantiateOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "i" {
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
	if !strings.HasSuffix(u.Name, "@") {
		m.setStatus(u.Name+" is not a template (templates end with @)", false)
		return m, nil, true
	}
	m.instancing = true
	m.instanceIn.Focus()
	return m, textinput.Blink, true

}

func (m Model) deleteKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "D" {
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
	m.pending = &pendingAction{verb: "delete", unit: u}
	m.setStatus("delete "+filepath.Base(u.Path)+"? the unit stops and the file is removed [y/N]", false)
	return m, nil, true

}

func (m Model) copyNameKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "y" {
		return m, nil, false
	}
	mm, cmd := m.copySelection("name")
	return mm, cmd, true

}

func (m Model) copyImageKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "Y" {
		return m, nil, false
	}
	mm, cmd := m.copySelection("image")
	return mm, cmd, true

}

func (m Model) reloadKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "R" {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	m.podmanTried = false // re-enrich app/pod grouping after regeneration
	return m, tea.Batch(m.setBusy("daemon-reload"), actionCmd("daemon-reload", m.sys.DaemonReload)), true

}

func (m Model) lingerKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "L" {
		return m, nil, false
	}
	if m.system {
		m.setStatus("linger only applies to rootless user sessions", false)
		return m, nil, true
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	target := !m.linger
	lc := m.lc
	return m, tea.Batch(m.setBusy("toggle linger"), func() tea.Msg {
		if err := lc.Set(context.Background(), "", target); err != nil {
			return lingerSetMsg{on: !target, err: err}
		}
		return lingerSetMsg{on: target, known: true}
	}), true

}
