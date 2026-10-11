package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Tree screen (t): units grouped by pod and by podman app, each with the
// things it references (pod, image, volumes, networks, [Unit] deps).
// Missing references are marked, healthy subtrees start folded, and the
// impact line says what blocks the selected unit and what needs it.

// treeDep is one reference from a unit's file.
type treeDep struct {
	kind    string // pod, img, vol, net, dep
	label   string // what the file says, e.g. "pgdata.volume", "After=db.service"
	target  string // quadlet base name it resolves to ("" for plain refs)
	missing bool   // a quadlet reference with no such unit
}

type treeViewState struct {
	deps         map[string][]treeDep // by unit base name
	podOf        map[string]string    // container base name -> pod base name
	open         map[string]bool      // explicit fold state by node id
	problemsOnly bool
	impact       bool
}

// computeTree parses every unit file once (screen open and refresh).
func (m Model) computeTree() treeViewState {
	t := treeViewState{deps: map[string][]treeDep{}, podOf: map[string]string{}, open: m.treeView.open,
		problemsOnly: m.treeView.problemsOnly, impact: m.treeView.impact}
	if t.open == nil {
		t.open = map[string]bool{}
	}
	names := map[string]bool{}
	byUnit := map[string]string{}
	for _, u := range m.units {
		names[u.Name] = true
		byUnit[u.UnitName] = u.Name
	}
	ref := func(kind, label string, quadletExt bool) treeDep {
		d := treeDep{kind: kind, label: label}
		if quadletExt {
			d.target = stripKnownExt(label)
			d.missing = !names[d.target]
		}
		return d
	}
	for _, u := range m.units {
		f, err := m.parseUnitFile(u)
		if err != nil {
			continue
		}
		sec := f.Section(string(u.Kind))
		var deps []treeDep
		if pod := sec.Get("Pod"); pod != "" {
			deps = append(deps, ref("pod", pod, true))
			t.podOf[u.Name] = stripKnownExt(pod)
		}
		if img := sec.Get("Image"); img != "" {
			deps = append(deps, ref("img", img, stripKnownExt(img) != img))
		}
		for _, n := range sec.GetAll("Network") {
			deps = append(deps, ref("net", n, stripKnownExt(n) != n))
		}
		for _, v := range sec.GetAll("Volume") {
			src, _, _ := strings.Cut(v, ":")
			if stripKnownExt(src) != src {
				deps = append(deps, ref("vol", src, true))
			}
		}
		for _, key := range []string{"Requires", "BindsTo", "Wants", "After", "PartOf"} {
			for _, val := range f.Section("Unit").GetAll(key) {
				for _, r := range strings.Fields(val) {
					if base, ok := byUnit[r]; ok {
						deps = append(deps, treeDep{kind: "dep", label: key + "=" + r, target: base})
					}
				}
			}
		}
		t.deps[u.Name] = deps
	}
	return t
}

func kindAbbrev(u quadlet.Unit) string {
	if strings.HasSuffix(u.Name, "@") {
		return "tpl"
	}
	switch u.Kind {
	case quadlet.KindContainer:
		return "ctr"
	case quadlet.KindVolume:
		return "vol"
	case quadlet.KindNetwork:
		return "net"
	case quadlet.KindImage:
		return "img"
	case quadlet.KindBuild:
		return "bld"
	}
	return string(u.Kind)
}

// treeProblem reports units the problems-only filter keeps.
func (m Model) treeProblem(u quadlet.Unit) bool {
	switch m.stateOf(u).glyph {
	case gFailed, gUnhealthy, gStarting, gNotFound:
		return true
	}
	if len(m.issues[u.Name]) > 0 {
		return true
	}
	for _, d := range m.treeView.deps[u.Name] {
		if d.missing {
			return true
		}
	}
	return false
}

// nodeOpen resolves a node's fold state: explicit toggles win; groups
// start open, units start open only when something is wrong.
func (m Model) nodeOpen(id string, problem bool) bool {
	if v, ok := m.treeView.open[id]; ok {
		return v
	}
	return strings.HasPrefix(id, "grp:") || problem
}

type treeGroup struct {
	id, label string
	head      *quadlet.Unit // the pod unit for pod groups
	members   []quadlet.Unit
}

