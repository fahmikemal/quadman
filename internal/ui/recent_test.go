package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestRecentActionsRecorded(t *testing.T) {
	m := New()
	model, _ := m.Update(actionMsg{desc: "start webapp.service"})
	mm := model.(Model)
	if len(mm.actions) != 1 || !mm.actions[0].ok || mm.actions[0].desc != "start webapp.service" {
		t.Fatalf("actions = %+v, want 1 ok record", mm.actions)
	}

	model, _ = mm.Update(actionMsg{desc: "stop db.service", err: errors.New("boom")})
	mm = model.(Model)
	if len(mm.actions) != 2 || mm.actions[1].ok {
		t.Fatalf("actions = %+v, want second record failed", mm.actions)
	}
	view := mm.actionsView()
	if !strings.Contains(view, "start webapp.service") || !strings.Contains(view, "boom") {
		t.Errorf("actionsView must list results:\n%s", view)
	}
}

func TestRecentActionsCapped(t *testing.T) {
	m := New()
	for i := 0; i < maxActions+5; i++ {
		m.recordAction("action", nil)
	}
	if len(m.actions) != maxActions {
		t.Fatalf("actions = %d, want cap %d", len(m.actions), maxActions)
	}
}

func TestRecentScreenFlow(t *testing.T) {
	m := New()
	model, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
	mm := model.(Model)
	if mm.mode != modeRecent {
		t.Fatal("A must open the recent-actions screen")
	}
	if !strings.Contains(mm.viewport.View(), "No actions yet") {
		t.Errorf("empty log must explain itself: %q", mm.viewport.View())
	}
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.(Model).mode != modeList {
		t.Error("esc must return to the list")
	}
}

func TestHealthRecorded(t *testing.T) {
	m := New()
	model, _ := m.Update(healthMsg{container: "systemd-web", ok: true})
	mm := model.(Model)
	if len(mm.actions) != 1 {
		t.Fatal("healthy healthcheck must be logged")
	}
	model, _ = mm.Update(healthMsg{container: "systemd-web", ok: false})
	mm = model.(Model)
	if len(mm.actions) != 2 || mm.actions[1].ok {
		t.Errorf("unhealthy healthcheck must log as failed: %+v", mm.actions)
	}
}

func TestTimerAndLingerRecorded(t *testing.T) {
	m := New()
	model, _ := m.Update(timerToggledMsg{})
	mm := model.(Model)
	if len(mm.actions) != 1 {
		t.Fatal("timer toggle must be logged")
	}
	model, _ = mm.Update(lingerSetMsg{on: true, known: true})
	if len(model.(Model).actions) != 2 {
		t.Fatal("linger set must be logged")
	}
}
