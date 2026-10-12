package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Actions screen (A): the session audit — what ran, how it was triggered,
// how long it took, the full error, the equivalent command, and a way to
// run it again.

// errUnhealthy marks a finished healthcheck with a failing result, so the
// recent-actions log shows it as failed while the status line keeps its
// dedicated UNHEALTHY wording.
var errUnhealthy = errors.New("unhealthy")

// actionRecord is one entry of the session log.
type actionRecord struct {
	at        time.Time
	desc      string
	ok        bool
	err       string
	took      time.Duration // 0 when not measured
	how       string        // "key r", "palette", "key y · confirmed", …
	cancelled bool
	hint      string
	// redo re-runs the same work (nil for actions that cannot repeat).
	redo func(context.Context) (string, error)
	// epoch is the session redo is bound to; guard the confirmation the
	// action passed, which a repeat must pass again.
	epoch uint64
	guard *confirmGuard
}

// maxActions bounds the in-memory session log.
const maxActions = 200

// recordAction appends a finished action to the session log.
func (m *Model) recordAction(desc string, err error) {
	m.appendAction(actionRecord{at: time.Now(), desc: desc, ok: err == nil, how: m.busyHow, err: errText(err)})
}

// recordActionMsg logs an actionMsg with its duration and redo closure.
func (m *Model) recordActionMsg(msg actionMsg) {
	m.appendAction(actionRecord{
		at: time.Now(), desc: msg.desc, ok: msg.err == nil, err: errText(msg.err),
		took: msg.took, how: m.busyHow, hint: msg.hint, redo: msg.run,
		epoch: msg.epoch, guard: msg.guard,
	})
}

// recordCancelled logs a confirmation the user declined.
func (m *Model) recordCancelled(desc, how string) {
	m.appendAction(actionRecord{at: time.Now(), desc: desc, cancelled: true, how: how})
}

