package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// selectedName returns the selected unit's name, or "" when none.
func (m Model) selectedName() string {
	if u, ok := m.selected(); ok {
		return u.Name
	}
	return ""
}

// cycleTab moves the detail view one tab in the given direction (+1/-1),
// loading each tab's content lazily.
func (m Model) cycleTab(dir int) (tea.Model, tea.Cmd) {
	m.tab = ((m.tab+dir)%tabCount + tabCount) % tabCount
	m.clearSearch()
	switch m.tab {
	case tabOverview:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.viewport.SetContent(m.overviewContent(u))
		m.viewport.GotoTop()
		return m, nil
	case tabSource:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		content, err := m.readUnitFile(u)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		m.viewport.SetContent(m.fileContent(u, content))
		m.viewport.GotoTop()
		return m, nil
	case tabStatus:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("status "+u.UnitName), func() tea.Msg {
			text, _ := sys.StatusText(context.Background(), u.UnitName)
			return statusMsg{content: text}
		})
	case tabJournal:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.sess == nil || m.sess.unit != u.UnitName {
			return m.startLogs(u)
		}
		return m, nil
	case tabInspect:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if u.Kind != quadlet.KindContainer {
			m.viewport.SetContent("podman inspect only applies to container units\n")
			m.viewport.GotoTop()
			return m, nil
		}
		container := m.containerName(u)
		pm := m.pm
		return m, tea.Batch(m.setBusy("inspect "+container), func() tea.Msg {
			text, err := pm.Inspect(context.Background(), container)
			if err != nil {
				text = err.Error()
			}
			return inspectMsg{content: text}
		})
	}
	return m, nil
}

// tabBar renders the detail view's tab strip with the active tab highlighted
// and the unit name next to it.
func (m Model) tabBar(unit string) string {
	tabs := []string{"overview", "source", "status", "journal", "inspect"}
	var b strings.Builder
	for i, t := range tabs {
		if i == m.tab {
			b.WriteString(tabActiveStyle.Render(" " + t + " "))
		} else {
			b.WriteString(" " + fgStyle.Render(t) + " ")
		}
	}
	b.WriteString("  " + keyHint("[ ]", "switch tab"))
	if unit != "" {
		b.WriteString("  " + mutedStyle.Render(unit))
	}
	if m.tab == tabJournal {
		state := okStyle.Render(glyph(gActive) + " live")
		switch {
		case m.sess == nil:
			state = mutedStyle.Render("snapshot")
		case !m.following:
			state = warnStyle.Render("❚❚ paused")
		}
		prio := "all"
		if m.logPriority != "" {
			prio = "≤ " + m.logPriority
		}
		state += dimStyle.Render("  prio ") + fgStyle.Render(prio)
		if m.logFilter != "" {
			state += dimStyle.Render("  grep ") + fgStyle.Render(fmt.Sprintf("%q [%d/%d]", m.logFilter, len(m.visibleLogLines()), len(m.logLines)))
		}
		state += dimStyle.Render(fmt.Sprintf("  %d lines", len(m.logLines)))
		b.WriteString("   " + state)
	}
	return b.String()
}

// detailHeader is the line above the tabs: unit, state, uptime, restarts.
func (m Model) detailHeader(w int) string {
	u, ok := m.selected()
	if !ok {
		return ""
	}
	st := m.stateOf(u)
	status := m.status[u.UnitName]
	parts := []string{
		brightStyle.Render(u.UnitName),
		glyphStyle(st.glyph).Render(glyph(st.glyph) + " " + st.word + subParen(st)),
	}
	if up := uptimeOf(status, time.Now()); up != "—" && st.glyph != gInactive && st.glyph != gFailed {
		parts = append(parts, mutedStyle.Render("up "+up))
	}
	if status.NRestarts > 0 {
		r := fmt.Sprintf("↻ %d restart%s", status.NRestarts, plural(status.NRestarts))
		if status.NRestarts >= 3 {
			parts = append(parts, warnStyle.Render(r+" · crash-loop? check Restart= / HealthCmd"))
		} else {
			parts = append(parts, mutedStyle.Render(r))
		}
	}
	if e := m.podmanInfo[u.UnitName]; e.App != "" || e.Pod != "" {
		parts = append(parts, kindStyle.Render(strings.TrimSpace(prefixed("app ", e.App)+" "+prefixed("pod ", e.Pod))))
	}
	return ansi.Truncate(" "+strings.Join(parts, dimStyle.Render("  ·  ")), w, "…")
}

func subParen(st unitState) string {
	if st.sub == "" || st.sub == "-" || st.sub == st.word {
		return ""
	}
	return " (" + st.sub + ")"
}

