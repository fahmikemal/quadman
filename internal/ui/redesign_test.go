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
	"github.com/fahmikemal/quadman/internal/systemd"
)

// redesignModel is a list with one unit per display state.
func redesignModel(t *testing.T) Model {
	t.Helper()
	states := []struct{ name, active, sub string }{
		{"zz-ok", "active", "running"},
		{"aa-off", "inactive", "dead"},
		{"mm-boot", "activating", "start"},
		{"kk-bad", "failed", "failed"},
		{"bb-sick", "active", "running"},
	}
	var units []quadlet.Unit
	st := map[string]systemd.Status{}
	for _, s := range states {
		u := quadlet.Unit{Name: s.name, Kind: quadlet.KindContainer, UnitName: s.name + ".service"}
		units = append(units, u)
		st[u.UnitName] = systemd.Status{Id: u.UnitName, LoadState: "loaded", ActiveState: s.active, SubState: s.sub}
	}
	model, _ := New().Update(refreshMsg{
		units: units, images: make([]string, len(units)), statuses: st, lingerOK: true,
		health: map[string]string{"systemd-bb-sick": "unhealthy"},
	})
	return model.(Model)
}

func TestSortProblemsFirst(t *testing.T) {
	m := redesignModel(t)
	var got []string
	for _, u := range m.filtered {
		got = append(got, u.Name)
	}
	want := "kk-bad bb-sick mm-boot zz-ok aa-off"
	if strings.Join(got, " ") != want {
		t.Errorf("order = %v, want failed → unhealthy → starting → active → inactive (%s)", got, want)
	}

	// S cycles to name order and keeps the cursor on the same unit.
	m.table.SetCursor(0) // kk-bad
	model, _ := m.Update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	m = model.(Model)
	if m.sortBy != sortName || m.filtered[0].Name != "aa-off" {
		t.Errorf("S must switch to name order, got %v first (sort %v)", m.filtered[0].Name, m.sortBy)
	}
	if u, _ := m.selected(); u.Name != "kk-bad" {
		t.Errorf("cursor must stay on kk-bad after re-sorting, got %s", u.Name)
	}
}

func TestStateGlyphsCarryWords(t *testing.T) {
	// TERM=linux turns on the ASCII glyphs; this test pins the Unicode set.
	prev := asciiGlyphs
	SetASCII(false)
	t.Cleanup(func() { SetASCII(prev) })
	m := redesignModel(t)
	for i, want := range []string{"✗ failed", "▲ unhealthy", "◐ starting", "● active", "○ inactive"} {
		if got := m.table.Rows()[i][0]; got != want {
			t.Errorf("row %d state = %q, want %q (glyph + word, never color alone)", i, got, want)
		}
	}
}

func TestASCIIGlyphFallback(t *testing.T) {
	defer SetASCII(false)
	SetASCII(true)
	if glyph(gFailed) != "x" || glyph(gActive) != "*" || logoMark() != "[q]" {
		t.Errorf("ASCII fallback = %q %q %q", glyph(gFailed), glyph(gActive), logoMark())
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		4 * time.Second:                "4s",
		19 * time.Hour:                 "19h",
		2*time.Hour + 11*time.Minute:   "2h 11m",
		49 * time.Hour:                 "2d 1h",
		48 * time.Hour:                 "2d",
		3*time.Minute + 20*time.Second: "3m",
		-5 * time.Second:               "0s",
	} {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%v) = %q, want %q", d, got, want)
		}
	}
	if got := uptimeOf(systemd.Status{}, time.Now()); got != "—" {
		t.Errorf("unknown start must render —, got %q", got)
	}
}

func TestLegendCapsRowsAndKeepsTail(t *testing.T) {
	for _, w := range []int{100, 124, 159, 200} {
		m := New()
		model, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		bar := model.(Model).helpBar()
		rows := strings.Split(bar, "\n")
		if len(rows) > maxLegendRows {
			t.Errorf("w=%d: legend has %d rows, max %d", w, len(rows), maxLegendRows)
		}
		for _, r := range rows {
			if ansi.StringWidth(r) > w {
				t.Errorf("w=%d: legend row overflows: %q", w, ansi.Strip(r))
			}
		}
		if !strings.Contains(ansi.Strip(rows[len(rows)-1]), "^P all commands") {
			t.Errorf("w=%d: the ^P/? tail must end the footer: %q", w, ansi.Strip(bar))
		}
	}
}

func TestTooSmallView(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	v := ansi.Strip(model.(Model).View().Content)
	if !strings.Contains(v, "Terminal too small") || !strings.Contains(v, "need 60×16") || !strings.Contains(v, "quadman") {
		t.Errorf("below 60×16 only the size notice renders: %q", v)
	}
}

func TestFrameFitsTerminal(t *testing.T) {
	// No size may push the footer and status line off screen.
	m := redesignModel(t)
	for _, sz := range [][2]int{{60, 16}, {80, 24}, {120, 36}, {170, 40}, {120, 29}} {
		model, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		out := model.(Model).View().Content
		lines := strings.Split(out, "\n")
		if len(lines) > sz[1] {
			t.Errorf("%dx%d: frame has %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if ansi.StringWidth(l) > sz[0] {
				t.Errorf("%dx%d: line %d is %d wide: %q", sz[0], sz[1], i, ansi.StringWidth(l), ansi.Strip(l))
			}
		}
		if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "linger") {
			t.Errorf("%dx%d: last line must be the status bar: %q", sz[0], sz[1], ansi.Strip(lines[len(lines)-1]))
		}
	}
}

func TestDashboardToggleAndAttention(t *testing.T) {
	m := redesignModel(t)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	m = model.(Model)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"RUNNING", "NEEDS ATTENTION", "kk-bad failed", "bb-sick unhealthy"} {
		if !strings.Contains(view, want) {
			t.Errorf("expanded dashboard must show %q", want)
		}
	}
	model, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	view = ansi.Strip(model.(Model).View().Content)
	if strings.Contains(view, "RUNNING") || !strings.Contains(view, "5 units") {
		t.Errorf("z must collapse the dashboard to the summary line: %q", view)
	}
	// Short terminals collapse on their own.
	model, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if model.(Model).dashExpanded() {
		t.Error("height < 30 must collapse the dashboard")
	}
}

func TestDashboardConfigOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "quadman"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "quadman", "config.yaml"), []byte("dashboard: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !New().dashOff {
		t.Error("config dashboard: false must start collapsed")
	}
}

func TestNavStripBadgesAndActive(t *testing.T) {
	m := redesignModel(t)
	m.issues = indexIssues([]quadlet.Issue{{File: "kk-bad.container", Severity: quadlet.SeverityError, Message: "bad key"}})
	nav := ansi.Strip(m.navStrip(160))
	if !strings.Contains(nav, "Problems 1 v") || !strings.Contains(nav, "Timers T") {
		t.Errorf("nav strip must list every view with key and badge: %q", nav)
	}
	if narrow := ansi.Strip(m.navStrip(80)); strings.Contains(narrow, "Timers") || !strings.Contains(narrow, "Units") {
		t.Errorf("below 100 cols the nav shows keys only (Units keeps its name): %q", narrow)
	}
	m.mode = modeStorage
	if m.navMode() != modeStorage {
		t.Error("the active view must be highlighted")
	}
}
