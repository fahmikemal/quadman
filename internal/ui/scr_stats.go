package ui

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Stats screen (o): host totals with a CPU sparkline, inline CPU bars,
// sortable columns, idle containers folded away, and a link from every
// container to its quadlet unit. It re-samples on every poll tick while
// open.

type statsData struct {
	entries   []podman.StatEntry
	host      podman.HostInfo
	ctrs      []podman.ContainerEntry
	cpuHist   []float64 // total CPU% per sample, oldest first
	sampledAt time.Time
	showIdle  bool
}

// statsHistLen is how many samples the sparkline keeps.
const statsHistLen = 30

type statsMsg struct {
	entries []podman.StatEntry
	host    podman.HostInfo
	ctrs    []podman.ContainerEntry
	err     error
}

func (s statsMsg) screenMode() mode        { return modeStats }
func (s statsMsg) screenErrPrefix() string { return "stats" }
func (s statsMsg) screenErr() error        { return s.err }
func (s statsMsg) apply(m *Model) {
	m.stats.entries = s.entries
	if s.host.CPUs > 0 {
		m.stats.host = s.host
	}
	if s.ctrs != nil {
		m.stats.ctrs = s.ctrs
	}
	total := 0.0
	for _, e := range s.entries {
		total += pct(e.CPU)
	}
	m.stats.cpuHist = append(m.stats.cpuHist, total)
	if len(m.stats.cpuHist) > statsHistLen {
		m.stats.cpuHist = m.stats.cpuHist[len(m.stats.cpuHist)-statsHistLen:]
	}
	m.stats.sampledAt = time.Now()
}

// statsCmd samples every container (running or stopped; stopped rows read
// zero). Host info and container labels are fetched alongside; failures
// there only cost the tiles and unit links, never the table.
func statsCmd(pm podman.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		entries, err := pm.Stats(ctx)
		if err != nil {
			return statsMsg{err: err}
		}
		host, _ := pm.Info(ctx)
		ctrs, _ := pm.Containers(ctx, false)
		return statsMsg{entries: entries, host: host, ctrs: ctrs}
	}
}

// pct parses "3.21%" (and "--" for stopped containers) to 3.21.
func pct(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	if err != nil || math.IsNaN(v) {
		return 0
	}
	return v
}

// pairBytes splits "18MB / 4.1MB" into byte counts.
func pairBytes(s string) (int64, int64) {
	a, b, _ := strings.Cut(s, "/")
	return podman.ParseBytes(a), podman.ParseBytes(b)
}

// statsCols are the sortable columns, in ←/→ order.
var statsSortKeys = []string{"CPU%", "MEM", "NET", "NAME", "PIDS"}

