package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/systemd"
)

// Timers screen (T): human schedules, next/last run, and the last 24
// runs of each timer's service as a ✓✗ strip.

type timersData struct {
	list     []systemd.Timer
	info     map[string]systemd.TimerInfo
	history  map[string][]systemd.RunRecord // by triggered unit
	logUnit  string                         // whose last-run log the panel shows
	logLines []string
	loadedAt time.Time
}

// maxTimerHistory bounds the per-timer journal reads on open.
const maxTimerHistory = 40

type timersMsg struct {
	data timersData
	err  error
}

func (s timersMsg) screenMode() mode        { return modeTimers }
func (s timersMsg) screenErrPrefix() string { return "timers" }
func (s timersMsg) screenErr() error        { return s.err }
func (s timersMsg) apply(m *Model) {
	logUnit, logLines := m.timers.logUnit, m.timers.logLines
	m.timers = s.data
	m.timers.logUnit, m.timers.logLines = logUnit, logLines
}

type timerLogMsg struct {
	unit  string
	lines []string
	err   error
}

func timersCmd(sys *systemd.Systemd) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		list, err := sys.ListTimers(ctx)
		if err != nil {
			return timersMsg{err: err}
		}
		d := timersData{list: list, info: map[string]systemd.TimerInfo{}, history: map[string][]systemd.RunRecord{}, loadedAt: time.Now()}
		names := make([]string, 0, len(list))
		for _, t := range list {
			names = append(names, t.Unit)
		}
		if infos, err := sys.TimerDetails(ctx, names); err == nil {
			for i, ti := range infos {
				d.info[ti.Unit] = ti
				if ti.Triggers != "" && i < maxTimerHistory {
					if runs, err := sys.RunHistory(ctx, ti.Triggers, 24); err == nil {
						d.history[ti.Triggers] = runs
					}
				}
			}
		}
		return timersMsg{data: d}
	}
}

// timerState is the STATE cell: failed runs and failed timers first.
func timerState(ti systemd.TimerInfo) (glyphKind, string) {
	switch {
	case ti.Failed():
		return gFailed, "failed"
	case ti.ActiveState == "active" && ti.SubState == "running":
		return gStarting, "running"
	case ti.ActiveState == "active":
		return gActive, "waiting"
	case ti.ActiveState == "":
		return gNotFound, "unknown"
	}
	return gInactive, "inactive"
}

// humanSchedule turns OnCalendar=/On*Sec= into words: "hourly",
// "daily 03:00", "Sun 04:00", "every 1d", "5min after boot".
func humanSchedule(expr string) string {
	key, val, ok := strings.Cut(expr, "=")
	if !ok {
		return expr
	}
	switch key {
	case "OnCalendar":
		return humanCalendar(val)
	case "OnUnitActiveUSec", "OnUnitActiveSec":
		return "every " + val
	case "OnUnitInactiveUSec", "OnUnitInactiveSec":
		return val + " after finish"
	case "OnBootUSec", "OnBootSec":
		return val + " after boot"
	case "OnStartupUSec", "OnStartupSec":
		return val + " after login"
	case "OnActiveUSec", "OnActiveSec":
		return val + " after start"
	}
	return expr
}

func humanCalendar(v string) string {
	switch v {
	case "*-*-* *:00:00":
		return "hourly"
	case "*-*-* 00:00:00":
		return "daily"
	case "Mon *-*-* 00:00:00":
		return "weekly"
	case "*-*-01 00:00:00":
		return "monthly"
	}
	f := strings.Fields(v)
	switch {
	case len(f) == 2 && f[0] == "*-*-*" && strings.HasSuffix(f[1], ":00") && !strings.ContainsAny(f[1], "*/,"):
		return "daily " + strings.TrimSuffix(f[1], ":00")
	case len(f) == 3 && f[1] == "*-*-*" && strings.HasSuffix(f[2], ":00") && !strings.ContainsAny(f[2], "*/,"):
		return f[0] + " " + strings.TrimSuffix(f[2], ":00")
	}
	return v
}

