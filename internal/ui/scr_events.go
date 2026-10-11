package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// Events screen (w): one timeline merged from `podman events`, the
// service manager's own messages about quadlet units (started, failed,
// scheduled restart), and quadman's own actions this session.

// eventRec is one timeline row.
type eventRec struct {
	at     time.Time
	typ    string // container, image, volume, network, pod, service, quadman, system
	event  string
	object string
	detail string
	unit   string // generated unit name, when known
	sev    glyphKind
	hasSev bool
}

type eventsData struct {
	recs     []eventRec // oldest first
	paused   bool
	pausedAt time.Time
	typeIdx  int           // index into eventTypeFilters
	unitOnly string        // generated unit name, "" = all
	since    time.Duration // display window
	loaded   time.Duration // history window already fetched
	jsess    *logSession   // systemd manager journal stream
}

// eventBufferCap bounds the timeline.
const eventBufferCap = 5000

var eventTypeFilters = []struct{ key, label string }{
	{"1", "all"}, {"2", "container"}, {"3", "image"}, {"4", "service"}, {"5", "quadman"}, {"6", "other"},
}

var eventWindows = []time.Duration{15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}

type eventsHistoryMsg struct {
	recs   []eventRec
	window time.Duration
	err    error
}

// startEvents opens the screen: live podman + journal streams, plus a
// history fetch so the last hour is there immediately.
func (m Model) startEvents() (tea.Model, tea.Cmd) {
	m.stopEvents()
	stop, stream, err := podman.EventsFollow(context.Background())
	if err != nil {
		m.setStatus("podman events: "+err.Error(), true)
		return m, nil
	}
	sess := &logSession{stop: stop, scan: bufio.NewScanner(stream), unit: "events"}
	sess.scan.Buffer(make([]byte, 256*1024), 256*1024)
	m.eventSess = sess
	cmds := []tea.Cmd{followLine(sess)}
	units := unitNames(m.units)
	if len(units) > 0 {
		if jstop, jstream, err := m.sys.FollowManagerJournal(context.Background(), units); err == nil {
			js := &logSession{stop: jstop, scan: bufio.NewScanner(jstream), unit: "events-journal"}
			js.scan.Buffer(make([]byte, 256*1024), 256*1024)
			m.events.jsess = js
			cmds = append(cmds, followLine(js))
		}
	}
	m.events.recs = nil
	m.events.paused = false
	m.events.since = time.Hour
	m.events.loaded = 0
	m.enterScreen(modeEvents)
	m.clearStatus()
	cmds = append(cmds, m.setBusy("events history"), m.eventsHistoryCmd(time.Hour))
	return m, tea.Batch(cmds...)
}

func (m *Model) stopEvents() {
	if m.eventSess != nil {
		m.eventSess.stop()
		m.eventSess = nil
	}
	if m.events.jsess != nil {
		m.events.jsess.stop()
		m.events.jsess = nil
	}
}

// eventsHistoryCmd loads podman events and manager messages for window.
func (m Model) eventsHistoryCmd(window time.Duration) tea.Cmd {
	sys := m.sys
	units := unitNames(m.units)
	resolve := m.eventUnitResolver()
	return func() tea.Msg {
		ctx := context.Background()
		evs, perr := podman.EventsSince(ctx, fmtWindow(window))
		var recs []eventRec
		for _, e := range evs {
			recs = append(recs, podmanEventRec(e, resolve))
		}
		entries, jerr := sys.ManagerJournal(ctx, units, "-"+fmtWindow(window))
		for _, e := range entries {
			recs = append(recs, managerEventRec(e))
		}
		slices.SortStableFunc(recs, func(a, b eventRec) int { return a.at.Compare(b.at) })
		return eventsHistoryMsg{recs: recs, window: window, err: errors.Join(perr, jerr)}
	}
}

// eventUnitResolver maps an event to one of our quadlet units: its
// PODMAN_SYSTEMD_UNIT label when that names a quadlet unit (podman-compose
// stamps the label too), else the default systemd-<name> container name.
func (m Model) eventUnitResolver() func(label, name string) string {
	known := map[string]bool{}
	byCtr := map[string]string{}
	for _, u := range m.units {
		known[u.UnitName] = true
		byCtr["systemd-"+u.Name] = u.UnitName
	}
	return func(label, name string) string {
		if known[label] {
			return label
		}
		return byCtr[name]
	}
}

