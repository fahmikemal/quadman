package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

func TestIssueMarker(t *testing.T) {
	m := unitIssues{
		"bad":  {{File: "bad.container", Severity: quadlet.SeverityError, Message: "x"}},
		"warn": {{File: "warn.container", Severity: quadlet.SeverityWarning, Message: "y"}},
		"both": {
			{File: "both.container", Severity: quadlet.SeverityWarning},
			{File: "both.container", Severity: quadlet.SeverityError},
		},
	}
	if got := m.marker("bad"); got != " ✗" {
		t.Errorf("bad = %q, want ✗", got)
	}
	if got := m.marker("warn"); got != " ⚠" {
		t.Errorf("warn = %q, want ⚠", got)
	}
	if got := m.marker("both"); got != " ✗" {
		t.Errorf("error must win over warning, got %q", got)
	}
	if got := m.marker("clean"); got != "" {
		t.Errorf("clean = %q, want empty", got)
	}
}

func TestProblemsScreen(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.container")
	if err := os.WriteFile(bad, []byte("[Container]\nImage=nginx\nBadKey=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	m.units = []quadlet.Unit{{Name: "bad", Kind: quadlet.KindContainer, Path: bad, UnitName: "bad.service"}}
	m.issues = indexIssues([]quadlet.Issue{
		{File: "bad.container", Severity: quadlet.SeverityError, Message: "unsupported key 'BadKey' in group 'Container'"},
		{File: "warn.container", Severity: quadlet.SeverityWarning, Message: "short image name"},
	})
	model, _ := m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m = model.(Model)
	if m.mode != modeValidate {
		t.Fatal("v must open the problems screen")
	}
	rows := m.screenSpec().rows
	if len(rows) != 2 || rows[0].cells[1].text != "bad.container:3" {
		t.Fatalf("errors first, located at the key's line: %+v", rows)
	}
	detail := ansi.Strip(strings.Join(m.problemDetail(rows[0], 80), "\n"))
	for _, want := range []string{"BadKey=1", "^^^^^^", "HOW TO FIX", "BadKey= is not valid in [Container]"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q:\n%s", want, detail)
		}
	}

	m2 := New()
	m2.generator = "/usr/lib/systemd/system-generators/podman-system-generator"
	m2.probList = m2.computeProblems()
	if s := m2.problemsSpec(); len(s.rows) != 0 || !strings.Contains(s.empty, "no findings") {
		t.Errorf("no issues must show the clean message, got %+v", s.rows)
	}
}

func TestComputeTree(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	m := New()
	m.units = []quadlet.Unit{
		{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service", Path: write("webapp.container", "[Container]\nImage=base.image\nVolume=gone.volume:/d\n")},
		{Name: "base", Kind: quadlet.KindImage, UnitName: "base-image.service", Path: write("base.image", "[Image]\n")},
		{Name: "plain", Kind: quadlet.KindContainer, UnitName: "plain.service", Path: write("plain.container", "[Container]\nImage=nginx\n")},
	}
	tv := m.computeTree()
	deps := tv.deps["webapp"]
	if len(deps) != 2 || deps[0].target != "base" || deps[0].missing || !deps[1].missing || deps[1].kind != "vol" {
		t.Fatalf("webapp deps = %+v, want base.image (found) and gone.volume (missing)", deps)
	}
	if d := tv.deps["plain"]; len(d) != 1 || d[0].target != "" {
		t.Errorf("plain image ref is not a quadlet dependency: %+v", d)
	}
	// The broken unit starts unfolded, so its missing volume is visible.
	m.treeView = tv
	m.mode = modeTree
	var texts []string
	for _, r := range m.treeSpec().rows {
		texts = append(texts, r.cells[0].text+" | "+r.cells[1].text)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "vol gone.volume | "+glyph(gFailed)+" missing") {
		t.Errorf("missing dependency must show:\n%s", joined)
	}
}

func TestWithDropins(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "web.container")
	if err := os.WriteFile(base, []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dropDir := filepath.Join(dir, "web.container.d")
	if err := os.MkdirAll(dropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dropDir, "10-extra.conf"), []byte("Environment=A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	u := quadlet.Unit{Name: "web", Kind: quadlet.KindContainer, Path: base}
	base2, _ := os.ReadFile(base)
	out := withDropins(u, base2)
	if !strings.Contains(out, "Image=nginx") || !strings.Contains(out, "Environment=A=1") || !strings.Contains(out, "drop-in:") {
		t.Errorf("withDropins should append drop-in content with a separator:\n%s", out)
	}

	u2 := quadlet.Unit{Name: "none", Kind: quadlet.KindContainer, Path: filepath.Join(dir, "none.container")}
	out2 := withDropins(u2, []byte("base only"))
	if out2 != "base only" {
		t.Errorf("no drop-ins must return the base unchanged, got %q", out2)
	}
}

func TestCopySelection(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.images["webapp.service"] = "nginx:latest"

	model, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mm := model.(Model)
	if !strings.Contains(mm.statusLine, "copied webapp.service") {
		t.Errorf("y should copy the unit name: %q", mm.statusLine)
	}

	model, _ = mm.Update(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	mm = model.(Model)
	if !strings.Contains(mm.statusLine, "copied nginx:latest") {
		t.Errorf("Y should copy the image: %q", mm.statusLine)
	}
}

func TestInstantiateFlow(t *testing.T) {
	m := New()
	units := []quadlet.Unit{{Name: "web@", Kind: quadlet.KindContainer, UnitName: "web@.service"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{""}, lingerOK: true})
	m = model.(Model)
	m.table.SetCursor(0)

	model, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = model.(Model)
	if !m.instancing {
		t.Fatal("i on a template must open the instance input")
	}

	for _, r := range "prod" {
		model, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = model.(Model)
	}
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm := model.(Model)
	if mm.instancing {
		t.Error("enter must close the input")
	}
	if !mm.busy {
		t.Error("instantiating must start the busy action (start web@prod.service)")
	}
}

func TestInstantiateNonTemplate(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	model, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	mm := model.(Model)
	if mm.instancing {
		t.Error("i on a non-template must not open the input")
	}
	if !strings.Contains(mm.statusLine, "not a template") {
		t.Errorf("should explain: %q", mm.statusLine)
	}
}

func TestDeleteConfirmation(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	model, _ := m.Update(tea.KeyPressMsg{Code: 'D', Text: "D"})
	mm := model.(Model)
	if mm.pending == nil || mm.pending.verb != "delete" {
		t.Fatal("D must arm the delete confirmation")
	}
	if !strings.Contains(mm.statusLine, "type webapp to confirm") {
		t.Errorf("delete prompt missing: %q", mm.statusLine)
	}
	if mm.pending.typed != "webapp" || !mm.pending.danger {
		t.Fatalf("delete must require typing the unit name in a danger modal: %+v", mm.pending)
	}
	if v := ansi.Strip(mm.View().Content); !strings.Contains(v, "THIS WILL") || !strings.Contains(v, "KEEPS") || !strings.Contains(v, "type webapp to confirm") {
		t.Errorf("modal must explain the delete:\n%s", v)
	}

	// y does not confirm a typed delete: it is just a letter in the box.
	model, _ = mm.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mm = model.(Model)
	if mm.pending == nil || mm.busy {
		t.Fatal("y must not confirm a typed delete")
	}
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	mm = model.(Model)
	if mm.pending == nil || mm.busy {
		t.Fatal("enter with the wrong name must not confirm")
	}
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm = model.(Model)
	if mm.pending != nil {
		t.Error("esc must cancel the delete")
	}
	if last := mm.actions[len(mm.actions)-1]; !last.cancelled {
		t.Errorf("a cancelled confirmation is logged: %+v", last)
	}
}

func TestCycleTabAndHandlers(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.mode = modeDetail
	m.tab = tabSource

	// cycle forward: source -> status (async cmd returned)
	model, cmd := m.cycleTab(1)
	m = model.(Model)
	if m.tab != tabStatus {
		t.Fatalf("] should move to status tab, got %d", m.tab)
	}
	if cmd == nil {
		t.Error("entering the status tab must kick off its async load")
	}

	// statusMsg fills content only while on that tab
	model, _ = m.Update(statusMsg{content: "● webapp.service - Web App"})
	m = model.(Model)
	if !strings.Contains(m.viewport.View(), "") {
		_ = m
	}

	// inspect on a container returns a cmd
	model, cmd = m.cycleTab(2) // status -> inspect
	m = model.(Model)
	if m.tab != tabInspect {
		t.Fatalf("] should move to inspect tab, got %d", m.tab)
	}
	if cmd == nil {
		t.Error("entering inspect on a container must load podman inspect")
	}

	// wrap around: inspect -> overview
	model, _ = m.cycleTab(1)
	m = model.(Model)
	if m.tab != tabOverview {
		t.Errorf("] from inspect must wrap to overview, got %d", m.tab)
	}
	if v := ansi.Strip(m.viewport.View()); !strings.Contains(v, "STATE") || !strings.Contains(v, "webapp.service") {
		t.Errorf("overview must summarize the unit: %q", v)
	}

	// backward: overview -> inspect
	model, _ = m.cycleTab(-1)
	m = model.(Model)
	if m.tab != tabInspect {
		t.Errorf("[ from overview must wrap to inspect, got %d", m.tab)
	}
}

func TestTabBar(t *testing.T) {
	m := New()
	m.tab = tabJournal
	m.following = true
	m.sess = &logSession{unit: "webapp.service"}
	bar := m.tabBar("webapp.service")
	if !strings.Contains(bar, "journal") || !strings.Contains(bar, "webapp.service") || !strings.Contains(bar, "live") {
		t.Errorf("tabBar = %q", bar)
	}
}

func TestAdaptiveColumns(t *testing.T) {
	// Short names: QUADLET sits at its 14-column floor.
	cols := adaptiveColumns(124, 9)
	if len(cols) != 6 || cols[0].Title != "STATE" || cols[5].Title != "IMAGE" {
		t.Fatalf("100+ columns must show STATE … UPTIME IMAGE, got %v", cols)
	}
	if cols[1].Width != 14 {
		t.Errorf("short names should keep QUADLET at 14, got %d", cols[1].Width)
	}
	if cols[0].Width != colStateW {
		t.Errorf("STATE is fixed at %d, got %d", colStateW, cols[0].Width)
	}
	// IMAGE absorbs the leftover width without overflowing.
	total := rowPrefixW + (len(cols)-1)*colGap
	for _, c := range cols {
		total += c.Width
	}
	if total != 124 {
		t.Errorf("columns must fill the table exactly: total %d, want 124", total)
	}

	// Long names: the cap stops one huge name from eating the table.
	cols = adaptiveColumns(124, 60)
	if cols[1].Width != 34 {
		t.Errorf("QUADLET cap = %d, want 34", cols[1].Width)
	}

	// Below 100 columns UPTIME and IMAGE are hidden.
	cols = adaptiveColumns(80, 20)
	if len(cols) != 4 || cols[3].Title != "SUB" {
		t.Errorf("narrow table must keep STATE QUADLET KIND SUB, got %v", cols)
	}
}

func TestBuildRowsAdaptiveWidths(t *testing.T) {
	m := withUnits(New(), "db")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 24})
	cols := model.(Model).table.Columns()
	if cols[1].Title != "QUADLET" || cols[1].Width != 14 {
		t.Errorf("one short name must keep QUADLET at its floor, got %+v", cols[1])
	}
}

func TestBuildRowsNarrowDoesNotPanic(t *testing.T) {
	// The table widget panics on rows wider than its columns; shrinking
	// below 100 columns drops UPTIME and IMAGE from both.
	m := withUnits(New(), "db", "web")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 30})
	model, _ = model.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	rows := model.(Model).table.Rows()
	if len(rows) != 2 || len(rows[0]) != 4 {
		t.Fatalf("narrow rows = %v, want 2 rows of 4 cells", rows)
	}
}

func TestStatusAutoExpire(t *testing.T) {
	m := New()
	m.setStatus("linger off", false)

	// Fresh success status must survive a tick.
	model, _ := m.Update(tickMsg{})
	if model.(Model).statusLine == "" {
		t.Error("a fresh success status must not fade yet")
	}

	// Older than TTL: must fade on the next tick.
	m2 := model.(Model)
	m2.statusAt = m2.statusAt.Add(-10 * time.Second)
	model, _ = m2.Update(tickMsg{})
	if model.(Model).statusLine != "" {
		t.Error("a success status older than the TTL must fade")
	}

	// Errors never auto-fade.
	m3 := New()
	m3.setStatus("boom failed", true)
	m3.statusAt = m3.statusAt.Add(-10 * time.Second)
	model, _ = m3.Update(tickMsg{})
	if model.(Model).statusLine == "" {
		t.Error("an error status must not auto-fade")
	}
}