func (m Model) timerSchedule(t systemd.Timer) string {
	ti := m.timers.info[t.Unit]
	if len(ti.Schedules) == 0 {
		return "—"
	}
	s := humanSchedule(ti.Schedules[0])
	if n := len(ti.Schedules) - 1; n > 0 {
		s += fmt.Sprintf(" +%d", n)
	}
	return s
}

// timerNext is "in 38m", or "—" when nothing is scheduled.
func (m Model) timerNext(t systemd.Timer) string {
	if ti := m.timers.info[t.Unit]; !ti.Next.IsZero() {
		if d := time.Until(ti.Next); d > 0 {
			return "in " + humanDuration(d)
		}
		return "due"
	}
	if t.Left == "" || t.Left == "-" || t.Left == "n/a" {
		return "—"
	}
	return "in " + t.Left
}

// timerLast is "✓ 16:00", "✗ Sun · exit 125" or "never".
func (m Model) timerLast(t systemd.Timer) (string, glyphKind) {
	ti := m.timers.info[t.Unit]
	if ti.Last.IsZero() {
		return "never", gInactive
	}
	g := gDone
	text := shortWhen(ti.Last)
	if ti.LastResult != "" && ti.LastResult != "success" {
		g = gFailed
		if ti.LastStatus != 0 {
			text += fmt.Sprintf(" · exit %d", ti.LastStatus)
		} else {
			text += " · " + ti.LastResult
		}
	}
	return glyph(g) + " " + text, g
}

// shortWhen is "16:00" today, "Sun" this week, "Oct 2" otherwise.
func shortWhen(t time.Time) string {
	now := time.Now()
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case now.Sub(t) < 6*24*time.Hour:
		return t.Format("Mon 15:04")
	}
	return t.Format("Jan 2")
}

