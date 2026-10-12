package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// This file holds the Elm update loop: Init plus the Update message
// switch. Key dispatch lives in keys.go (keyChain), screen transitions in
// screens.go; Update only routes.

// statusTTL is how long a success notification stays before it fades.
// Errors stay until something else replaces them.
const statusTTL = 5 * time.Second

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(enrichFull), m.pollTick())
}

// Update routes msg, then re-syncs the list scroll window so the cursor
// stays visible whichever handler moved it.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	wake := m.noteActivity(msg)
	model, cmd := m.update(msg)
	cmd = tea.Batch(cmd, wake)
	if mm, ok := model.(Model); ok {
		mm.syncListOffset()
		mm.syncScreenOffset()
		if mm.pending != nil && mm.pending != mm.armedFor {
			mm.armPending()
			mm.armedFor = mm.pending
		}
		return mm, cmd
	}
	return model, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		if msg.gen != m.tickGen {
			return m, nil // a slow chain replaced on wake
		}
		// Fade stale success notifications; errors stay put.
		if m.statusLine != "" && !m.statusErr && time.Since(m.statusAt) > statusTTL {
			m.clearStatus()
		}
		// Poll: refresh the list. Full podman enrichment runs once (or after
		// daemon-reload); health refreshes on a slower cadence.
		m.pollCount++
		var extra tea.Cmd
		if m.mode == modeStats && !m.busy && !m.statsInFlight {
			m.statsInFlight = true
			extra = m.inSession(m.statsLoad(false).background())
		}
		// One poll refresh at a time: on a slow session (SSH over a long
		// link) ticks must not stack refreshes faster than they finish.
		if m.refreshInFlight {
			return m, tea.Batch(m.pollTick(), extra)
		}
		level := enrichNone
		if !m.podmanTried {
			level = enrichFull
		} else if m.pollCount%healthPollEvery == 0 {
			level = enrichHealth
		}
		m.refreshInFlight = true
		req := m.refreshReq(level)
		req.fromPoll = true
		req.linger = req.linger || m.pollCount%lingerPollEvery == 0
		return m, tea.Batch(req.cmd(), m.pollTick(), extra)

	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case refreshMsg:
		return m.applyRefresh(msg)

	case actionMsg:
		m.busy = false
		m.recordActionMsg(msg)
		if msg.err != nil {
			// A job systemd keeps running is not a failure: say so in a
			// calm status and let the next refresh show where it went.
			pending := errors.Is(msg.err, systemd.ErrJobPending)
			m.setStatus(msg.desc+": "+msg.err.Error(), !pending)
			return m, m.refresh(enrichHealth)
		}
		status := strings.TrimSpace(msg.desc + " ok " + msg.out)
		if msg.hint != "" {
			status += " - " + msg.hint
		}
		m.setStatus(status, false)
		return m, m.refresh(enrichHealth)

	case bulkMsg:
		m.busy = false
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("%s: %d/%d ok: %s", msg.desc, msg.done, msg.total, msg.err.Error()), true)
			return m, m.refresh(enrichHealth)
		}
		m.clearMarks()
		m.setStatus(fmt.Sprintf("%s: %d/%d ok", msg.desc, msg.done, msg.total), false)
		return m, m.refresh(enrichHealth)

	case logsMsg:
		m.busy = false
		if msg.err != nil {
			m.viewport.SetContent(
				dimErrStyle.Render("journalctl failed for " + msg.unit + "\n\n" + msg.err.Error() +
					"\n\nHint: the user journal may be missing. Is systemd user session running?"))
		} else {
			if msg.out != "" {
				m.setLogLines(strings.Split(msg.out, "\n"))
				m.viewport.SetContent(m.logViewText())
			} else {
				m.viewport.SetContent(msg.out)
			}
		}
		m.viewport.GotoBottom()
		m.mode = modeDetail
		m.tab = tabJournal
		m.following = false
		m.resize()
		m.clearStatus()
		return m, nil

	case logLineMsg:
		if (msg.sess == m.eventSess && m.eventSess != nil) || (msg.sess == m.events.jsess && m.events.jsess != nil) {
			return m, m.applyEventLine(msg)
		}
		if msg.sess != m.sess { // a superseded session must not touch the view
			return m, nil
		}
		wasAtBottom := m.viewport.AtBottom()
		m.appendLogLine(msg.line)
		m.viewport.SetContent(m.logViewText())
		if m.following && wasAtBottom {
			m.viewport.GotoBottom()
		}
		return m, followLine(m.sess)

	case logsEndMsg:
		if msg.sess == m.sess && msg.err != nil {
			m.setStatus("journal stream ended: "+msg.err.Error(), true)
		}
		return m, nil

	case lingerSetMsg:
		m.busy = false
		if msg.err != nil {
			m.recordAction("toggle linger", msg.err)
			m.setStatus("linger: "+msg.err.Error(), true)
			return m, nil
		}
		m.linger = msg.on
		m.lingerKnown = true
		m.recordAction("linger "+onOff(msg.on), nil)
		m.setStatus("linger "+onOff(msg.on), false)
		return m, nil

	case updatesMsg:
		// The dashboard's AUTO-UPDATE tile uses these entries too, so they
		// are kept even when the updates screen is no longer on display.
		shown := m.screenLanded(modeUpdates)
		if msg.applied {
			m.busy = false
			m.recordAction("podman auto-update", msg.err)
		}
		if msg.err != nil {
			if shown || msg.applied {
				m.screenFailed(modeUpdates, "auto-update", msg.err)
			}
			if !msg.applied {
				return m, nil
			}
		}
		m.markLoaded(modeUpdates)
		m.updateEntries = msg.entries
		if m.updateEntries == nil {
			m.updateEntries = []podman.AutoUpdateEntry{}
		}
		m.timerEnabled = msg.timerEnabled
		m.timerActive = msg.timerActive
		m.updateTimer = msg.timer
		m.updatesApplied = msg.applied
		if msg.applied && msg.err == nil {
			m.setStatus(fmt.Sprintf("auto-update ran on %d container(s)", len(msg.entries)), false)
		}
		return m, nil

	case timerToggledMsg:
		m.busy = false
		m.recordAction("toggle "+autoUpdateTimer, msg.err)
		if msg.err != nil {
			m.setStatus("timer: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("timer toggled", false)
		return m, m.inSession(updatesLoad(m.sys, m.pm).background())

	// Background results tagged with their session (screens.go).
	case sessionMsg:
		if msg.epoch != m.epoch || msg.msg == nil {
			return m, nil // fetched for a previous session: never shown here
		}
		return m.update(msg.msg)

	// A screen load's result: only the newest load, still awaited, lands.
	case loadMsg:
		if msg.seq != m.scrSeq || m.scrLoading == modeList || msg.msg == nil {
			return m, nil // replaced, cancelled, or already answered
		}
		return m.update(msg.msg)

	case statsMsg:
		m.statsInFlight = false
		return m.applyScreenMsg(msg)

	// Async data screens (storage, stats, timers, secrets) share one
	// transition; see screenMsg in screens.go.
	case screenMsg:
		return m.applyScreenMsg(msg)

	case eventsHistoryMsg:
		return m.applyEventsHistory(msg)

	case timerLogMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("journal: "+msg.err.Error(), true)
			return m, nil
		}
		m.timers.logUnit, m.timers.logLines = msg.unit, msg.lines
		return m, nil

	case secretValueMsg:
		return m.applySecretValue(msg)

	case clipboardClearMsg:
		m.setStatus("clipboard cleared (secret "+msg.secret+")", false)
		return m, tea.SetClipboard("")

	case generateMsg:
		return m.applyGenerate(msg)

	case statusMsg:
		return m.applyTabMsg(msg.content, tabStatus)

	case inspectMsg:
		return m.applyTabMsg(msg.content, tabInspect)

	case customMsg:
		m.busy = false
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(msg.desc+": "+msg.err.Error(), true)
			return m, nil
		}
		status := strings.TrimSpace(msg.desc + " ok " + msg.out)
		m.setStatus(status, false)
		return m, m.refresh(enrichNone)

	case editorFinishedMsg:
		if msg.err != nil {
			m.setStatus("editor: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.changed {
			m.setStatus("file changed - press R to regenerate", false)
		} else {
			m.setStatus("no changes", false)
		}
		if m.mode == modeDetail && m.tab == tabSource {
			if u, ok := m.selected(); ok {
				if content, rerr := m.readUnitFile(u); rerr == nil {
					m.viewport.SetContent(m.fileContent(u, content))
				}
			}
		}
		return m, m.refresh(enrichNone)

	case execResolveMsg:
		if msg.err != nil {
			m.setStatus("exec: "+msg.err.Error(), true)
			return m, nil
		}
		if !msg.found {
			m.execing = true
			m.execIn.Focus()
			m.setStatus(fmt.Sprintf("container %q not found — is the unit running? (edit name, enter)", msg.name), true)
			return m, nil
		}
		m.setStatus("exec in "+msg.name+" (/bin/sh) — exit the shell to return", false)
		return m, m.execShell(msg.name)

	case execFinishedMsg:
		if msg.err != nil {
			m.setStatus("exec: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("exec session ended", false)
		return m, m.refresh(enrichNone)

	case healthMsg:
		m.busy = false
		if msg.err != nil {
			m.recordAction("healthcheck "+msg.container, msg.err)
			m.setStatus("healthcheck: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.ok {
			m.recordAction("healthcheck "+msg.container, nil)
			m.setStatus("healthcheck "+msg.container+": healthy", false)
		} else {
			m.recordAction("healthcheck "+msg.container, errUnhealthy)
			m.setStatus("healthcheck "+msg.container+": UNHEALTHY", true)
		}
		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouse(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Everything else (mouse, focus…) goes to the active widget.
	var cmd tea.Cmd
	if m.mode == modeList {
		m.table, cmd = m.table.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}
