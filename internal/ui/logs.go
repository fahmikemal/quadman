package ui

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

// logBufferCap bounds how many journal lines the follow view keeps. It
// defaults to 1000 and is overridden from the YAML config (log_buffer) in
// New when set.
var logBufferCap = 1000

// logTailLines is the journal snapshot size for a new follow stream. It
// defaults to 200 and is overridden from the YAML config (log_tail).
var logTailLines = 200

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

// visibleLogLines returns lines filtered by m.logFilter if active.
func (m Model) visibleLogLines() []string {
	if m.logFilter == "" {
		return m.logLines
	}
	var out []string
	q := strings.ToLower(m.logFilter)
	for _, l := range m.logLines {
		if strings.Contains(strings.ToLower(l), q) {
			out = append(out, l)
		}
	}
	return out
}

// refreshLogContent updates the viewport content with visible filtered lines and re-applies search highlights.
func (m *Model) refreshLogContent() {
	lines := m.visibleLogLines()
	m.viewport.SetContent(strings.Join(lines, "\n"))
	m.applySearch()
}

// cycleLogPriority cycles journal priority: all -> err -> warning -> info -> all
func (m Model) cycleLogPriority() (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	switch m.logPriority {
	case "":
		m.logPriority = "err"
	case "err":
		m.logPriority = "warning"
	case "warning":
		m.logPriority = "info"
	case "info":
		m.logPriority = ""
	default:
		m.logPriority = ""
	}
	label := m.logPriority
	if label == "" {
		label = "all"
	}
	mm, cmd := m.startLogsWithPriority(u, m.logPriority)
	if mmm, ok := mm.(Model); ok {
		mmm.setStatus(fmt.Sprintf("log priority: %s", label), false)
		return mmm, cmd
	}
	return mm, cmd
}

// startLogs opens a live journal stream for u with current priority.
func (m Model) startLogs(u quadlet.Unit) (tea.Model, tea.Cmd) {
	return m.startLogsWithPriority(u, m.logPriority)
}

// startLogsWithPriority opens a live journal stream for u filtered by priority level.
func (m Model) startLogsWithPriority(u quadlet.Unit, priority string) (tea.Model, tea.Cmd) {
	m.stopLogs()
	m.logPriority = priority
	stop, stream, err := m.sys.FollowJournalWithPriority(context.Background(), u.UnitName, logTailLines, priority)
	if err != nil {
		return m, tea.Batch(m.setBusy("loading logs "+u.UnitName), logsCmdWithPriority(m.sys, u.UnitName, priority))
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
	return logsCmdWithPriority(sys, unit, "")
}

func logsCmdWithPriority(sys *systemd.Systemd, unit string, priority string) tea.Cmd {
	return func() tea.Msg {
		out, err := sys.JournalWithPriority(context.Background(), unit, 300, priority)
		return logsMsg{unit: unit, out: out, err: err}
	}
}

// exportLogs exports the current unit's visible log lines.
func (m Model) exportLogs(format string) (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		m.setStatus("no unit selected", true)
		return m, nil
	}
	lines := m.visibleLogLines()
	if len(lines) == 0 {
		m.setStatus("no log lines to export", true)
		return m, nil
	}

	if format == "clipboard" {
		text := strings.Join(lines, "\n")
		m.setStatus(fmt.Sprintf("copied %d log lines to clipboard", len(lines)), false)
		m.recordAction(fmt.Sprintf("copy %d log lines", len(lines)), nil)
		return m, tea.SetClipboard(text)
	}

	if m.readonly {
		m.setStatus("export logs refused: readonly", true)
		return m, nil
	}
	if m.ssh.IsRemote() {
		m.setStatus("export to local file unavailable over SSH (use 'c' to copy)", true)
		return m, nil
	}

	ext := "log"
	if format == "json" {
		ext = "jsonl"
	}
	filename := defaultExportFilename(u.UnitName, ext)
	var err error
	if format == "json" {
		err = exportLogsJSONL(filename, u.UnitName, lines)
	} else {
		err = exportLogsText(filename, u.UnitName, lines)
	}

	if err != nil {
		m.recordAction("export logs "+u.UnitName, err)
		m.setStatus("export failed: "+err.Error(), true)
		return m, nil
	}

	desc := fmt.Sprintf("exported %d lines to %s", len(lines), filename)
	m.recordAction(desc, nil)
	m.setStatus(desc, false)
	return m, nil
}