func podmanEventRec(e podman.Event, resolve func(label, name string) string) eventRec {
	r := eventRec{at: e.At(), typ: strings.ToLower(e.Type), event: e.Status, object: e.Name}
	if r.typ == "container" {
		r.unit = resolve(e.SystemdUnit(), e.Name)
	}
	if r.object == "" {
		r.object = shortHex(e.ID)
	}
	switch e.Status {
	case "health_status":
		r.event = e.HealthStatus
		switch e.HealthStatus {
		case "unhealthy":
			r.sev, r.hasSev = gUnhealthy, true
			r.event = glyph(gUnhealthy) + " unhealthy"
		case "healthy":
			r.sev, r.hasSev = gActive, true
			r.event = glyph(gActive) + " healthy"
		}
	case "died":
		if e.ContainerExit != nil {
			r.detail = exitDetail(*e.ContainerExit)
			if *e.ContainerExit != 0 {
				r.sev, r.hasSev = gWarn, true
			}
		}
	case "pull":
		r.detail = e.Image
	}
	if r.detail == "" && e.ID != "" {
		r.detail = shortHex(e.ID) + "…"
	}
	if r.typ == "" {
		r.typ = "system"
	}
	return r
}

// exitDetail explains signal exit codes: 137 = SIGKILL, 143 = SIGTERM.
func exitDetail(code int) string {
	switch code {
	case 137:
		return "exit 137 (SIGKILL)"
	case 143:
		return "exit 143 (SIGTERM)"
	}
	return fmt.Sprintf("exit %d", code)
}

func managerEventRec(e systemd.ManagerEntry) eventRec {
	r := eventRec{at: e.At, typ: "service", object: e.Unit, detail: e.Message}
	msg := e.Message
	switch {
	case e.JobType == "start" && e.JobResult == "done":
		r.event, r.sev, r.hasSev = glyph(gActive)+" started", gActive, true
	case e.JobResult == "failed" || e.JobResult == "timeout" || e.JobResult == "dependency":
		r.event, r.sev, r.hasSev = glyph(gFailed)+" failed", gFailed, true
	case strings.Contains(msg, "Failed with result"):
		r.event, r.sev, r.hasSev = glyph(gFailed)+" failed", gFailed, true
	case strings.Contains(msg, "Scheduled restart job"):
		r.event, r.sev, r.hasSev = "↻ restart", gWarn, true
	case strings.Contains(msg, "Main process exited"):
		r.event = "exited"
	case e.JobType == "stop" && e.JobResult == "done":
		r.event, r.sev, r.hasSev = glyph(gInactive)+" stopped", gInactive, true
	case e.JobType != "" && e.JobResult == "":
		r.event = e.JobType + "…"
	default:
		r.event = "log"
		if e.Priority <= 3 {
			r.sev, r.hasSev = gFailed, true
		} else if e.Priority == 4 {
			r.sev, r.hasSev = gWarn, true
		}
	}
	// "demo-web.service: Failed with result 'exit-code'." → drop the unit prefix.
	if rest, ok := strings.CutPrefix(r.detail, e.Unit+": "); ok {
		r.detail = rest
	}
	r.unit = e.Unit
	return r
}

// applyEventLine routes one live line from either stream.
func (m *Model) applyEventLine(msg logLineMsg) tea.Cmd {
	switch {
	case msg.sess == m.eventSess && m.eventSess != nil:
		if e, ok := podman.ParseEvent(msg.line); ok {
			m.appendEvent(podmanEventRec(e, m.eventUnitResolver()))
		}
		return followLine(m.eventSess)
	case msg.sess == m.events.jsess && m.events.jsess != nil:
		if e, ok := systemd.ParseManagerEntry(msg.line); ok {
			m.appendEvent(managerEventRec(e))
		}
		return followLine(m.events.jsess)
	}
	return nil
}

func (m *Model) appendEvent(r eventRec) {
	m.events.recs = append(m.events.recs, r)
	if len(m.events.recs) > eventBufferCap {
		m.events.recs = m.events.recs[len(m.events.recs)-eventBufferCap:]
	}
	// Live and on the newest row: stay pinned to the top as rows arrive.
}

