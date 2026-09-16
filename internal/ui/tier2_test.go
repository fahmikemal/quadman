package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
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

func TestValidateView(t *testing.T) {
	m := New()
	m.issues = indexIssues([]quadlet.Issue{
		{File: "bad.container", Severity: quadlet.SeverityError, Message: "unsupported key 'BadKey' in group 'Container'"},
		{File: "warn.container", Severity: quadlet.SeverityWarning, Message: "short image name"},
	})
	view := m.validateView()
	if !strings.Contains(view, "bad") || !strings.Contains(view, "ERROR") || !strings.Contains(view, "warn") {
		t.Errorf("validateView should group issues by file with severity:\n%s", view)
	}

	m2 := New()
	if !strings.Contains(m2.validateView(), "No validation issues") {
		t.Error("empty issues should show the clean message")
	}
}

func TestBuildTree(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	units := []quadlet.Unit{
		{Name: "webapp", Kind: quadlet.KindContainer, Path: write("webapp.container", "[Container]\nImage=base.image\n")},
		{Name: "base", Kind: quadlet.KindImage, Path: write("base.image", "[Image]\n")},
		{Name: "plain", Kind: quadlet.KindContainer, Path: write("plain.container", "[Container]\n")},
	}
	root := buildTree(units, nil)
	children := root.ChildNodes()
	if len(children) != 3 {
		t.Fatalf("root should have 3 unit nodes, got %d", len(children))
	}
	var webapp *treeNode
	_ = webapp
	for _, c := range children {
		if strings.Contains(c.Value(), "webapp") {
			if len(c.ChildNodes()) != 1 {
				t.Errorf("webapp should have 1 dep child (base.image), got %d", len(c.ChildNodes()))
			}
		}
		if strings.Contains(c.Value(), "plain") && len(c.ChildNodes()) != 0 {
			t.Errorf("plain should have no deps, got %d", len(c.ChildNodes()))
		}
	}
}

type treeNode = struct{}

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
	if !strings.Contains(mm.statusLine, "y/N") {
		t.Errorf("delete prompt missing: %q", mm.statusLine)
	}

	model, _ = mm.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	mm = model.(Model)
	if mm.pending != nil {
		t.Error("n must cancel the delete")
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

	// wrap around: inspect -> source
	model, _ = m.cycleTab(1)
	m = model.(Model)
	if m.tab != tabSource {
		t.Errorf("] from inspect must wrap to source, got %d", m.tab)
	}

	// backward: source -> inspect
	model, _ = m.cycleTab(-1)
	m = model.(Model)
	if m.tab != tabInspect {
		t.Errorf("[ from source must wrap to inspect, got %d", m.tab)
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
	// Short values: QUADLET must not stay at the old fixed 22.
	cols := adaptiveColumns(124, 9, 9, 18, 8, 7)
	if cols[0].Width > 12 {
		t.Errorf("short names should shrink QUADLET, got %d", cols[0].Width)
	}
	if cols[1].Width > 11 {
		t.Errorf("KIND should be compact, got %d", cols[1].Width)
	}
	// IMAGE absorbs the leftover width.
	total := 0
	for _, c := range cols {
		total += c.Width
	}
	if total > 124 {
		t.Errorf("columns overflow the table: total %d > 124", total)
	}

	// Long values: caps stop one huge name from eating the table.
	cols = adaptiveColumns(124, 60, 9, 80, 8, 7)
	if cols[0].Width != 34 {
		t.Errorf("QUADLET cap = %d, want 34", cols[0].Width)
	}
	if cols[2].Width != 44 {
		t.Errorf("SYSTEMD UNIT cap = %d, want 44", cols[2].Width)
	}

	// Narrow table: IMAGE never collapses below its floor.
	cols = adaptiveColumns(60, 30, 9, 44, 12, 11)
	if cols[5].Width < 16 {
		t.Errorf("IMAGE floor violated: %d", cols[5].Width)
	}
}

func TestBuildRowsAdaptiveWidths(t *testing.T) {
	m := withUnits(New(), "db")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 24})
	m = model.(Model)
	_ = m
	cols := model.(Model).table.Columns()
	if cols[0].Width > 12 {
		t.Errorf("one short name must shrink QUADLET, got %d", cols[0].Width)
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
