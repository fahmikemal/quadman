package ui

import (
	"bufio"
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

// logBufferCap bounds how many journal lines the follow view keeps.
const logBufferCap = 1000

// logSession is one live `journalctl -f` stream.
type logSession struct {
	stop func()
	scan *bufio.Scanner
	unit string
}

type logLineMsg struct {
	sess *logSession
	line string
}

type logsEndMsg struct {
	sess *logSession
	err  error
}

// followLine reads exactly one line from the session; Update re-issues it,
// so at most one read is ever in flight and lines cannot interleave.
func followLine(sess *logSession) tea.Cmd {
	return func() tea.Msg {
		if sess.scan.Scan() {
			return logLineMsg{sess: sess, line: sess.scan.Text()}
		}
		return logsEndMsg{sess: sess, err: sess.scan.Err()}
	}
}

// startLogs opens a live journal stream for u, replacing any active session.
// If the stream cannot start it falls back to the 300-line snapshot.
func (m Model) startLogs(u quadlet.Unit) (tea.Model, tea.Cmd) {
	m.stopLogs()
	stop, stream, err := m.sys.FollowJournal(context.Background(), u.UnitName, 200)
	if err != nil {
		return m, tea.Batch(m.setBusy("loading logs "+u.UnitName), logsCmd(m.sys, u.UnitName))
	}
	sess := &logSession{stop: stop, scan: bufio.NewScanner(stream), unit: u.UnitName}
	sess.scan.Buffer(make([]byte, 256*1024), 256*1024) // long journal lines must not kill the stream
	m.sess = sess
	m.logLines = nil
	m.clearSearch()
	m.following = true
	m.mode = modeDetail
	m.tab = tabJournal
	m.viewport.SetContent("")
	m.viewport.GotoBottom()
	m.resize()
	m.clearStatus()
	return m, followLine(sess)
}

// stopLogs kills the active journal stream, if any.
func (m *Model) stopLogs() {
	if m.sess != nil {
		m.sess.stop()
		m.sess = nil
	}
}

func logsCmd(sys *systemd.Systemd, unit string) tea.Cmd {
	return func() tea.Msg {
		out, err := sys.Journal(context.Background(), unit, 300)
		return logsMsg{unit: unit, out: out, err: err}
	}
}