func (m Model) treeGroups() []treeGroup {
	units := m.sortUnits(m.units)
	var groups []treeGroup
	placed := map[string]bool{}
	for _, u := range units {
		if u.Kind != quadlet.KindPod {
			continue
		}
		g := treeGroup{id: "grp:pod:" + u.Name, label: "pod " + u.Name, head: &u}
		placed[u.Name] = true
		for _, c := range units {
			if m.treeView.podOf[c.Name] == u.Name {
				g.members = append(g.members, c)
				placed[c.Name] = true
			}
		}
		groups = append(groups, g)
	}
	apps := map[string]*treeGroup{}
	var appOrder []string
	for _, u := range units {
		app := m.podmanInfo[u.UnitName].App
		if placed[u.Name] || app == "" {
			continue
		}
		if apps[app] == nil {
			apps[app] = &treeGroup{id: "grp:app:" + app, label: "app " + app}
			appOrder = append(appOrder, app)
		}
		apps[app].members = append(apps[app].members, u)
		placed[u.Name] = true
	}
	for _, a := range appOrder {
		groups = append(groups, *apps[a])
	}
	rest := treeGroup{id: "grp:none", label: "ungrouped"}
	for _, u := range units {
		if !placed[u.Name] {
			rest.members = append(rest.members, u)
		}
	}
	if len(rest.members) > 0 {
		groups = append(groups, rest)
	}
	return groups
}

// groupState is the worst member state, "degraded" when mixed.
func (m Model) groupState(g treeGroup) (glyphKind, string) {
	if g.head != nil {
		st := m.stateOf(*g.head)
		return st.glyph, st.word
	}
	worst, n := gInactive, 0
	all := map[glyphKind]bool{}
	for _, u := range g.members {
		st := m.stateOf(u).glyph
		all[st] = true
		if stateRank(st) < stateRank(worst) {
			worst = st
		}
		n++
	}
	switch {
	case n == 0:
		return gInactive, "empty"
	case len(all) == 1 && worst == gActive:
		return gActive, "active"
	case worst == gFailed || worst == gUnhealthy:
		return gUnhealthy, "degraded"
	case worst == gStarting:
		return gStarting, "starting"
	}
	return worst, m.stateWord(worst)
}

func (m Model) stateWord(g glyphKind) string {
	return map[glyphKind]string{gActive: "active", gInactive: "inactive", gStarting: "starting",
		gFailed: "failed", gUnhealthy: "unhealthy", gNotFound: "not-found", gTemplate: "template"}[g]
}

func (m Model) treeSpec() screenSpec {
	var rows []scrRow
	units, groupsN := 0, 0
	for _, g := range m.treeGroups() {
		members := g.members
		if m.treeView.problemsOnly {
			members = slices.DeleteFunc(slices.Clone(members), func(u quadlet.Unit) bool { return !m.treeProblem(u) })
			if len(members) == 0 && (g.head == nil || !m.treeProblem(*g.head)) {
				continue
			}
		}
		groupsN++
		gOpen := m.nodeOpen(g.id, false)
		gg, gw := m.groupState(g)
		fold := "▾"
		if !gOpen {
			fold = "▸"
		}
		state := glyph(gg) + " " + gw
		if !gOpen {
			state = fmt.Sprintf("%s +%d", glyph(gg), len(members))
		}
		head := scrRow{id: g.id, cells: []scrCell{cell(fold+" "+g.label, brightStyle), cell(state, glyphStyle(gg))}}
		if g.head != nil {
			head.unit = g.head.UnitName
		}
		rows = append(rows, head)
		if !gOpen {
			units += len(members)
			continue
		}
		for i, u := range members {
			units++
			last := i == len(members)-1
			rows = append(rows, m.treeUnitRows(u, last)...)
		}
	}
	impact := ""
	if m.treeView.impact {
		// The selection comes from the rows built here: asking
		// selectedRow would rebuild this spec and recurse forever.
		if r, ok := m.selectedRowIn(m.visibleRows(rows)); ok {
			impact = m.impactLine(r)
		}
	}
	toolbar := " " + dimStyle.Render(fmt.Sprintf("%d groups · %d units", groupsN, units))
	if m.treeView.problemsOnly {
		toolbar += "   " + warnStyle.Render("problems only") + " " + keyHint("f", "show all")
	}
	if impact != "" {
		toolbar = impact
	}
	return screenSpec{
		toolbar: toolbar,
		cols: []scrCol{
			{title: "UNIT", min: 24},
			{title: "STATE", w: 16},
		},
		rows:  rows,
		empty: "no quadlet units",
		info:  fmt.Sprintf("%d groups · %d units", groupsN, units),
	}
}

