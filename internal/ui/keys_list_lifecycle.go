package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// Lifecycle actions on the selected unit: start, stop, restart.

func (m Model) startRestartKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "s" && msg.String() != "r" {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	verb := map[string]string{"s": "start", "r": "restart"}[msg.String()]
	if marked := m.markedUnits(); len(marked) > 0 {
		m.pending = &pendingAction{verb: "bulk-" + verb, unit: marked[0]}
		m.setStatus(verb+" "+bulkNoun(marked)+"? [y/N]", false)
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	if isExternalUnit(u.Path) {
		m.setStatus(u.Name+" lives in an extra dir - "+externalHint(), true)
		return m, nil, true
	}
	st := m.status[u.UnitName]
	if verb == "start" {
		if st.LoadState == "not-found" {
			m.setStatus(u.UnitName+" not found by systemd — press R to daemon-reload", true)
			return m, nil, true
		}
		if st.ActiveState == "active" {
			m.setStatus(u.UnitName+" is already active ("+st.SubState+")", false)
			return m, nil, true
		}
	}
	sys := m.sys
	return m, tea.Batch(m.setBusy(verb+" "+u.UnitName),
		actionCmd(verb+" "+u.UnitName, func(ctx context.Context) (string, error) {
			return sys.UnitAction(ctx, verb, u.UnitName)
		})), true
}

func (m Model) stopKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "x" {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	if marked := m.markedUnits(); len(marked) > 0 {
		m.pending = &pendingAction{verb: "bulk-stop", unit: marked[0]}
		m.setStatus("stop "+bulkNoun(marked)+"? containers will be removed (quadlet runs --rm) [y/N]", false)
		return m, nil, true
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	st := m.status[u.UnitName]
	if st.ActiveState == "inactive" || st.ActiveState == "failed" {
		m.setStatus(u.UnitName+" is already stopped ("+st.ActiveState+")", false)
		return m, nil, true
	}
	m.pending = &pendingAction{verb: "stop", unit: u}
	m.setStatus("stop "+u.UnitName+"? container will be removed (quadlet runs --rm) [y/N]", false)
	return m, nil, true
}