// applyEventsHistory merges fetched history with what streamed meanwhile.
func (m Model) applyEventsHistory(msg eventsHistoryMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.setStatus("events history incomplete: "+msg.err.Error(), true)
	}
	seen := map[string]bool{}
	key := func(r eventRec) string { return r.at.Format(time.RFC3339Nano) + r.typ + r.event + r.object }
	merged := make([]eventRec, 0, len(msg.recs)+len(m.events.recs))
	for _, r := range append(slices.Clone(msg.recs), m.events.recs...) {
		if k := key(r); !seen[k] {
			seen[k] = true
			merged = append(merged, r)
		}
	}
	slices.SortStableFunc(merged, func(a, b eventRec) int { return a.at.Compare(b.at) })
	if len(merged) > eventBufferCap {
		merged = merged[len(merged)-eventBufferCap:]
	}
	m.events.recs = merged
	m.events.loaded = max(m.events.loaded, msg.window)
	return m, nil
}

// eventTimeline returns the filtered rows, newest first, with quadman's
// own actions merged in.
func (m Model) eventTimeline() []eventRec {
	all := slices.Clone(m.events.recs)
	for _, a := range m.actions {
		r := eventRec{at: a.at, typ: "quadman", object: "", detail: a.how}
		verb, target := actionVerbTarget(a.desc)
		r.event, r.object, r.unit = verb, target, m.actionUnit(a.desc)
		switch {
		case a.cancelled:
			r.sev, r.hasSev, r.detail = gWarn, true, "cancelled"
		case !a.ok:
			r.sev, r.hasSev, r.detail = gFailed, true, a.err
		}
		all = append(all, r)
	}
	cutoff := time.Now().Add(-m.events.since)
	upto := time.Now()
	if m.events.paused {
		upto = m.events.pausedAt
	}
	typ := eventTypeFilters[m.events.typeIdx].label
	var out []eventRec
	for _, r := range all {
		if r.at.Before(cutoff) || r.at.After(upto) {
			continue
		}
		if m.events.unitOnly != "" && r.unit != m.events.unitOnly {
			continue
		}
		switch typ {
		case "all":
		case "other":
			if slices.Contains([]string{"container", "image", "service", "quadman"}, r.typ) {
				continue
			}
		default:
			if r.typ != typ {
				continue
			}
		}
		out = append(out, r)
	}
	slices.SortStableFunc(out, func(a, b eventRec) int { return b.at.Compare(a.at) })
	return out
}

func (m Model) eventsSpec() screenSpec {
	rows := m.eventTimeline()
	var failures, restarts, pulls int
	failedUnits := map[string]bool{}
	restartBy := map[string]int{}
	for _, r := range rows {
		switch {
		case r.hasSev && r.sev == gFailed:
			failures++
			if r.unit != "" {
				failedUnits[r.unit] = true
			}
		case strings.HasPrefix(r.event, "↻"):
			restarts++
			restartBy[r.unit]++
		}
		if r.typ == "image" && r.event == "pull" {
			pulls++
		}
	}
	loop, loopN := "", 0
	for u, n := range restartBy {
		if n >= 3 && n > loopN {
			loop, loopN = u, n
		}
	}
	window := fmtWindow(m.events.since)
	tiles := []tile{
		{label: "LAST " + strings.ToUpper(window), value: fmt.Sprintf("%d", len(rows)), note: "events", valueStyle: brightStyle},
		{label: "FAILURES", value: fmt.Sprintf("%s %d", glyph(gFailed), failures), note: fmt.Sprintf("%d unit%s", len(failedUnits), plural(len(failedUnits))), valueStyle: errStyle},
		{label: "RESTARTS", value: fmt.Sprintf("↻ %d", restarts), note: "no crash loop", valueStyle: warnStyle},
		{label: "PULLS", value: fmt.Sprintf("%d", pulls), note: "image pulls", valueStyle: fgStyle},
	}
	if failures > 0 {
		tiles[1].tint = activePalette.err
	} else {
		tiles[1].valueStyle = mutedStyle
	}
	if loop != "" {
		tiles[2].note = "crash-loop " + strings.TrimSuffix(loop, ".service")
		tiles[2].tint = activePalette.warn
	} else if restarts == 0 {
		tiles[2].valueStyle = mutedStyle
	}

	live := okStyle.Render(glyph(gActive) + " LIVE")
	if m.events.paused {
		live = warnStyle.Render("❚❚ PAUSED")
	}
	unit := "all"
	if m.events.unitOnly != "" {
		unit = m.events.unitOnly
	}
	toolbar := " " + live + "   " + dimStyle.Render("type ") + fgStyle.Render(eventTypeFilters[m.events.typeIdx].label) +
		"   " + dimStyle.Render("unit ") + fgStyle.Render(unit) + "   " + dimStyle.Render("since ") + fgStyle.Render(window) +
		"   " + mutedStyle.Render("podman events + systemd journal + quadman actions")

	s := screenSpec{
		tiles:   tiles,
		toolbar: toolbar,
		cols: []scrCol{
			{title: "TIME", w: 8},
			{title: "TYPE", w: 9},
			{title: "EVENT", w: 13},
			{title: "OBJECT", min: 16, max: 34},
			{title: "DETAIL", min: 12, hide: breakpointMid},
		},
		empty:  "no events in this window yet — they stream in live",
		detail: m.eventDetail,
		info:   fmt.Sprintf("streaming · %d in the last %s", len(rows), window),
	}
	if m.events.paused {
		s.info = "paused at " + m.events.pausedAt.Format("15:04:05") + " · f resumes"
	}
	for i, r := range rows {
		st := fgStyle
		if r.hasSev {
			st = glyphStyle(r.sev)
		}
		s.rows = append(s.rows, scrRow{
			id:   fmt.Sprintf("ev:%d:%s", i, r.at.Format(time.RFC3339Nano)),
			unit: r.unit,
			cells: []scrCell{
				cell(r.at.Format("15:04:05"), mutedStyle),
				cell(r.typ, kindStyle),
				cell(r.event, st),
				cell(orDash(r.object), fgStyle),
				cell(r.detail, mutedStyle),
			},
		})
	}
	return s
}