func prefixed(p, v string) string {
	if v == "" {
		return ""
	}
	return p + v
}

// overviewContent is the Overview tab: everything quadman knows about the
// unit on one page.
func (m Model) overviewContent(u quadlet.Unit) string {
	st := m.stateOf(u)
	status := m.status[u.UnitName]
	var lines []string
	row := func(k, v string) { lines = append(lines, dimStyle.Render(padCell(k, 11))+v) }
	state := glyphStyle(st.glyph).Render(glyph(st.glyph) + " " + st.word + subParen(st))
	if up := uptimeOf(status, time.Now()); up != "—" && st.glyph != gInactive && st.glyph != gFailed {
		state += mutedStyle.Render(" · up " + up)
	}
	if status.NRestarts > 0 {
		state += warnStyle.Render(fmt.Sprintf(" · ↻ %d restarts", status.NRestarts))
	}
	row("STATE", state)
	if status.Result != "" && status.Result != "success" {
		row("RESULT", errStyle.Render(fmt.Sprintf("%s (status %d)", status.Result, status.ExecMainStatus)))
	}
	row("UNIT", fgStyle.Render(u.UnitName)+mutedStyle.Render(" · "+string(u.Kind)))
	f, ferr := m.parseUnitFile(u)
	boot := mutedStyle.Render("not started at boot · e enables")
	if ferr == nil && f.Section("Install").Get("WantedBy") != "" {
		boot = okStyle.Render("starts at boot") + mutedStyle.Render(" (WantedBy="+f.Section("Install").Get("WantedBy")+")")
	}
	row("FILE", fgStyle.Render(u.Path))
	row("BOOT", boot)
	if img := m.images[u.UnitName]; img != "" {
		v := fgStyle.Render(img)
		if ferr == nil {
			if pol := f.Section(u.Kind.Section()).Get("AutoUpdate"); pol != "" {
				v += mutedStyle.Render(" · AutoUpdate=" + pol)
			}
		}
		row("IMAGE", v)
	}
	if e := m.podmanInfo[u.UnitName]; e.App != "" || e.Pod != "" {
		row("APP / POD", kindStyle.Render(strings.TrimSpace(prefixed("app ", e.App)+"  "+prefixed("pod ", e.Pod))))
	}
	if ferr == nil {
		sec := f.Section(u.Kind.Section())
		if ports := sec.GetAll("PublishPort"); len(ports) > 0 {
			row("PORTS", fgStyle.Render(strings.Join(ports, ", ")))
		}
		if vols := sec.GetAll("Volume"); len(vols) > 0 {
			row("VOLUMES", fgStyle.Render(strings.Join(vols, ", ")))
		}
		var secrets []string
		for _, s := range f.Secrets() {
			as := s.Type
			if s.Target != "" {
				as += " " + s.Target
			}
			secrets = append(secrets, s.Name+mutedStyle.Render(" ("+as+")"))
		}
		if len(secrets) > 0 {
			row("SECRETS", fgStyle.Render(strings.Join(secrets, ", ")))
		}
	}
	tree := m.computeTree()
	var deps []string
	for _, d := range tree.deps[u.Name] {
		text := d.kind + " " + d.label
		switch {
		case d.missing:
			text += " " + errStyle.Render(glyph(gFailed)+" missing")
		case d.target != "":
			if i, ok := m.unitByName(d.target); ok {
				ts := m.stateOf(m.units[i])
				text += " " + glyphStyle(ts.glyph).Render(glyph(ts.glyph))
			}
		}
		deps = append(deps, text)
	}
	if len(deps) > 0 {
		row("DEPENDS ON", fgStyle.Render(strings.Join(deps, " · ")))
	}
	var needed []string
	for other, ds := range tree.deps {
		for _, d := range ds {
			if d.target == u.Name {
				needed = append(needed, other)
				break
			}
		}
	}
	slices.Sort(needed)
	if len(needed) > 0 {
		row("NEEDED BY", fgStyle.Render(strings.Join(needed, ", ")))
	}
	for i, is := range m.issues[u.Name] {
		g := gWarn
		if is.Severity == quadlet.SeverityError {
			g = gFailed
		}
		k := ""
		if i == 0 {
			k = "PROBLEMS"
		}
		row(k, glyphStyle(g).Render(glyph(g)+" ")+fgStyle.Render(is.Message))
	}
	lines = append(lines, "", keyHint("l", "journal")+"   "+keyHint("E", "edit")+"   "+keyHint("s/x/r", "start · stop · restart")+"   "+keyHint("[ ]", "tabs"))
	return strings.Join(lines, "\n")
}
