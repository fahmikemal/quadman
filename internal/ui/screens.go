package ui

import (
	tea "charm.land/bubbletea/v2"
)

// screenMsg is implemented by one-shot viewport screens that share the
// exact same Update shape: on error keep the current mode and report the
// failure in the status line, otherwise switch to the screen's mode and
// render the content at the top. Adding a screen means adding its three
// methods — no new case in Update.
type screenMsg interface {
	tea.Msg
	// screenMode is the mode to enter when the fetch succeeded.
	screenMode() mode
	// screenErrPrefix prefixes the status line on failure
	// (e.g. "stats" renders "stats: <err>").
	screenErrPrefix() string
	screenErr() error
	// apply stores the fetched data on the model.
	apply(m *Model)
}

// applyScreenMsg runs the shared one-shot-screen transition: on error keep
// the current mode and report it; otherwise store the data and open the
// screen (a refresh of the open screen keeps cursor, tab and filter).
func (m Model) applyScreenMsg(msg screenMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if err := msg.screenErr(); err != nil {
		m.setStatus(msg.screenErrPrefix()+": "+err.Error(), true)
		return m, nil
	}
	pin, _ := m.selectedRow()
	msg.apply(&m)
	if m.mode != msg.screenMode() {
		m.enterScreen(msg.screenMode())
		return m, nil
	}
	m.pinScreenRow(pin.id)
	return m, nil
}

// pinScreenRow keeps the cursor on the row with id after data changed.
func (m *Model) pinScreenRow(id string) {
	if id == "" {
		return
	}
	rows := m.visibleRows(m.screenSpec().rows)
	for i, idx := range selectable(rows) {
		if rows[idx].id == id {
			m.scrCur = i
			return
		}
	}
}

// applyTabMsg runs the shared detail-tab refresh: only viewports already
// showing the tab are replaced, anything else just clears the spinner.
func (m Model) applyTabMsg(content string, tab int) (tea.Model, tea.Cmd) {
	m.busy = false
	if m.tab == tab {
		m.viewport.SetContent(content)
		m.viewport.GotoTop()
	}
	return m, nil
}
