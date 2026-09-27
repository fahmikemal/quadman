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
	screenContent() string
	screenErr() error
}

// applyScreenMsg runs the shared one-shot-screen transition.
func (m Model) applyScreenMsg(msg screenMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if err := msg.screenErr(); err != nil {
		m.setStatus(msg.screenErrPrefix()+": "+err.Error(), true)
		return m, nil
	}
	m.mode = msg.screenMode()
	m.viewport.SetContent(msg.screenContent())
	m.viewport.GotoTop()
	m.resize()
	return m, nil
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

func (s storageMsg) screenMode() mode        { return modeStorage }
func (s storageMsg) screenErrPrefix() string { return "system df" }
func (s storageMsg) screenContent() string   { return s.content }
func (s storageMsg) screenErr() error        { return s.err }

func (s statsMsg) screenMode() mode        { return modeStats }
func (s statsMsg) screenErrPrefix() string { return "stats" }
func (s statsMsg) screenContent() string   { return s.content }
func (s statsMsg) screenErr() error        { return s.err }

func (s timersMsg) screenMode() mode        { return modeTimers }
func (s timersMsg) screenErrPrefix() string { return "timers" }
func (s timersMsg) screenContent() string   { return s.content }
func (s timersMsg) screenErr() error        { return s.err }

func (s secretsMsg) screenMode() mode        { return modeSecrets }
func (s secretsMsg) screenErrPrefix() string { return "secrets" }
func (s secretsMsg) screenContent() string   { return s.content }
func (s secretsMsg) screenErr() error        { return s.err }