func (m Model) statsSpec() screenSpec {
	d := m.stats
	var memUsed, rx, tx int64
	busy := 0
	for _, e := range d.entries {
		u, _ := pairBytes(e.Mem)
		memUsed += u
		r, t := pairBytes(e.Net)
		rx += r
		tx += t
		if pct(e.CPU) > 0 {
			busy++
		}
	}
	cpuNow := 0.0
	if n := len(d.cpuHist); n > 0 {
		cpuNow = d.cpuHist[n-1]
	}
	cores := "cores ?"
	if d.host.CPUs > 0 {
		cores = fmt.Sprintf("%d cores", d.host.CPUs)
	}
	memNote := "host total unknown"
	if d.host.MemTotal > 0 {
		memNote = "of " + podman.HumanBytes(d.host.MemTotal)
	}
	tiles := []tile{
		{label: "CPU · " + cores, value: fmt.Sprintf("%.1f%%", cpuNow), note: sparkline(d.cpuHist, 18), valueStyle: accentStyle},
		{label: "MEMORY", value: podman.HumanBytes(memUsed), note: memNote, valueStyle: brightStyle},
		{label: "NET I/O", value: "↓ " + podman.HumanBytes(rx), note: "↑ " + podman.HumanBytes(tx), valueStyle: fgStyle},
		{label: "CONTAINERS", value: fmt.Sprintf("%d", len(d.entries)), note: fmt.Sprintf("%d busy · %d idle", busy, len(d.entries)-busy), valueStyle: brightStyle},
	}
	sortKey := statsSortKeys[m.scrSort%len(statsSortKeys)]
	title := func(t string) string {
		if t == sortKey {
			return t + " ▾"
		}
		return t
	}
	s := screenSpec{
		tiles:   tiles,
		summary: fmt.Sprintf(" CPU %.1f%% · MEM %s · NET ↓%s ↑%s · %d ctr", cpuNow, podman.HumanBytes(memUsed), podman.HumanBytes(rx), podman.HumanBytes(tx), len(d.entries)),
		cols: []scrCol{
			{title: title("NAME"), min: 16, max: 34},
			{title: title("CPU%"), w: 17},
			{title: title("MEM"), w: 9, right: true},
			{title: "MEM%", w: 6, right: true},
			{title: title("NET") + " ↓/↑", w: 17},
			{title: "BLOCK R/W", w: 19, hide: breakpointMid},
			{title: title("PIDS"), w: 6, right: true},
		},
		empty:  "no containers (podman stats --all is empty)",
		detail: m.statDetail,
		info:   "podman stats · sampled " + ago(d.sampledAt),
	}
	entries := slices.Clone(d.entries)
	slices.SortStableFunc(entries, func(a, b podman.StatEntry) int {
		var x, y float64
		switch sortKey {
		case "MEM":
			ma, _ := pairBytes(a.Mem)
			mb, _ := pairBytes(b.Mem)
			x, y = float64(ma), float64(mb)
		case "NET":
			ra, ta := pairBytes(a.Net)
			rb, tb := pairBytes(b.Net)
			x, y = float64(ra+ta), float64(rb+tb)
		case "PIDS":
			x, y = pct(a.PIDs), pct(b.PIDs)
		case "NAME":
			return strings.Compare(a.Name, b.Name)
		default:
			x, y = pct(a.CPU), pct(b.CPU)
		}
		switch {
		case x > y:
			return -1
		case x < y:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	maxCPU := 0.0
	for _, e := range entries {
		maxCPU = math.Max(maxCPU, pct(e.CPU))
	}
	var idle []podman.StatEntry
	for _, e := range entries {
		if pct(e.CPU) == 0 {
			idle = append(idle, e)
			continue
		}
		s.rows = append(s.rows, m.statRow(e, maxCPU))
	}
	if len(idle) > 0 {
		// With nothing busy, folding the idle group would leave an empty
		// table: show it.
		show := d.showIdle || busy == 0
		hint := "i show"
		if show {
			hint = "i hide"
		}
		if busy == 0 {
			hint = "nothing is using CPU"
		}
		s.rows = append(s.rows, scrRow{heading: fmt.Sprintf("IDLE · %d container%s at 0.00%% CPU  (%s)", len(idle), plural(len(idle)), hint)})
		if show {
			for _, e := range idle {
				r := m.statRow(e, maxCPU)
				r.dim = true
				s.rows = append(s.rows, r)
			}
		}
	}
	return s
}

func (m Model) statUnit(name string) string {
	for _, c := range m.stats.ctrs {
		if c.Name() == name {
			return m.ctrUnit(c)
		}
	}
	for _, u := range m.units {
		if u.Kind == quadlet.KindContainer && m.containerName(u) == name {
			return u.UnitName
		}
	}
	return ""
}

func (m Model) statRow(e podman.StatEntry, maxCPU float64) scrRow {
	cpu := pct(e.CPU)
	name := e.Name
	unit := m.statUnit(e.Name)
	if unit != "" {
		if i, ok := m.unitByName(unit); ok && m.containerHealth(m.units[i]) == "unhealthy" {
			name += " " + glyph(gUnhealthy)
		}
	}
	ratio := 0.0
	if maxCPU > 0 {
		ratio = cpu / maxCPU
	}
	return scrRow{
		id:   "ctr:" + e.Name,
		unit: unit,
		cells: []scrCell{
			cell(name, fgStyle),
			cell(fmt.Sprintf("%s %5.2f", bar(ratio, 10), cpu), accentStyle),
			cell(strings.TrimSpace(strings.SplitN(e.Mem, "/", 2)[0]), fgStyle),
			cell(e.MemPerc, mutedStyle),
			cell(e.Net, mutedStyle),
			cell(e.Block, mutedStyle),
			cell(e.PIDs, mutedStyle),
		},
	}
}

func (m Model) statDetail(r scrRow, _ int) []string {
	name := strings.TrimPrefix(r.id, "ctr:")
	for _, e := range m.stats.entries {
		if e.Name != name {
			continue
		}
		lines := []string{brightStyle.Render(e.Name) + mutedStyle.Render("  "+shortHex(e.ID))}
		if r.unit != "" {
			lines = append(lines, detailField("Unit", m.unitGlyph(r.unit)))
		}
		lines = append(lines,
			detailField("CPU", e.CPU+mutedStyle.Render("  (100% = one core)")),
			detailField("Memory", e.Mem+" · "+e.MemPerc),
			detailField("Net / Block", e.Net+"   "+e.Block),
		)
		keys := keyHint("r", "refresh")
		if r.unit != "" {
			keys += "   " + keyHint("l", "logs") + "   " + keyHint("↵", "open unit")
		}
		return append(lines, keys)
	}
	return nil
}

// bar draws a fixed-width meter with eighth-block resolution.
func bar(ratio float64, width int) string {
	ratio = math.Max(0, math.Min(1, ratio))
	if asciiGlyphs {
		n := int(math.Round(ratio * float64(width)))
		return strings.Repeat("#", n) + strings.Repeat(".", width-n)
	}
	eighths := int(math.Round(ratio * float64(width*8)))
	full, part := eighths/8, eighths%8
	s := strings.Repeat("█", full)
	if part > 0 {
		s += string([]rune("▏▎▍▌▋▊▉")[part-1])
	}
	return s + strings.Repeat("░", width-ansiWidth(s))
}

func ansiWidth(s string) int { return len([]rune(s)) }

// sparkline renders values as ▁▂▃▄▅▆▇█, newest on the right.
func sparkline(vals []float64, width int) string {
	if len(vals) == 0 {
		return "collecting…"
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	peak := 0.0
	for _, v := range vals {
		peak = math.Max(peak, v)
	}
	levels := []rune("▁▂▃▄▅▆▇█")
	if asciiGlyphs {
		levels = []rune("_.-=+*#@")
	}
	var b strings.Builder
	for _, v := range vals {
		i := 0
		if peak > 0 {
			i = int(math.Round(v / peak * float64(len(levels)-1)))
		}
		b.WriteRune(levels[i])
	}
	return b.String()
}

// statsKeysIn handles the stats screen's own keys.
func (m Model) statsKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "r":
		return m, tea.Batch(m.setBusy("stats"), statsCmd(m.pm)), true
	case "i":
		m.stats.showIdle = !m.stats.showIdle
		return m, nil, true
	case "right":
		m.scrSort = (m.scrSort + 1) % len(statsSortKeys)
		return m, nil, true
	case "left":
		m.scrSort = (m.scrSort + len(statsSortKeys) - 1) % len(statsSortKeys)
		return m, nil, true
	case "l":
		r, ok := m.selectedRow()
		if !ok || r.unit == "" {
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