func (m Model) sortedTimers() []systemd.Timer {
	ts := slices.Clone(m.timers.list)
	rank := func(t systemd.Timer) int {
		g, _ := timerState(m.timers.info[t.Unit])
		return stateRank(g)
	}
	slices.SortStableFunc(ts, func(a, b systemd.Timer) int {
		if d := rank(a) - rank(b); d != 0 {
			return d
		}
		na, nb := m.timers.info[a.Unit].Next, m.timers.info[b.Unit].Next
		switch {
		case !na.IsZero() && !nb.IsZero() && !na.Equal(nb):
			return na.Compare(nb)
		case na.IsZero() != nb.IsZero():
			if na.IsZero() {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Unit, b.Unit)
	})
	return ts
}

func (m Model) timersSpec() screenSpec {
	ts := m.sortedTimers()
	var active, failed, persistent int
	var soonest systemd.TimerInfo
	failedName := ""
	for _, t := range ts {
		ti := m.timers.info[t.Unit]
		if ti.ActiveState == "active" {
			active++
		}
		if ti.Failed() {
			failed++
			if failedName == "" {
				failedName = t.Unit
			}
		}
		if ti.Persistent {
			persistent++
		}
		if !ti.Next.IsZero() && (soonest.Next.IsZero() || ti.Next.Before(soonest.Next)) {
			soonest = ti
		}
	}
	next := tile{label: "NEXT", value: "—", note: "nothing scheduled", valueStyle: mutedStyle}
	if !soonest.Next.IsZero() {
		next = tile{label: "NEXT", value: soonest.Next.Format("15:04"), valueStyle: accentStyle,
			note: strings.TrimSuffix(soonest.Unit, ".timer") + " in " + humanDuration(time.Until(soonest.Next))}
	}
	lastFailed := tile{label: "LAST FAILED", value: glyph(gDone) + " none", note: "every last run ok", valueStyle: okStyle}
	if failed > 0 {
		lastFailed = tile{label: "LAST FAILED", value: fmt.Sprintf("%s %d", glyph(gFailed), failed), note: failedName,
			valueStyle: errStyle, tint: activePalette.err}
	}
	s := screenSpec{
		tiles: []tile{
			{label: "TIMERS", value: fmt.Sprintf("%d", len(ts)), note: fmt.Sprintf("%d active", active), valueStyle: brightStyle},
			next,
			lastFailed,
			{label: "PERSISTENT", value: fmt.Sprintf("%d", persistent), note: "catch up after sleep", valueStyle: fgStyle},
		},
		cols: []scrCol{
			{title: "STATE", w: 11},
			{title: "TIMER", min: 16, max: 36},
			{title: "SCHEDULE", w: 18},
			{title: "NEXT", w: 10},
			{title: "LAST", w: 22},
			{title: "TRIGGERS", min: 14, hide: breakpointMid},
		},
		empty:  "no systemd timers in this scope",
		detail: m.timerDetail,
		info:   fmt.Sprintf("systemctl list-timers · %d timer%s", len(ts), plural(len(ts))),
	}
	for _, t := range ts {
		ti := m.timers.info[t.Unit]
		g, word := timerState(ti)
		last, lg := m.timerLast(t)
		triggers := t.Activates
		if triggers == "" {
			triggers = ti.Triggers
		}
		unit := ""
		if i, ok := m.unitByName(triggers); ok {
			unit = m.units[i].UnitName
			triggers = m.unitGlyph(triggers)
		}
		s.rows = append(s.rows, scrRow{
			id:   "timer:" + t.Unit,
			unit: unit,
			cells: []scrCell{
				cell(glyph(g)+" "+word, glyphStyle(g)),
				cell(t.Unit, fgStyle),
				cell(m.timerSchedule(t), mutedStyle),
				cell(m.timerNext(t), fgStyle),
				cell(last, glyphStyle(lg)),
				cell(triggers, mutedStyle),
			},
		})
	}
	return s
}

func (m Model) timerByID(id string) (systemd.Timer, systemd.TimerInfo, bool) {
	name := strings.TrimPrefix(id, "timer:")
	for _, t := range m.timers.list {
		if t.Unit == name {
			return t, m.timers.info[name], true
		}
	}
	return systemd.Timer{}, systemd.TimerInfo{}, false
}

func (m Model) timerDetail(r scrRow, _ int) []string {
	t, ti, ok := m.timerByID(r.id)
	if !ok {
		return nil
	}
	lines := []string{brightStyle.Render(t.Unit)}
	for _, sc := range ti.Schedules {
		key, val, _ := strings.Cut(sc, "=")
		text := humanSchedule(sc)
		if text != val {
			text += mutedStyle.Render("  (" + val + ")")
		}
		lines = append(lines, detailField(key, text))
	}
	if !ti.Next.IsZero() {
		lines = append(lines, detailField("Next", ti.Next.Format("2006-01-02 15:04:05 MST")))
	}
	opts := "Persistent " + yesNo(ti.Persistent)
	if ti.RandomDelay != "" && ti.RandomDelay != "0" {
		opts += " · RandomizedDelaySec=" + ti.RandomDelay
	}
	if ti.Accuracy != "" {
		opts += " · AccuracySec=" + ti.Accuracy
	}
	lines = append(lines, detailField("Options", opts))
	if ti.Triggers != "" {
		lines = append(lines, detailField("Runs", ti.Triggers+m.timerUnitNote(ti.Triggers)))
	}
	if runs := m.timers.history[ti.Triggers]; len(runs) > 0 {
		lines = append(lines, detailField("History", historyStrip(runs)))
	}
	if ti.Path != "" {
		lines = append(lines, detailField("File", ti.Path+mutedStyle.Render(" · "+orDash(ti.FileState))))
	}
	if m.timers.logUnit == ti.Triggers && len(m.timers.logLines) > 0 {
		lines = append(lines, dimStyle.Bold(true).Render("LAST RUN LOG"))
		for _, l := range m.timers.logLines {
			lines = append(lines, mutedStyle.Render(l))
		}
	}
	return lines
}

func (m Model) timerUnitNote(unit string) string {
	if _, ok := m.unitByName(unit); ok {
		return mutedStyle.Render(" → ") + m.unitGlyph(unit)
	}
	return ""
}

// historyStrip renders runs as ✓✗ plus a one-line summary.
func historyStrip(runs []systemd.RunRecord) string {
	var b strings.Builder
	failed := 0
	var lastFail time.Time
	for _, r := range runs {
		if r.OK {
			b.WriteString(okStyle.Render(glyph(gDone)))
		} else {
			failed++
			lastFail = r.At
			b.WriteString(errStyle.Render(glyph(gFailed)))
		}
	}
	note := fmt.Sprintf("  last %d run%s", len(runs), plural(len(runs)))
	if failed > 0 {
		note += fmt.Sprintf(" · %d failed, last %s", failed, shortWhen(lastFail))
	}
	return b.String() + mutedStyle.Render(note)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// timersKeysIn handles the timers screen's own keys.
func (m Model) timersKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	k := msg.String()
	if k == "r" {
		return m, tea.Batch(m.setBusy("systemd timers"), timersCmd(m.sys)), true
	}
	r, ok := m.selectedRow()
	if !ok {
		return m, nil, false
	}
	t, ti, _ := m.timerByID(r.id)
	sys := m.sys
	act := func(desc string, run func(context.Context) (string, error)) (tea.Model, tea.Cmd, bool) {
		if m.refuseReadonly() {
			return m, nil, true
		}
		return m, tea.Batch(m.setBusy(desc), actionCmd(desc, run), timersCmd(sys)), true
	}
	switch k {
	case "n":
		if ti.Triggers == "" {
			m.setStatus(t.Unit+": no triggered unit known", true)
			return m, nil, true
		}
		return act("start "+ti.Triggers, func(ctx context.Context) (string, error) {
			return sys.UnitAction(ctx, "start", ti.Triggers)
		})
	case "s":
		return act("start "+t.Unit, func(ctx context.Context) (string, error) { return sys.UnitAction(ctx, "start", t.Unit) })
	case "x":
		return act("stop "+t.Unit, func(ctx context.Context) (string, error) { return sys.UnitAction(ctx, "stop", t.Unit) })
	case "e":
		return act("enable "+t.Unit, func(ctx context.Context) (string, error) { return sys.Enable(ctx, t.Unit, true) })
	case "d":
		return act("disable "+t.Unit, func(ctx context.Context) (string, error) { return sys.Disable(ctx, t.Unit, true) })
	case "l":
		if m.timers.logUnit == ti.Triggers && len(m.timers.logLines) > 0 {
			m.timers.logUnit, m.timers.logLines = "", nil
			return m, nil, true
		}
		unit := ti.Triggers
		return m, tea.Batch(m.setBusy("journal "+unit), func() tea.Msg {
			out, err := sys.Journal(context.Background(), unit, 8)
			var lines []string
			if out != "" {
				lines = strings.Split(out, "\n")
			}
			return timerLogMsg{unit: unit, lines: lines, err: err}
		}), true
	case "E":
		if ti.Path == "" {
			m.setStatus(t.Unit+": unit file unknown", true)
			return m, nil, true
		}
		if !userWritable(ti.Path) {
			m.setStatus(ti.Path+" is not yours to edit: copy it to ~/.config/systemd/user/ (or use systemctl --user edit)", true)
			return m, nil, true
		}
		mm, cmd := m.editPathAt(ti.Path, 0)
		return mm, cmd, true
	}
	return m, nil, false
}

// userWritable reports a local file the current user may write.
func userWritable(path string) bool {
	f, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY, 0) // #nosec G304 -- probing write access to a unit file the user asked to edit
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
