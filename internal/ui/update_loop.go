package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		// Fade stale success notifications; errors stay put.
		if m.statusLine != "" && !m.statusErr && time.Since(m.statusAt) > statusTTL {
			m.clearStatus()
		}
		// Poll: refresh the list. Full podman enrichment runs once (or after
		// daemon-reload); health refreshes on a slower cadence.
		m.pollCount++
		level := enrichNone
		if !m.podmanTried {
			level = enrichFull
		} else if m.pollCount%healthPollEvery == 0 {
			level = enrichHealth
		}
		return m, tea.Batch(m.refresh(level), m.pollTick())

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
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(msg.desc+": "+msg.err.Error(), true)
			return m, nil
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
				m.logLines = strings.Split(msg.out, "\n")
				m.viewport.SetContent(strings.Join(m.visibleLogLines(), "\n"))
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
		if msg.sess == m.eventSess && m.eventSess != nil {
			m.eventLines = append(m.eventLines, msg.line)
			if len(m.eventLines) > logBufferCap {
				m.eventLines = m.eventLines[len(m.eventLines)-logBufferCap:]
			}
			if m.mode == modeEvents {
				wasAtBottom := m.viewport.AtBottom()
				m.viewport.SetContent(strings.Join(m.eventLines, "\n"))
				if m.following && wasAtBottom {
					m.viewport.GotoBottom()
				}
			}
			return m, followLine(m.eventSess)
		}
		if msg.sess != m.sess { // a superseded session must not touch the view
			return m, nil
		}
		wasAtBottom := m.viewport.AtBottom()
		m.logLines = append(m.logLines, msg.line)
		if len(m.logLines) > logBufferCap {
			m.logLines = m.logLines[len(m.logLines)-logBufferCap:]
		}
		m.viewport.SetContent(strings.Join(m.visibleLogLines(), "\n"))
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
		m.busy = false
		if msg.err != nil {
			m.setStatus("auto-update: "+msg.err.Error(), true)
			return m, nil
		}
		m.updateEntries = msg.entries
		m.timerEnabled = msg.timerEnabled
		m.timerActive = msg.timerActive
		m.mode = modeUpdates
		m.viewport.SetContent(m.updatesView())
		m.viewport.GotoTop()
		m.resize()
		m.clearStatus()
		return m, nil

	case timerToggledMsg:
		m.busy = false
		m.recordAction("toggle "+autoUpdateTimer, msg.err)
		if msg.err != nil {
			m.setStatus("timer: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("timer toggled", false)
		return m, updatesCmd(m.sys, m.ssh)

	// One-shot viewport screens (storage, stats, timers, secrets) share
	// one transition; see screenMsg in screens.go.
	case screenMsg:
		return m.applyScreenMsg(msg)

	case generateMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("generate: "+msg.err.Error(), true)
			return m, nil
		}
		m.genContent = msg.content
		m.mode = modeGenerate
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		m.resize()
		return m, nil

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
