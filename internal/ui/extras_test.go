package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

func TestSmartHintsTimedOut(t *testing.T) {
	m := New()
	m.units = []quadlet.Unit{{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service", Path: "/nope/webapp.container"}}
	m.status = map[string]systemd.Status{
		"webapp.service": {Id: "webapp.service", LoadState: "loaded", ActiveState: "failed", SubState: "timed-out"},
	}
	hints := m.smartHints()
	found := false
	for _, h := range hints {
		if strings.Contains(h, "TimeoutStartSec") && strings.Contains(h, "webapp.service") {
			found = true
		}
	}
	if !found {
		t.Errorf("timed-out unit should produce a TimeoutStartSec hint, got %v", hints)
	}
}

func TestSmartHintsStartLimit(t *testing.T) {
	m := New()
	m.units = []quadlet.Unit{{Name: "db", Kind: quadlet.KindContainer, UnitName: "db.service", Path: "/nope/db.container"}}
	m.status = map[string]systemd.Status{
		"db.service": {Id: "db.service", LoadState: "loaded", ActiveState: "failed", SubState: "start-limit"},
	}
	hints := m.smartHints()
	found := false
	for _, h := range hints {
		if strings.Contains(h, "crash-loop") {
			found = true
		}
	}
	if !found {
		t.Errorf("start-limit unit should produce a crash-loop hint, got %v", hints)
	}
}

func TestSmartHintsClean(t *testing.T) {
	m := New()
	m.units = []quadlet.Unit{{Name: "ok", Kind: quadlet.KindContainer, UnitName: "ok.service", Path: "/nope/ok.container"}}
	m.status = map[string]systemd.Status{
		"ok.service": {Id: "ok.service", LoadState: "loaded", ActiveState: "active", SubState: "running"},
	}
	if hints := m.smartHints(); len(hints) != 0 {
		t.Errorf("healthy units should produce no hints, got %v", hints)
	}
}

func TestGenerateFlowMissingPodlet(t *testing.T) {
	// With podlet absent from PATH the n key must show the install hint.
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	// podlet may or may not exist here; only assert when truly absent.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	mm := model.(Model)
	if !podletAvailableForTest() && mm.generating {
		t.Error("without podlet the input must not open")
	}
}

func TestEventsSessionRouting(t *testing.T) {
	m := New()
	sess := &logSession{unit: "events"}
	m.eventSess = sess
	m.mode = modeEvents
	m.following = true

	model, _ := m.Update(logLineMsg{sess: sess, line: `{"status":"start"}`})
	m = model.(Model)
	if len(m.eventLines) != 1 || m.eventLines[0] != `{"status":"start"}` {
		t.Errorf("event lines = %v", m.eventLines)
	}

	// journal session lines must not leak into events
	other := &logSession{unit: "webapp.service"}
	model, _ = m.Update(logLineMsg{sess: other, line: "journal line"})
	m = model.(Model)
	if len(m.eventLines) != 1 {
		t.Errorf("foreign session line must be dropped: %v", m.eventLines)
	}
}

// podletAvailableForTest reports whether podlet is on PATH (checked via the
// same mechanism as the feature, through a shell-free lookup).
func podletAvailableForTest() bool {
	return podletAvailable()
}