func (m Model) eventDetail(r scrRow, _ int) []string {
	if len(r.cells) < 5 {
		return nil
	}
	lines := []string{
		brightStyle.Render(r.cells[2].text+" · "+r.cells[3].text) + mutedStyle.Render("  "+r.cells[1].text+" · "+r.cells[0].text),
	}
	if d := r.cells[4].text; d != "" {
		lines = append(lines, fgStyle.Render(d))
	}
	if r.unit != "" {
		lines = append(lines, detailField("Unit", m.unitGlyph(r.unit))+"   "+keyHint("u", "this unit only")+"   "+keyHint("↵", "open unit"))
	}
	return lines
}

// eventsKeysIn handles the events screen's own keys.
func (m Model) eventsKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	k := msg.String()
	switch k {
	case "f":
		m.events.paused = !m.events.paused
		m.following = !m.events.paused
		if m.events.paused {
			m.events.pausedAt = time.Now()
		}
		return m, nil, true
	case "1", "2", "3", "4", "5", "6":
		for i, f := range eventTypeFilters {
			if f.key == k {
				m.events.typeIdx = i
			}
		}
		m.scrCur, m.scrOff = 0, 0
		return m, nil, true
	case "u":
		if m.events.unitOnly != "" {
			m.events.unitOnly = ""
		} else if r, ok := m.selectedRow(); ok && r.unit != "" {
			m.events.unitOnly = r.unit
		} else if u, ok := m.selected(); ok {
			m.events.unitOnly = u.UnitName
		}
		m.scrCur, m.scrOff = 0, 0
		return m, nil, true
	case "s":
		i := slices.Index(eventWindows, m.events.since)
		m.events.since = eventWindows[(i+1)%len(eventWindows)]
		m.scrCur, m.scrOff = 0, 0
		if m.events.since > m.events.loaded {
			return m, tea.Batch(m.setBusy("events history"), m.eventsHistoryCmd(m.events.since)), true
		}
		return m, nil, true
	case "S":
		return m.exportEvents()
	}
	return m, nil, false
}

// exportEvents writes the visible timeline as TSV.
func (m Model) exportEvents() (tea.Model, tea.Cmd, bool) {
	rows := m.eventTimeline()
	if len(rows) == 0 {
		m.setStatus("nothing to export", false)
		return m, nil, true
	}
	var b strings.Builder
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", r.at.Format(time.RFC3339), r.typ, r.event, r.object, r.detail)
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = os.TempDir()
	}
	f, path, err := createExportFile(filepath.Join(dir, "quadman-events-"+time.Now().Format("20060102-150405")+".tsv"))
	if err != nil {
		m.setStatus("export events: "+err.Error(), true)
		return m, nil, true
	}
	_, werr := f.WriteString(b.String())
	if err := errors.Join(werr, f.Close()); err != nil {
		m.setStatus("export events: "+err.Error(), true)
		return m, nil, true
	}
	m.setStatus(fmt.Sprintf("exported %d events to %s", len(rows), path), false)
	return m, nil, true
}

// fmtWindow renders a whole-unit window as "15m", "1h", "24h" — a form
// both podman --since and journalctl --since accept.
func fmtWindow(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
