package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// Updates screen (u): podman auto-update as tiles (timer, next and last
// run, pending), one row per labeled container with its result, and an
// apply-now action behind a confirmation.

// autoUpdateTimer is the user timer that drives `podman auto-update`.
const autoUpdateTimer = "podman-auto-update.timer"

type updatesMsg struct {
	entries      []podman.AutoUpdateEntry
	timerEnabled string // "enabled" / "disabled" / "static" / …
	timerActive  string // "active" / "inactive" / …
	timer        systemd.TimerInfo
	applied      bool // entries come from a real run, not a dry run
	err          error
}

type timerToggledMsg struct{ err error }

// updatesCmd previews `podman auto-update --dry-run` and reads the timer
// state. Works without podman: the screen then shows the timer only.
// Podman is probed through the session's client, so SSH and compartment
// sessions read the remote podman instead of the operator's PATH.
func updatesCmd(sys *systemd.Systemd, pm podman.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg := updatesMsg{}
		if pm.Reachable(ctx) {
			entries, err := pm.AutoUpdateDryRun(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.entries = entries
		}
		msg.timerEnabled, _ = sys.IsEnabled(ctx, autoUpdateTimer)
		msg.timerActive, _ = sys.IsActive(ctx, autoUpdateTimer)
		if ti, err := sys.TimerDetails(ctx, []string{autoUpdateTimer}); err == nil && len(ti) == 1 {
			msg.timer = ti[0]
		}
		return msg
	}
}

// applyUpdatesCmd runs auto-update for real, then re-reads the timer.
func applyUpdatesCmd(sys *systemd.Systemd, pm podman.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		entries, err := pm.AutoUpdateApply(ctx)
		msg := updatesMsg{entries: entries, applied: true, err: err}
		msg.timerEnabled, _ = sys.IsEnabled(ctx, autoUpdateTimer)
		msg.timerActive, _ = sys.IsActive(ctx, autoUpdateTimer)
		if ti, err := sys.TimerDetails(ctx, []string{autoUpdateTimer}); err == nil && len(ti) == 1 {
			msg.timer = ti[0]
		}
		return msg
	}
}

// updateResult maps podman's Updated field to glyph + word.
func updateResult(updated string) (string, glyphKind, int) {
	switch updated {
	case "failed":
		return glyph(gFailed) + " failed", gFailed, 0
	case "pending":
		return "↑ pending", gStarting, 1
	case "true":
		return glyph(gDone) + " updated", gActive, 2
	case "rolled back":
		return "↺ rolled back", gWarn, 3
	case "false":
		return "= current", gInactive, 4
	}
	return updated, gInactive, 5
}

