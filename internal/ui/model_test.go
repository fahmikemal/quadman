package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

func TestHelpBarLingerStates(t *testing.T) {
	m := New()
	if !strings.Contains(m.helpBar(), "linger: ?") {
		t.Error("before the first refresh, linger must render as unknown (?)")
	}
	m.lingerKnown = true
	if !strings.Contains(m.helpBar(), "linger: off") {
		t.Error("known-disabled linger must render as off")
	}
	m.linger = true
	if !strings.Contains(m.helpBar(), "linger: on") {
		t.Error("known-enabled linger must render as on")
	}
}

func TestApplyRefreshSetsRows(t *testing.T) {
	m := New()
	msg := refreshMsg{
		units: []quadlet.Unit{{
			Name:     "webapp",
			Kind:     quadlet.KindContainer,
			Path:     "/nowhere/webapp.container",
			UnitName: "webapp.service",
		}},
		images: []string{"docker.io/library/nginx:latest"},
		statuses: map[string]systemd.Status{
			"webapp.service": {Id: "webapp.service", LoadState: "loaded", ActiveState: "active", SubState: "running"},
		},
		linger:   true,
		lingerOK: true,
	}

	model, _ := m.Update(msg)
	mm := model.(Model)
	if mm.loading {
		t.Error("a successful refresh must clear the loading state")
	}
	if !mm.lingerKnown || !mm.linger {
		t.Errorf("linger = %v (known %v), want true/true", mm.linger, mm.lingerKnown)
	}
	rows := mm.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row[0] != "webapp" || row[2] != "webapp.service" || row[3] != "active" || row[4] != "running" || row[5] != "docker.io/library/nginx:latest" {
		t.Errorf("row = %v", row)
	}
}

func TestApplyRefreshErrorWithoutUnitsKeepsLoading(t *testing.T) {
	m := New()
	model, _ := m.Update(refreshMsg{err: errString("boom")})
	mm := model.(Model)
	if !mm.loading {
		t.Error("a failed discovery must keep the loading state")
	}
	if mm.statusLine == "" || !mm.statusErr {
		t.Errorf("a failed discovery must set an error status, got %q (err=%v)", mm.statusLine, mm.statusErr)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestEscReturnsToList(t *testing.T) {
	m := New()
	m.mode = modeLogs
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.(Model).mode != modeList {
		t.Error("esc must return from the logs view to the list")
	}
}

func TestQReturnsToListFromFileView(t *testing.T) {
	m := New()
	m.mode = modeFile
	model, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if model.(Model).mode != modeList {
		t.Error("q must return from the file view to the list")
	}
}

func TestQQuitsFromList(t *testing.T) {
	m := New()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Error("q in the list must quit")
	}
}

func TestCursorPinnedAcrossRefresh(t *testing.T) {
	m := New()
	refresh := func(names ...string) Model {
		units := make([]quadlet.Unit, len(names))
		images := make([]string, len(names))
		for i, n := range names {
			units[i] = quadlet.Unit{Name: n, Kind: quadlet.KindContainer, UnitName: n + ".service"}
		}
		model, _ := m.Update(refreshMsg{units: units, images: images, lingerOK: true})
		m = model.(Model)
		return m
	}

	m = refresh("alpha", "bravo", "charlie")
	m.table.SetCursor(2) // select charlie
	if m.table.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", m.table.Cursor())
	}

	m = refresh("alpha", "charlie", "bravo") // order changed, same units
	if c := m.table.Cursor(); c != 1 {
		t.Errorf("cursor = %d, want 1 (pinned on charlie by name)", c)
	}
	if u, ok := m.selected(); !ok || u.Name != "charlie" {
		t.Errorf("selection = %+v, want charlie", u)
	}
}

func TestBusyClearedByActionResult(t *testing.T) {
	m := New()
	units := []quadlet.Unit{{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{""}})
	m = model.(Model)
	m.table.SetCursor(0)

	model, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	mm := model.(Model)
	if !mm.busy {
		t.Fatal("pressing start should set the busy spinner")
	}

	model, _ = mm.Update(actionMsg{desc: "start webapp.service"})
	mm = model.(Model)
	if mm.busy {
		t.Error("the action result must clear the busy spinner")
	}
	if mm.statusLine == "" {
		t.Error("the action result should set a status line")
	}
}

func TestStaleReloadBanner(t *testing.T) {
	m := New()
	units := []quadlet.Unit{{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service"}}
	msg := refreshMsg{units: units, images: []string{""}, stale: units, lingerOK: true}
	model, _ := m.Update(msg)
	view := model.(Model).View().Content

	if !strings.Contains(view, "daemon-reload") {
		t.Error("the view should warn about stale quadlet files")
	}
}

func TestHelpBarContextual(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 24})
	m = model.(Model)
	list := m.helpBar()
	if !strings.Contains(list, "e/d") || !strings.Contains(list, "reload") {
		t.Errorf("list help bar should mention enable/disable and daemon-reload: %q", list)
	}
	if strings.Contains(list, "…") {
		t.Errorf("compact help bar should fit without truncation at 124 cols: %q", list)
	}
	m.mode = modeLogs
	view := m.helpBar()
	if !strings.Contains(view, "back") {
		t.Errorf("logs view help should offer back navigation: %q", view)
	}
	if strings.Contains(view, "boot") {
		t.Errorf("logs view help must not offer list-only actions: %q", view)
	}
}
