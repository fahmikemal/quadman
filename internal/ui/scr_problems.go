package ui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Problems screen (v): every finding — the generator's own dry-run errors
// and warnings plus hints derived from live state — with the offending
// line quoted from the file and the steps that fix it.

// problem is one finding, located in its file when possible.
type problem struct {
	sev     glyphKind // gFailed or gWarn
	unit    string    // quadlet base name
	file    string    // file base name
	path    string
	line    int // 1-based, 0 = unknown
	col     int // 0-based start of the offending token on that line
	span    int // token width for the ^^^ marker
	message string
	fixes   []string
	excerpt []string // the lines around line, quoted at compute time
}

var quotedToken = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)

// problems returns the cached findings; computeProblems refreshes them
// when the screen opens and after every refresh while it is open, so a
// render never reads files (over SSH that would be a cat per frame).
func (m Model) problems() []problem { return m.probList }

// computeProblems rebuilds the findings from issues, hints and files.
func (m Model) computeProblems() []problem {
	byName := map[string]quadlet.Unit{}
	for _, u := range m.units {
		byName[u.Name] = u
	}
	var out []problem
	names := make([]string, 0, len(m.issues))
	for name := range m.issues {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		u := byName[name]
		for _, is := range m.issues[name] {
			p := problem{sev: gWarn, unit: name, file: is.File, path: u.Path, message: is.Message}
			if is.Severity == quadlet.SeverityError {
				p.sev = gFailed
			}
			if p.path == "" {
				p.path = is.Path
			}
			p.line, p.col, p.span = m.locate(u, is.Message)
			p.fixes = m.fixesFor(is, u)
			if p.line > 0 {
				p.excerpt = m.excerpt(p)
			}
			out = append(out, p)
		}
	}
	for _, h := range m.smartHints() {
		unitName, msg, _ := strings.Cut(h, ": ")
		name := strings.TrimSuffix(unitName, ".service")
		u := byName[name]
		if i, ok := m.unitByName(unitName); ok {
			u = m.units[i]
			name = u.Name
		}
		short, fix, _ := strings.Cut(msg, " — ")
		p := problem{sev: gWarn, unit: name, file: baseName(u.Path), path: u.Path, message: short}
		if fix != "" {
			p.fixes = []string{fix}
		}
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b problem) int { return stateRank(a.sev) - stateRank(b.sev) })
	return out
}

// locate finds the line a message is about: a quoted key ('Foo' →
// "Foo="), else the first line containing any quoted token.
func (m Model) locate(u quadlet.Unit, msg string) (int, int, int) {
	if u.Path == "" {
		return 0, 0, 0
	}
	data, err := m.readUnitFile(u)
	if err != nil {
		return 0, 0, 0
	}
	lines := strings.Split(string(data), "\n")
	var tokens []string
	for _, mt := range quotedToken.FindAllStringSubmatch(msg, -1) {
		tok := mt[1] + mt[2]
		if tok != "" && tok != u.Name && !strings.HasSuffix(tok, "."+string(u.Kind)) {
			tokens = append(tokens, tok)
		}
	}
	for _, tok := range tokens {
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), tok+"=") {
				return i + 1, strings.Index(l, tok), len(tok)
			}
		}
	}
	for _, tok := range tokens {
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			if c := strings.Index(l, tok); c >= 0 {
				return i + 1, c, len(tok)
			}
		}
	}
	return 0, 0, 0
}

// fixesFor turns a generator finding into concrete next steps.
func (m Model) fixesFor(is quadlet.Issue, u quadlet.Unit) []string {
	var fixes []string
	if h := m.gateHint(is); h != "" {
		fixes = append(fixes, h+" — upgrade Podman or drop the key")
	}
	msg := is.Message
	if mt := regexp.MustCompile(`unsupported key '([^']+)' in group '([^']+)'`).FindStringSubmatch(msg); mt != nil {
		fixes = append(fixes, fmt.Sprintf("%s= is not valid in [%s]: remove it, or move it to the section it belongs to", mt[1], mt[2]))
	}
	for _, mt := range quotedToken.FindAllStringSubmatch(msg, -1) {
		ref := mt[1] + mt[2]
		base := stripKnownExt(ref)
		if base == ref || base == u.Name {
			continue
		}
		if _, ok := m.unitByName(base); ok {
			continue
		}
		if near := m.nearestUnit(base); near != "" {
			fixes = append(fixes, fmt.Sprintf("did you mean %s? (%s)", near+ref[len(base):], m.unitGlyph(near)))
		}
		fixes = append(fixes, "or create "+ref+" next to "+baseName(u.Path))
	}
	fixes = append(fixes, "E opens the file at the line · R reloads the generator once it is fixed")
	return fixes
}

func stripKnownExt(s string) string {
	for _, ext := range []string{".container", ".pod", ".kube", ".volume", ".network", ".image", ".build", ".artifact"} {
		if strings.HasSuffix(s, ext) {
			return strings.TrimSuffix(s, ext)
		}
	}
	return s
}

