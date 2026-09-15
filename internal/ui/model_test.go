package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/podman"
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
	if !strings.Contains(list, "enable") || !strings.Contains(list, "reload") || !strings.Contains(list, "filter") {
		t.Errorf("list legend should spell out actions in full words: %q", list)
	}
	if strings.Contains(list, "…") {
		t.Errorf("two-line legend should fit without truncation at 124 cols: %q", list)
	}
	m.mode = modeLogs
	view := m.helpBar()
	if !strings.Contains(view, "back") || !strings.Contains(view, "pause") {
		t.Errorf("logs view legend should offer pause/back navigation: %q", view)
	}
	if strings.Contains(view, "boot") || strings.Contains(view, "filter") {
		t.Errorf("logs view legend must not offer list-only actions: %q", view)
	}
}

func withUnits(m Model, names ...string) Model {
	units := make([]quadlet.Unit, len(names))
	images := make([]string, len(names))
	for i, n := range names {
		units[i] = quadlet.Unit{Name: n, Kind: quadlet.KindContainer, UnitName: n + ".service"}
	}
	model, _ := m.Update(refreshMsg{units: units, images: images, lingerOK: true})
	return model.(Model)
}

func TestFilterUnits(t *testing.T) {
	m := withUnits(New(), "webapp", "db", "cache", "metrics")

	m.filterStr = "web"
	m.refilter()
	if len(m.filtered) != 1 || m.filtered[0].Name != "webapp" {
		t.Errorf("filter 'web' = %+v, want [webapp]", m.filtered)
	}

	m.filterStr = "mtc" // subsequence of "metrics"
	m.refilter()
	if len(m.filtered) != 1 || m.filtered[0].Name != "metrics" {
		t.Errorf("subsequence filter 'mtc' = %+v, want [metrics]", m.filtered)
	}

	m.filterStr = "zzz"
	m.refilter()
	if len(m.filtered) != 0 {
		t.Errorf("filter 'zzz' should match nothing, got %+v", m.filtered)
	}
	if _, ok := m.selected(); ok {
		t.Error("selected() must fail when the filter matches nothing")
	}
}

func TestFilterKeyFlow(t *testing.T) {
	m := withUnits(New(), "webapp", "db")

	model, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = model.(Model)
	if !m.filtering {
		t.Fatal("/ should focus the filter input")
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = model.(Model)
	if m.filterStr != "w" || len(m.filtered) != 1 {
		t.Errorf("typing should filter live: str=%q filtered=%v", m.filterStr, m.filtered)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = model.(Model)
	if m.filtering || m.filterStr != "" || len(m.filtered) != 2 {
		t.Errorf("esc must clear the filter: filtering=%v str=%q filtered=%v", m.filtering, m.filterStr, m.filtered)
	}
}

func TestStopConfirmationFlow(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = model.(Model)
	if !m.confirmStop {
		t.Fatal("x should arm the stop confirmation")
	}
	if !strings.Contains(m.statusLine, "y/N") {
		t.Errorf("confirmation prompt missing: %q", m.statusLine)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = model.(Model)
	if m.confirmStop {
		t.Error("any key other than y must cancel the confirmation")
	}
}

func TestStopConfirmedSetsBusy(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = model.(Model)
	model, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mm := model.(Model)
	if mm.confirmStop {
		t.Error("y must consume the confirmation")
	}
	if !mm.busy {
		t.Error("confirmed stop should start the busy spinner")
	}
}

func TestFollowLogLines(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.mode = modeLogs
	m.following = true
	sess := &logSession{unit: "webapp.service"}
	m.sess = sess

	model, _ := m.Update(logLineMsg{sess: sess, line: "first"})
	m = model.(Model)
	model, _ = m.Update(logLineMsg{sess: sess, line: "second"})
	m = model.(Model)
	if got := strings.Join(m.logLines, "\n"); got != "first\nsecond" {
		t.Errorf("logLines = %q", got)
	}

	// a superseded session must be ignored
	other := &logSession{unit: "webapp.service"}
	model, _ = m.Update(logLineMsg{sess: other, line: "stale"})
	m = model.(Model)
	if len(m.logLines) != 2 {
		t.Errorf("stale session line must be dropped: %v", m.logLines)
	}
}

func TestHealthShownInRows(t *testing.T) {
	m := New()
	units := []quadlet.Unit{{Name: "webapp", Kind: quadlet.KindContainer, UnitName: "webapp.service"}}
	msg := refreshMsg{
		units:    units,
		images:   []string{"nginx"},
		statuses: map[string]systemd.Status{"webapp.service": {Id: "webapp.service", LoadState: "loaded", ActiveState: "active", SubState: "running"}},
		health:   map[string]string{"systemd-webapp": "unhealthy"},
		lingerOK: true,
	}
	model, _ := m.Update(msg)
	rows := model.(Model).table.Rows()
	if rows[0][3] != "unhealthy" || rows[0][4] != "health" {
		t.Errorf("row state = %q/%q, want unhealthy/health", rows[0][3], rows[0][4])
	}
}

func TestUpdatesScreen(t *testing.T) {
	m := New()
	msg := updatesMsg{
		entries:      []podman.AutoUpdateEntry{{Unit: "webapp.service", Policy: "registry", Updated: "pending", Image: "nginx"}},
		timerEnabled: "enabled",
		timerActive:  "active",
	}
	model, _ := m.Update(msg)
	mm := model.(Model)
	if mm.mode != modeUpdates {
		t.Fatal("updatesMsg should switch to the updates screen")
	}
	if !strings.Contains(mm.updatesView(), "webapp.service") || !strings.Contains(mm.updatesView(), "pending") {
		t.Errorf("updates view missing entry: %q", mm.updatesView())
	}
}

func TestEditorFinishedHint(t *testing.T) {
	m := New()
	model, _ := m.Update(editorFinishedMsg{changed: true})
	if got := model.(Model).statusLine; !strings.Contains(got, "press R") {
		t.Errorf("changed file should hint daemon-reload: %q", got)
	}
	model, _ = m.Update(editorFinishedMsg{changed: false})
	if got := model.(Model).statusLine; !strings.Contains(got, "no changes") {
		t.Errorf("unchanged file should say so: %q", got)
	}
}
