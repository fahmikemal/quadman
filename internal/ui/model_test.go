package ui

import (
	"os"
	"path/filepath"
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

func TestApplyRefreshEmptyDiscoveryShowsEmptyState(t *testing.T) {
	// Discover returns nil units when nothing is found; that must still
	// finish loading so the empty-state hint renders (first-run bug found
	// by the e2e).
	m := New()
	model, _ := m.Update(refreshMsg{units: nil, lingerOK: true})
	mm := model.(Model)
	if mm.loading {
		t.Fatal("an empty but successful discovery must finish loading")
	}
	if view := mm.View().Content; !strings.Contains(view, "No quadlet units found") {
		t.Error("the empty-state hint must render when no units exist")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestEscReturnsToList(t *testing.T) {
	m := New()
	m.mode = modeDetail
	m.tab = tabJournal
	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.(Model).mode != modeList {
		t.Error("esc must return from the logs view to the list")
	}
}

func TestQReturnsToListFromFileView(t *testing.T) {
	m := New()
	m.mode = modeDetail
	m.tab = tabSource
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
	m.mode = modeDetail
	m.tab = tabJournal
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
	if m.pending == nil || m.pending.verb != "stop" {
		t.Fatal("x should arm the stop confirmation")
	}
	if !strings.Contains(m.statusLine, "y/N") {
		t.Errorf("confirmation prompt missing: %q", m.statusLine)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = model.(Model)
	if m.pending != nil {
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
	if mm.pending != nil {
		t.Error("y must consume the confirmation")
	}
	if !mm.busy {
		t.Error("confirmed stop should start the busy spinner")
	}
}

func TestEnableBootFlow(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	// No [Install] in the file → 'e' arms a confirmation mentioning it.
	model, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m = model.(Model)
	if m.pending == nil || m.pending.verb != "enable" {
		t.Fatal("e without [Install] should arm the enable confirmation")
	}
	if !strings.Contains(m.statusLine, "[Install]") {
		t.Errorf("prompt should explain the [Install] edit: %q", m.statusLine)
	}

	model, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	mm := model.(Model)
	if mm.pending != nil || !mm.busy {
		t.Error("confirmed enable should start the busy action")
	}
}

func TestEnableAlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\n\n[Install]\nWantedBy=default.target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	units := []quadlet.Unit{{Name: "webapp", Kind: quadlet.KindContainer, Path: path, UnitName: "webapp.service"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{"nginx"}, lingerOK: true})
	m = model.(Model)
	m.table.SetCursor(0)

	model, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	mm := model.(Model)
	if mm.pending != nil {
		t.Error("already boot-enabled unit must skip the confirmation")
	}
	if !mm.busy {
		t.Error("already boot-enabled unit should just be started")
	}
}

func TestDisableNotEnabled(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	mm := model.(Model)
	if mm.pending != nil {
		t.Error("disable on a non-enabled unit must not arm a confirmation")
	}
	if !strings.Contains(mm.statusLine, "not enabled") {
		t.Errorf("disable on non-enabled unit should say so: %q", mm.statusLine)
	}
}

func TestFollowLogLines(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.mode = modeDetail
	m.tab = tabJournal
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

func TestEditorPickerFirstUse(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	choices := availableEditors()
	if len(choices) == 0 {
		t.Skip("no editor binaries on PATH in this environment")
	}

	model, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	m = model.(Model)
	if !m.pickingEditor {
		t.Fatal("first E with no $EDITOR and no saved choice must open the picker")
	}
	if len(m.editorChoices) != len(choices) {
		t.Errorf("picker choices = %v, want %v", m.editorChoices, choices)
	}

	// Pick the first choice: it must be saved and the picker closed.
	model, _ = m.Update(tea.KeyPressMsg{Code: rune(choices[0].key[0]), Text: choices[0].key})
	m = model.(Model)
	if m.pickingEditor {
		t.Error("a valid pick must close the picker")
	}
	if m.cfg.Editor != choices[0].bin {
		t.Errorf("saved editor = %q, want %q", m.cfg.Editor, choices[0].bin)
	}
	// The choice must persist for a fresh model.
	fresh := New()
	if fresh.cfg.Editor != choices[0].bin {
		t.Errorf("fresh model should load the saved editor %q, got %q", choices[0].bin, fresh.cfg.Editor)
	}
}

func TestEditorPickerCancel(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	if len(availableEditors()) == 0 {
		t.Skip("no editor binaries on PATH")
	}

	model, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	m = model.(Model)
	model, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = model.(Model)
	if m.pickingEditor {
		t.Error("esc must cancel the picker")
	}
	if m.cfg.Editor != "" {
		t.Errorf("cancel must not save an editor, got %q", m.cfg.Editor)
	}
}

func TestEditorSavedChoiceSkipsPicker(t *testing.T) {
	t.Setenv("EDITOR", "")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.cfg.Editor = "nano"

	model, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	if model.(Model).pickingEditor {
		t.Error("a saved editor choice must skip the picker")
	}
}

func TestEditorEnvVarWins(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)

	model, _ := m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	if model.(Model).pickingEditor {
		t.Error("$EDITOR must skip the picker")
	}
}

func TestExpandedHelpHeightReservation(t *testing.T) {
	// resize() reserves one screen row per rendered helpBar line; the
	// expanded block must be fully reserved or short terminals clip its
	// last lines (found by the PTY e2e).
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 30})
	m = model.(Model)
	model, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m = model.(Model)
	if !m.showHelp {
		t.Fatal("? should expand the help")
	}
	lines := strings.Count(m.helpBar(), "\n") + 1
	if lines < 8 {
		t.Errorf("expanded help block should have many lines (key table + explanations), got %d", lines)
	}
	// And the compact legend must never be taller than the expanded block.
	m.showHelp = false
	if compact := strings.Count(m.helpBar(), "\n") + 1; compact > lines {
		t.Errorf("compact legend (%d lines) taller than expanded help (%d lines)", compact, lines)
	}
}

func TestLegendWrapsByWidth(t *testing.T) {
	// Narrow terminal: two lines.
	m := New()
	model, _ := m.Update(tea.WindowSizeMsg{Width: 124, Height: 24})
	narrow := model.(Model).helpBar()
	if !strings.Contains(narrow, "\n") {
		t.Errorf("legend should wrap to two lines at 124 cols: %q", narrow)
	}

	// Wide terminal (200): the full sorted key list on two lines.
	m2 := New()
	model, _ = m2.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
	wide := model.(Model).helpBar()
	if !strings.Contains(wide, "y/Y copy") || !strings.Contains(wide, "D delete") || !strings.Contains(wide, "w events") {
		t.Errorf("wide legend should contain the full sorted key set: %q", wide)
	}

	// Very wide terminal: one line.
	m3 := New()
	model, _ = m3.Update(tea.WindowSizeMsg{Width: 320, Height: 24})
	oneline := model.(Model).helpBar()
	if strings.Contains(oneline, "\n") {
		t.Errorf("legend should fit on one line at 320 cols: %q", oneline)
	}
}

func TestSystemModeTitle(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	m.width, m.height = 120, 30
	v := m.View()
	if !strings.Contains(v.Content, "[SYSTEM]") {
		t.Error("system mode title must contain [SYSTEM]")
	}
	if strings.Contains(v.Content, "rootless") {
		t.Error("system mode title must not contain 'rootless'")
	}
}

func TestSystemModeHelpBarChip(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	bar := m.helpBar()
	if !strings.Contains(bar, "system") {
		t.Error("system mode helpBar must show 'system' chip")
	}
	// "linger:" is the chip format; plain "linger" may appear in key help (L linger).
	if strings.Contains(bar, "linger:") {
		t.Error("system mode helpBar must not show 'linger:' chip")
	}
}

func TestSystemModeLingerGuard(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	mm, _, ok := m.lingerKeys(tea.KeyPressMsg{Code: 'L', Text: "L"})
	if !ok {
		t.Error("lingerKeys should consume 'L' in system mode")
	}
	model := mm.(Model)
	if !strings.Contains(model.statusLine, "rootless") {
		t.Errorf("expected rootless status message, got %q", model.statusLine)
	}
}

func TestSystemModeLingerGuardView(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	m.loading = false
	m.width = 120
	m.height = 42
	mm, _, ok := m.lingerKeys(tea.KeyPressMsg{Code: 'L', Text: "L"})
	if !ok {
		t.Fatal("not ok")
	}
	v := mm.(Model).View().Content
	if !strings.Contains(v, "linger only applies to rootless") {
		t.Fatalf("View missing status: %q", v)
	}
}

func TestSystemModeSearchDirs(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	dirs := m.searchDirs()
	if len(dirs) < 3 {
		t.Fatalf("searchDirs() = %v, want at least 3 entries", dirs)
	}
	if dirs[0] != "/run/containers/systemd" {
		t.Errorf("dirs[0] = %q, want /run/containers/systemd", dirs[0])
	}
}

func TestNewWithOptionsSystemSetsUser(t *testing.T) {
	m := NewWithOptions(Options{System: true})
	if m.sys.User {
		t.Error("System mode must set sys.User = false")
	}
	if !m.system {
		t.Error("System mode must set m.system = true")
	}
}
