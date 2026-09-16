package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

func (m Model) modeUpdatesKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeUpdates {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "U":
		if m.refuseReadonly() {
			return m, nil, true
		}
		target := m.timerEnabled != "enabled"
		sys := m.sys
		return m, tea.Batch(m.setBusy("toggle "+autoUpdateTimer), func() tea.Msg {
			ctx := context.Background()
			var err error
			if target {
				_, err = sys.Enable(ctx, autoUpdateTimer, true)
			} else {
				_, err = sys.Disable(ctx, autoUpdateTimer, true)
			}
			return timerToggledMsg{err: err}
		}), true
	case "r":
		return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys)), true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true

}