func (m *Model) appendAction(rec actionRecord) {
	m.actions = append(m.actions, rec)
	if len(m.actions) > maxActions {
		m.actions = m.actions[len(m.actions)-maxActions:]
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// actionVerbTarget splits "restart demo-web.service" into its verb and
// target; multi-word verbs ("enable at boot", "bulk start") stay whole.
func actionVerbTarget(desc string) (string, string) {
	for _, verb := range []string{"enable at boot", "disable at boot", "bulk start", "bulk stop", "bulk restart",
		"create secret", "rotate secret", "remove secret", "export logs", "system prune", "toggle linger"} {
		if rest, ok := strings.CutPrefix(desc, verb); ok {
			return verb, strings.TrimSpace(rest)
		}
	}
	verb, target, _ := strings.Cut(desc, " ")
	return verb, target
}

// actionCommand reconstructs the systemctl command for lifecycle actions,
// so a failure can be retried by hand; "" for anything else.
func (m Model) actionCommand(desc string) string {
	verb, target := actionVerbTarget(desc)
	scope := ""
	if !m.system {
		scope = " --user"
	}
	prefix := ""
	switch {
	case m.ssh.IsRemote():
		prefix = "ssh " + m.ssh.Target + " "
	case m.ssh.As != "":
		prefix = "sudo -u " + m.ssh.As + " "
	}
	switch verb {
	case "start", "stop", "restart", "enable", "disable":
		if strings.HasSuffix(target, ".service") || strings.HasSuffix(target, ".timer") {
			return prefix + "systemctl" + scope + " " + verb + " " + target
		}
	case "daemon-reload":
		return prefix + "systemctl" + scope + " daemon-reload"
	case "healthcheck":
		return prefix + "podman healthcheck run " + target
	case "pull":
		return prefix + "podman pull " + target
	}
	return ""
}

// actionUnit maps an action target back to a quadlet unit, if any.
func (m Model) actionUnit(desc string) string {
	_, target := actionVerbTarget(desc)
	target = strings.TrimPrefix(target, "systemd-")
	if i, ok := m.unitByName(target); ok {
		return m.units[i].UnitName
	}
	return ""
}

func (m Model) actionsSpec() screenSpec {
	var ok, failed, cancelled int
	lastFail, lastCancel := "", ""
	for _, a := range m.actions {
		switch {
		case a.cancelled:
			cancelled++
			lastCancel = a.desc
		case a.ok:
			ok++
		default:
			failed++
			lastFail = a.desc
		}
	}
	since := "—"
	if len(m.actions) > 0 {
		since = "since " + m.actions[0].at.Format("15:04")
	}
	tiles := []tile{
		{label: "THIS SESSION", value: fmt.Sprintf("%d", len(m.actions)), note: "actions · " + since, valueStyle: brightStyle},
		{label: "SUCCEEDED", value: fmt.Sprintf("%s %d", glyph(gDone), ok), note: "ok", valueStyle: okStyle},
		{label: "FAILED", value: fmt.Sprintf("%s %d", glyph(gFailed), failed), note: orDash(lastFail), valueStyle: errStyle},
		{label: "CANCELLED", value: fmt.Sprintf("%d", cancelled), note: orDash(lastCancel), valueStyle: mutedStyle},
	}
	if failed > 0 {
		tiles[2].tint = activePalette.err
	} else {
		tiles[2].valueStyle = mutedStyle
	}
	s := screenSpec{
		tiles: tiles,
		cols: []scrCol{
			{title: "TIME", w: 8},
			{title: "", w: 1},
			{title: "ACTION", min: 10, max: 18},
			{title: "TARGET", min: 16},
			{title: "TOOK", w: 6, right: true},
			{title: "HOW", min: 10, max: 24, hide: breakpointMid},
		},
		empty:     "no actions yet this session — start, stop, reload, linger, health and timer actions show up here",
		detail:    m.actionDetail,
		detailMax: 7,
		info:      "kept in memory · cleared on exit",
	}
	// Newest first.
	for i := len(m.actions) - 1; i >= 0; i-- {
		a := m.actions[i]
		g, st := gDone, okStyle
		switch {
		case a.cancelled:
			g, st = gWarn, warnStyle
		case !a.ok:
			g, st = gFailed, errStyle
		}
		verb, target := actionVerbTarget(a.desc)
		took := "—"
		if a.took > 0 {
			took = fmtTook(a.took)
		}
		how := a.how
		switch {
		case a.cancelled:
			how = "cancelled at confirm"
		case !a.ok && a.err != "":
			how = strings.TrimSpace(how + " · error")
		}
		s.rows = append(s.rows, scrRow{
			id:   fmt.Sprintf("act:%d", i),
			unit: m.actionUnit(a.desc),
			cells: []scrCell{
				cell(a.at.Format("15:04:05"), mutedStyle),
				cell(glyph(g), st),
				cell(verb, fgStyle),
				cell(orDash(target), fgStyle),
				cell(took, mutedStyle),
				cell(orDash(how), mutedStyle),
			},
			search: a.desc + " " + a.err,
		})
	}
	return s
}

// fmtTook is "0.8s", "21.3s", "2m4s".
func fmtTook(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func (m Model) actionAt(id string) (actionRecord, bool) {
	var i int
	if _, err := fmt.Sscanf(id, "act:%d", &i); err != nil || i < 0 || i >= len(m.actions) {
		return actionRecord{}, false
	}
	return m.actions[i], true
}

func (m Model) actionDetail(r scrRow, _ int) []string {
	a, ok := m.actionAt(r.id)
	if !ok {
		return nil
	}
	g, st := gDone, okStyle
	switch {
	case a.cancelled:
		g, st = gWarn, warnStyle
	case !a.ok:
		g, st = gFailed, errStyle
	}
	lines := []string{st.Render(glyph(g)+" "+a.desc) + mutedStyle.Render(" · "+a.at.Format("15:04:05"))}
	if c := m.actionCommand(a.desc); c != "" {
		lines = append(lines, accentStyle.Render("$ ")+fgStyle.Render(c))
	}
	if a.err != "" {
		lines = append(lines, errStyle.Render(a.err))
	}
	if u := m.actionUnit(a.desc); u != "" {
		if i, ok := m.unitByName(u); ok {
			for _, is := range m.issues[m.units[i].Name] {
				lines = append(lines, warnStyle.Render("hint ")+fgStyle.Render(is.Message)+"  "+keyHint("v", "open problem"))
				break
			}
		}
	}
	if a.hint != "" && a.ok {
		lines = append(lines, mutedStyle.Render(a.hint))
	}
	keys := []string{}
	if a.redo != nil {
		keys = append(keys, keyHint(".", "repeat"))
	}
	if m.actionCommand(a.desc) != "" {
		keys = append(keys, keyHint("c", "copy command"))
	}
	if r.unit != "" {
		keys = append(keys, keyHint("l", "logs"), keyHint("↵", "open unit"))
	}
	if len(keys) > 0 {
		lines = append(lines, strings.Join(keys, "   "))
	}
	return lines
}

// actionsKeysIn handles the actions screen's own keys.
func (m Model) actionsKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	k := msg.String()
	if k == "S" {
		return m.exportActions()
	}
	r, ok := m.selectedRow()
	if !ok {
		return m, nil, false
	}
	a, _ := m.actionAt(r.id)
	switch k {
	case ".":
		if a.redo == nil {
			m.setStatus("this action cannot be repeated from here", false)
			return m, nil, true
		}
		if m.refuse(capMutate) {
			return m, nil, true
		}
		// The work is bound to the session it first ran against: never
		// replay it after a switch, where the same words would hit
		// another host's or user's units.
		if a.epoch != 0 && a.epoch != m.epoch {
			m.setStatus("not repeated: "+a.desc+" ran against another session (switch back to repeat it)", true)
			return m, nil, true
		}
		desc, hint, run := a.desc, a.hint, a.redo
		p := &pendingAction{
			verb:  "repeat",
			title: "Repeat " + desc + "?",
			lines: []string{"run " + desc + " again"},
			run: func(m Model) (tea.Model, tea.Cmd) {
				return m, tea.Batch(m.setBusy(desc), m.actionCmdHint(desc, hint, run))
			},
		}
		// A repeat asks exactly what the original asked: a typed name
		// stays a typed name, a danger dialog stays a danger dialog.
		if g := a.guard; g != nil {
			if g.title != "" {
				p.title = "Repeat: " + strings.TrimSuffix(g.title, "?") + "?"
			}
			p.lines = append([]string{"run " + desc + " again:"}, g.lines...)
			p.keeps, p.danger, p.typed = g.keeps, g.danger, g.typed
		}
		m.pending = p
		return m, nil, true
	case "c":
		c := m.actionCommand(a.desc)
		if c == "" {
			m.setStatus("no shell command for "+a.desc, false)
			return m, nil, true
		}
		m.setStatus("copied: "+c, false)
		return m, tea.SetClipboard(c), true
	case "v":
		if r.unit == "" {
			return m, nil, true
		}
		mm, cmd := m.openView(modeValidate)
		return mm, cmd, true
	case "l":
		if r.unit == "" {
			return m, nil, true
		}
		mm, cmd := m.openUnitInList(r.unit)
		base := mm.(Model)
		if u, ok := base.selected(); ok && u.UnitName == r.unit {
			lm, lcmd := base.startLogs(u)
			return lm, tea.Batch(cmd, lcmd), true
		}
		return base, cmd, true
	}
	return m, nil, false
}

// exportActions writes the session log to a text file next to log exports.
func (m Model) exportActions() (tea.Model, tea.Cmd, bool) {
	if len(m.actions) == 0 {
		m.setStatus("nothing to export yet", false)
		return m, nil, true
	}
	var b strings.Builder
	for _, a := range m.actions {
		status := "ok"
		switch {
		case a.cancelled:
			status = "cancelled"
		case !a.ok:
			status = "FAILED"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s", a.at.Format(time.RFC3339), status, a.desc, fmtTook(a.took), a.how)
		if a.err != "" {
			fmt.Fprintf(&b, "\t%s", a.err)
		}
		b.WriteString("\n")
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.TempDir()
	}
	f, path, err := createExportFile(filepath.Join(dir, "quadman-actions-"+time.Now().Format("20060102-150405")+".tsv"))
	if err != nil {
		m.setStatus("export actions: "+err.Error(), true)
		return m, nil, true
	}
	_, werr := f.WriteString(b.String())
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		m.setStatus("export actions: "+err.Error(), true)
		return m, nil, true
	}
	m.setStatus("exported "+fmt.Sprint(len(m.actions))+" actions to "+path, false)
	return m, nil, true
}

// actionsView renders the session log as plain text (kept for the skill
// export and tests that read the log without a terminal).
func (m Model) actionsView() string {
	var b strings.Builder
	if len(m.actions) == 0 {
		b.WriteString("No actions yet this session.\n")
		b.WriteString("Start, stop, reload, linger, health, and timer actions are logged here with their time and result.\n")
		return b.String()
	}
	for _, a := range m.actions {
		mark := "ok"
		switch {
		case a.cancelled:
			mark = "SKIP"
		case !a.ok:
			mark = "FAIL"
		}
		line := fmt.Sprintf("%s  %-4s  %s", a.at.Format("15:04:05"), mark, a.desc)
		if !a.ok && a.err != "" {
			line += " - " + a.err
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