func (m Model) updatesSpec() screenSpec {
	pending, failed := 0, 0
	for _, e := range m.updateEntries {
		switch e.Updated {
		case "pending":
			pending++
		case "failed":
			failed++
		}
	}
	timerOn := m.timerEnabled == "enabled"
	timerTile := tile{label: "TIMER", value: glyph(gInactive) + " off", note: "U turns it on", valueStyle: warnStyle, tint: activePalette.warn}
	if timerOn {
		timerTile = tile{label: "TIMER", value: glyph(gActive) + " on", note: orUnknown(m.timerActive) + " · U toggles", valueStyle: okStyle}
	}
	next := tile{label: "NEXT RUN", value: "—", note: "not scheduled", valueStyle: mutedStyle}
	if t := m.updateTimer.Next; !t.IsZero() {
		next = tile{label: "NEXT RUN", value: t.Format("15:04"), note: "in " + humanDuration(time.Until(t)), valueStyle: accentStyle}
	}
	last := tile{label: "LAST RUN", value: "never", note: "", valueStyle: mutedStyle}
	if t := m.updateTimer.Last; !t.IsZero() {
		last = tile{label: "LAST RUN", value: glyph(gDone) + " " + shortWhen(t), note: m.updateTimer.LastResult, valueStyle: okStyle}
		if m.updateTimer.LastResult != "" && m.updateTimer.LastResult != "success" {
			last.value = glyph(gFailed) + " " + shortWhen(t)
			last.valueStyle, last.tint = errStyle, activePalette.err
		}
	}
	pend := tile{label: "PENDING", value: fmt.Sprintf("%d", pending), note: "a applies now", valueStyle: accentStyle}
	if pending == 0 {
		pend.valueStyle, pend.note = mutedStyle, "everything current"
	}
	if failed > 0 {
		pend.note = fmt.Sprintf("%d failed", failed)
		pend.tint = activePalette.err
	}
	s := screenSpec{
		tiles: []tile{timerTile, next, last, pend},
		cols: []scrCol{
			{title: "RESULT", w: 14},
			{title: "UNIT", min: 16, max: 34},
			{title: "IMAGE", min: 16},
			{title: "POLICY", w: 9},
			{title: "CONTAINER", min: 12, max: 28, hide: breakpointMid},
		},
		empty:  "no unit has AutoUpdate=registry or =local in its [Container] section",
		detail: m.updateDetail,
		info:   fmt.Sprintf("podman auto-update --dry-run · %d pending", pending),
	}
	if m.updatesApplied {
		s.info = "podman auto-update ran · results below"
	}
	type ranked struct {
		e    podman.AutoUpdateEntry
		rank int
	}
	var rs []ranked
	for _, e := range m.updateEntries {
		_, _, r := updateResult(e.Updated)
		rs = append(rs, ranked{e, r})
	}
	for rank := 0; rank <= 5; rank++ {
		for _, r := range rs {
			if r.rank != rank {
				continue
			}
			e := r.e
			text, g, _ := updateResult(e.Updated)
			unit := ""
			if _, ok := m.unitByName(e.Unit); ok {
				unit = e.Unit
			}
			s.rows = append(s.rows, scrRow{
				id:   "upd:" + e.Unit + ":" + e.Container,
				unit: unit,
				cells: []scrCell{
					cell(text, glyphStyle(g)),
					cell(e.Unit, fgStyle),
					cell(e.Image, mutedStyle),
					cell(e.Policy, mutedStyle),
					cell(e.Container, mutedStyle),
				},
			})
		}
	}
	return s
}

func (m Model) updateDetail(r scrRow, _ int) []string {
	for _, e := range m.updateEntries {
		if "upd:"+e.Unit+":"+e.Container != r.id {
			continue
		}
		text, g, _ := updateResult(e.Updated)
		lines := []string{
			brightStyle.Render(e.Unit) + "  " + glyphStyle(g).Render(text),
			detailField("Image", e.Image),
			detailField("Policy", e.Policy+mutedStyle.Render("  · pull → restart → roll back on failure")),
		}
		if strings.Contains(e.Image, "@sha256:") {
			lines = append(lines, warnStyle.Render(glyph(gWarn)+" pinned by digest — auto-update has no effect"))
		}
		if i, ok := m.unitByName(e.Unit); ok && isExternalUnit(m.units[i].Path) {
			lines = append(lines, errStyle.Render(glyph(gFailed)+" not loaded by the generator (extra dir)"))
		}
		lines = append(lines, keyHint("a", "apply now")+"   "+keyHint("r", "re-check")+"   "+keyHint("U", "timer")+"   "+keyHint("↵", "open unit"))
		return lines
	}
	return nil
}

// updatesKeysIn handles the updates screen's own keys.
func (m Model) updatesKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "U":
		if m.refuse(capMutate) {
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
		return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys, m.pm)), true
	case "a":
		if m.refuse(capMutate) {
			return m, nil, true
		}
		var pending []string
		for _, e := range m.updateEntries {
			if e.Updated == "pending" {
				pending = append(pending, e.Unit)
			}
		}
		if len(pending) == 0 {
			m.setStatus("nothing pending — r re-checks the registries", false)
			return m, nil, true
		}
		sys, pm := m.sys, m.pm
		m.pending = &pendingAction{
			verb:  "apply updates",
			title: fmt.Sprintf("Apply %d update%s now?", len(pending), plural(len(pending))),
			lines: []string{"pull the new images", "restart " + strings.Join(pending, ", "), "roll a unit back to its old image if it fails to start"},
			keeps: []string{"volumes and their data", "units without an AutoUpdate policy"},
			run: func(m Model) (tea.Model, tea.Cmd) {
				return m, tea.Batch(m.setBusy("podman auto-update"), applyUpdatesCmd(sys, pm))
			},
		}
		return m, nil, true
	}
	return m, nil, false
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