// nearestUnit returns the unit name within edit distance 3 of name.
func (m Model) nearestUnit(name string) string {
	best, bestD := "", 4
	for _, u := range m.units {
		if d := editDistance(name, u.Name); d < bestD {
			best, bestD = u.Name, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func (m Model) problemsSpec() screenSpec {
	ps := m.problems()
	errs, warns := 0, 0
	affected := map[string]bool{}
	for _, p := range ps {
		if p.sev == gFailed {
			errs++
		} else {
			warns++
		}
		affected[p.unit] = true
	}
	okN := 0
	for _, u := range m.units {
		if !affected[u.Name] {
			okN++
		}
	}
	gen := "podman-system-generator"
	if m.generator == "" {
		gen = "not found"
	}
	tiles := []tile{
		{label: "ERRORS", value: fmt.Sprintf("%s %d", glyph(gFailed), errs), note: "unit won't load", valueStyle: errStyle},
		{label: "WARNINGS", value: fmt.Sprintf("%s %d", glyph(gWarn), warns), note: "loads, but check it", valueStyle: warnStyle},
		{label: "OK", value: fmt.Sprintf("%s %d", glyph(gDone), okN), note: "units without findings", valueStyle: okStyle},
		{label: "GENERATOR", value: "dry-run", note: gen, valueStyle: fgStyle},
	}
	if errs > 0 {
		tiles[0].tint = activePalette.err
	} else {
		tiles[0].valueStyle = mutedStyle
	}
	if warns > 0 {
		tiles[1].tint = activePalette.warn
	} else {
		tiles[1].valueStyle = mutedStyle
	}
	s := screenSpec{
		tiles:   tiles,
		summary: fmt.Sprintf(" %s %d errors · %s %d warnings · %s %d ok", glyph(gFailed), errs, glyph(gWarn), warns, glyph(gDone), okN),
		cols: []scrCol{
			{title: "", w: 1},
			{title: "FILE", min: 14, max: 32},
			{title: "PROBLEM", min: 20},
		},
		empty:     "no findings — every Quadlet file converted cleanly",
		detail:    m.problemDetail,
		detailMax: 12,
		info:      "quadlet generator dry-run",
	}
	if m.podmanVersion != "" {
		s.info += " · podman " + m.podmanVersion
	}
	if m.generator == "" {
		s.empty = "podman-system-generator not found: validation unavailable"
	}
	for i, p := range ps {
		where := p.file
		if p.line > 0 {
			where += fmt.Sprintf(":%d", p.line)
		}
		unit := ""
		if j, ok := m.unitByName(p.unit); ok {
			unit = m.units[j].UnitName
		}
		s.rows = append(s.rows, scrRow{
			id:   fmt.Sprintf("prob:%d:%s", i, p.unit),
			unit: unit,
			cells: []scrCell{
				cell(glyph(p.sev), glyphStyle(p.sev)),
				cell(where, fgStyle),
				cell(p.message, fgStyle),
			},
		})
	}
	return s
}

func (m Model) problemAt(id string) (problem, bool) {
	var i int
	if _, err := fmt.Sscanf(id, "prob:%d:", &i); err != nil {
		return problem{}, false
	}
	ps := m.problems()
	if i < 0 || i >= len(ps) {
		return problem{}, false
	}
	return ps[i], true
}

func (m Model) problemDetail(r scrRow, width int) []string {
	p, ok := m.problemAt(r.id)
	if !ok {
		return nil
	}
	lines := []string{glyphStyle(p.sev).Render(glyph(p.sev)+" ") + brightStyle.Render(p.file) + mutedStyle.Render(lineSuffix(p.line))}
	lines = append(lines, fgStyle.Render(ansi.Truncate(p.message, width, "…")))
	lines = append(lines, p.excerpt...)
	if len(p.fixes) > 0 {
		lines = append(lines, dimStyle.Bold(true).Render("HOW TO FIX"))
		for i, f := range p.fixes {
			lines = append(lines, accentStyle.Render(fmt.Sprintf("%d  ", i+1))+fgStyle.Render(f))
		}
	}
	return lines
}

func lineSuffix(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("  line %d", n)
}

// excerpt quotes the lines around p.line with a gutter and a ^ marker
// under the offending token.
func (m Model) excerpt(p problem) []string {
	i, ok := m.unitByName(p.unit)
	if !ok {
		return nil
	}
	data, err := m.readUnitFile(m.units[i])
	if err != nil {
		return nil
	}
	src := strings.Split(string(data), "\n")
	var out []string
	for n := max(p.line-1, 1); n <= min(p.line+1, len(src)); n++ {
		gutter := dimStyle.Render(fmt.Sprintf("%4d │ ", n))
		text := src[n-1]
		if n == p.line {
			out = append(out, gutter+brightStyle.Render(text))
			if p.span > 0 {
				out = append(out, dimStyle.Render("     │ ")+errStyle.Render(strings.Repeat(" ", p.col)+strings.Repeat("^", p.span)))
			}
			continue
		}
		out = append(out, gutter+mutedStyle.Render(text))
	}
	return out
}

// problemsKeysIn handles the problems screen's own keys.
func (m Model) problemsKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "R":
		return m.reloadKeys(msg)
	case "E", "y":
		r, ok := m.selectedRow()
		if !ok {
			return m, nil, true
		}
		p, ok := m.problemAt(r.id)
		if !ok {
			return m, nil, true
		}
		if msg.String() == "y" {
			m.setStatus("copied the problem text", false)
			return m, tea.SetClipboard(p.file + ": " + p.message), true
		}
		if p.path == "" {
			m.setStatus("file of "+p.unit+" unknown", true)
			return m, nil, true
		}
		mm, cmd := m.editPathAt(p.path, p.line)
		return mm, cmd, true
	}
	return m, nil, false
}
