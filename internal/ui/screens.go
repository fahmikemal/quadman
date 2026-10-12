package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// screenMsg is implemented by the async screens' data results, which share
// one Update shape (applyScreenMsg). Adding a screen means adding these
// methods — no new case in Update.
type screenMsg interface {
	tea.Msg
	// screenMode is the screen the data belongs to.
	screenMode() mode
	// screenErrPrefix prefixes the status line on failure
	// (e.g. "stats" renders "stats: <err>").
	screenErrPrefix() string
	screenErr() error
	// apply stores the fetched data on the model.
	apply(m *Model)
}

// Async screens (updates, storage, timers, secrets, stats) fetch their
// data in the background. They open at once — showing their last data, or
// a loading state on the first visit — so the view behind them never
// flashes in between. A result only ever updates its own screen: one that
// lands after the user left is kept for the next visit, never used to
// pull the user back.

// asyncScreens is the set of screens whose data arrives after they open.
const asyncScreens = uint32(1)<<modeUpdates | uint32(1)<<modeStorage |
	uint32(1)<<modeTimers | uint32(1)<<modeSecrets | uint32(1)<<modeStats

func modeBit(md mode) uint32 { return uint32(1) << uint(md) }

func isAsyncScreen(md mode) bool { return asyncScreens&modeBit(md) != 0 }

// screenLoaded reports whether md has data from this session.
func (m Model) screenLoaded(md mode) bool { return m.scrLoaded&modeBit(md) != 0 }

// retryKey is the key that reloads md (secrets uses r for rotate).
func retryKey(md mode) string {
	if md == modeSecrets {
		return "ctrl+r"
	}
	return "r"
}

// sessionMsg is a background result tagged with the session it was
// fetched for; Update drops it after a session switch.
type sessionMsg struct {
	epoch uint64
	msg   tea.Msg
}

// inSession tags cmd's result with the current session.
func (m Model) inSession(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	epoch := m.epoch
	return func() tea.Msg { return sessionMsg{epoch: epoch, msg: cmd()} }
}

// screenLoad fetches a screen's data. Its context ends when the user
// leaves the screen or a newer load replaces it, which kills the commands
// still running — abandoned loads never pile up behind the next one.
type screenLoad func(context.Context) tea.Msg

// background runs l to completion, for reloads the user waits on in place
// (after an action, the stats sampler).
func (l screenLoad) background() tea.Cmd {
	return func() tea.Msg { return l(context.Background()) }
}

// loadMsg is the result of the screen load numbered seq.
type loadMsg struct {
	seq uint64
	msg tea.Msg
}

// screenTTL is how long a screen's data is shown as is when reopened.
func screenTTL(md mode) time.Duration {
	switch md {
	case modeUpdates:
		return 5 * time.Minute // a dry run asks every image's registry
	case modeStats:
		return 0 // live: every open samples, the poll tick keeps sampling
	}
	return 30 * time.Second
}

// screenFresh reports data for md recent enough to show without reloading.
func (m Model) screenFresh(md mode) bool {
	return m.screenLoaded(md) && time.Since(m.scrLoadedAt[md]) < screenTTL(md)
}

// openScreen switches to the async screen md right away. Fresh data is
// shown as is (with its age); otherwise a load starts, unless one already
// runs for md.
func (m Model) openScreen(md mode, what string, load screenLoad) (tea.Model, tea.Cmd, bool) {
	m.enterScreen(md)
	switch {
	case m.scrLoading == md:
		return m, nil, true
	case m.screenFresh(md):
		m.setStatus("data from "+ago(m.scrLoadedAt[md])+"  ·  "+retryKey(md)+" refreshes", false)
		return m, nil, true
	}
	cmd := m.loadScreen(md, what, load)
	return m, cmd, true
}

// loadScreen (re)fetches md's data with the spinner running, replacing
// (and cancelling) any load still in flight.
func (m *Model) loadScreen(md mode, what string, load screenLoad) tea.Cmd {
	m.cancelLoad()
	ctx, cancel := context.WithCancel(context.Background())
	m.scrSeq++
	seq := m.scrSeq
	m.scrCancel = cancel
	m.scrLoading = md
	if m.scrErrFor == md {
		m.scrErr, m.scrErrFor = "", modeList
	}
	return tea.Batch(m.setBusy(what), m.inSession(func() tea.Msg {
		defer cancel()
		return loadMsg{seq: seq, msg: load(ctx)}
	}))
}

// cancelLoad abandons the screen load in flight, if any: its commands are
// killed and its result, when it lands, is dropped.
func (m *Model) cancelLoad() {
	if m.scrCancel != nil {
		m.scrCancel()
		m.scrCancel = nil
	}
	if m.scrLoading != modeList {
		m.scrLoading = modeList
		m.busy = false
	}
}

// actionThenReload runs a screen action, then reloads the screen: in
// sequence, so the reloaded rows show what the action did.
func (m Model) actionThenReload(action tea.Cmd, reload screenLoad) tea.Cmd {
	return tea.Sequence(action, m.inSession(reload.background()))
}

// screenLanded settles the bookkeeping for a result of screen md: it ends
// the load it answers (and its spinner) and reports whether md is the
// screen on display.
func (m *Model) screenLanded(md mode) bool {
	awaited, shown := m.scrLoading == md, m.mode == md
	if awaited {
		m.scrLoading = modeList // modeList: no screen load in flight
	}
	if awaited || shown {
		m.busy = false
	}
	return shown
}

// screenFailed reports a load error for md — only while md is on display,
// and as its body text when there is nothing else to show.
func (m *Model) screenFailed(md mode, prefix string, err error) {
	m.setStatus(prefix+": "+err.Error(), true)
	if !m.screenLoaded(md) {
		m.scrErr, m.scrErrFor = err.Error(), md
	}
}

// applyScreenMsg stores an async screen's data. Errors are reported only
// while that screen is on display; a refresh of the open screen keeps its
// cursor, tab and filter.
func (m Model) applyScreenMsg(msg screenMsg) (tea.Model, tea.Cmd) {
	md := msg.screenMode()
	shown := m.screenLanded(md)
	if err := msg.screenErr(); err != nil {
		if shown {
			m.screenFailed(md, msg.screenErrPrefix(), err)
		}
		return m, nil
	}
	var pin scrRow
	if shown {
		pin, _ = m.selectedRow()
	}
	msg.apply(&m)
	m.markLoaded(md)
	if shown {
		m.pinScreenRow(pin.id)
	}
	return m, nil
}

// markLoaded records that md holds data from this session, as of now.
func (m *Model) markLoaded(md mode) {
	m.scrLoaded |= modeBit(md)
	m.scrLoadedAt[md] = time.Now()
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