func (m Model) treeUnitRows(u quadlet.Unit, last bool) []scrRow {
	deps := m.treeView.deps[u.Name]
	id := "unit:" + u.Name
	open := len(deps) > 0 && m.nodeOpen(id, m.treeProblem(u))
	branch, stem := "├─", "│ "
	if last {
		branch, stem = "└─", "  "
	}
	fold := "─"
	switch {
	case len(deps) > 0 && open:
		fold = "▾"
	case len(deps) > 0:
		fold = "▸"
	}
	st := m.stateOf(u)
	state := glyph(st.glyph) + " " + st.word
	if len(deps) > 0 && !open {
		state = fmt.Sprintf("%s +%d", glyph(st.glyph), len(deps))
	}
	name := fmt.Sprintf("  %s%s %s %s", branch, fold, kindAbbrev(u), u.Name) + m.issues.marker(u.Name)
	rows := []scrRow{{id: id, unit: u.UnitName, cells: []scrCell{cell(name, fgStyle), cell(state, glyphStyle(st.glyph))}, search: u.Name}}
	if !open {
		return rows
	}
	for i, d := range deps {
		b := "├──"
		if i == len(deps)-1 {
			b = "└──"
		}
		text := fmt.Sprintf("  %s   %s %s %s", stem, b, d.kind, d.label)
		dstate, dst := "", mutedStyle
		switch {
		case d.missing:
			dstate, dst = glyph(gFailed)+" missing", errStyle
		case d.target != "":
			if j, ok := m.unitByName(d.target); ok {
				ts := m.stateOf(m.units[j])
				dstate, dst = glyph(ts.glyph)+" "+ts.word, glyphStyle(ts.glyph)
			}
		case d.kind == "img" && m.imagePending(u.UnitName):
			dstate, dst = "↑ update pending", accentStyle
		}
		unit := ""
		if j, ok := m.unitByName(d.target); ok {
			unit = m.units[j].UnitName
		}
		rows = append(rows, scrRow{id: fmt.Sprintf("dep:%s:%d", u.Name, i), unit: unit,
			cells: []scrCell{cell(text, mutedStyle), cell(dstate, dst)}, search: u.Name + " " + d.label})
	}
	return rows
}

func (m Model) imagePending(unitName string) bool {
	for _, e := range m.updateEntries {
		if e.Unit == unitName && e.Updated == "pending" {
			return true
		}
	}
	return false
}

// impactLine: "impact web: blocked by pgdata2.volume (missing) · needed by api, worker".
func (m Model) impactLine(r scrRow) string {
	name := ""
	switch {
	case strings.HasPrefix(r.id, "unit:"):
		name = strings.TrimPrefix(r.id, "unit:")
	case strings.HasPrefix(r.id, "dep:"):
		name, _, _ = strings.Cut(strings.TrimPrefix(r.id, "dep:"), ":")
	case strings.HasPrefix(r.id, "grp:pod:"):
		name = strings.TrimPrefix(r.id, "grp:pod:")
	default:
		return " " + dimStyle.Render("impact: select a unit")
	}
	var blocked []string
	for _, d := range m.treeView.deps[name] {
		switch {
		case d.missing:
			blocked = append(blocked, d.label+" (missing)")
		case d.target != "":
			if j, ok := m.unitByName(d.target); ok {
				if g := m.stateOf(m.units[j]).glyph; g == gFailed || g == gNotFound {
					blocked = append(blocked, d.target+" ("+m.stateWord(g)+")")
				}
			}
		}
	}
	var needed []string
	for other, deps := range m.treeView.deps {
		for _, d := range deps {
			if d.target == name {
				needed = append(needed, other)
				break
			}
		}
	}
	slices.Sort(needed)
	b := "nothing"
	if len(blocked) > 0 {
		b = strings.Join(blocked, ", ")
	}
	n := "nothing"
	if len(needed) > 0 {
		n = strings.Join(needed, ", ")
	}
	st := mutedStyle
	if len(blocked) > 0 {
		st = errStyle
	}
	return " " + accentStyle.Render("impact "+name+": ") + st.Render("blocked by "+b) + dimStyle.Render(" · ") + fgStyle.Render("needed by "+n)
}

// treeKeysIn handles the tree screen's own keys.
func (m Model) treeKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	r, hasRow := m.selectedRow()
	setOpen := func(open bool) {
		if hasRow && (strings.HasPrefix(r.id, "grp:") || strings.HasPrefix(r.id, "unit:")) {
			if m.treeView.open == nil {
				m.treeView.open = map[string]bool{}
			}
			m.treeView.open[r.id] = open
		}
	}
	switch msg.String() {
	case "right", "l":
		setOpen(true)
		return m, nil, true
	case "left", "h":
		setOpen(false)
		return m, nil, true
	case "z":
		anyClosed := false
		for _, g := range m.treeGroups() {
			if !m.nodeOpen(g.id, false) {
				anyClosed = true
			}
			for _, u := range g.members {
				if len(m.treeView.deps[u.Name]) > 0 && !m.nodeOpen("unit:"+u.Name, m.treeProblem(u)) {
					anyClosed = true
				}
			}
		}
		open := map[string]bool{}
		for _, g := range m.treeGroups() {
			open[g.id] = anyClosed
			for _, u := range g.members {
				open["unit:"+u.Name] = anyClosed
			}
		}
		m.treeView.open = open
		m.scrCur, m.scrOff = 0, 0
		return m, nil, true
	case "f":
		m.treeView.problemsOnly = !m.treeView.problemsOnly
		m.scrCur, m.scrOff = 0, 0
		return m, nil, true
	case "i":
		m.treeView.impact = !m.treeView.impact
		return m, nil, true
	}
	return m, nil, false
}
