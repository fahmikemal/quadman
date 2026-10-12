package ui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// sampleModel is a small stack in the shape of the design mockups: a pod
// with two containers (one failed, referencing a missing volume and a
// secret), a volume, and a container with an auto-update policy.
func sampleModel(t *testing.T, w, h int) Model {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	units := []quadlet.Unit{
		{Name: "aegis-app", Kind: quadlet.KindContainer, UnitName: "aegis-app.service", Path: write("aegis-app.container",
			"[Container]\nImage=ghcr.io/x/aegis:1.4.2\nPod=aegis.pod\nVolume=pgdata2.volume:/var/lib/aegis\nSecret=db-pass,type=env,target=DB_PASS\nPublishPort=8443:8443\n[Unit]\nAfter=aegis-db.service\n")},
		{Name: "aegis-db", Kind: quadlet.KindContainer, UnitName: "aegis-db.service", Path: write("aegis-db.container",
			"[Container]\nImage=postgres:17-alpine\nPod=aegis.pod\nVolume=pgdata.volume:/var/lib/postgresql/data\nVolume=shared:/shared\nVolume=/srv/x:/x\nSecret=db-pass\nSecret=grafana-admin,target=admin\n")},
		{Name: "aegis", Kind: quadlet.KindPod, UnitName: "aegis-pod.service", Path: write("aegis.pod", "[Pod]\n")},
		{Name: "pgdata", Kind: quadlet.KindVolume, UnitName: "pgdata-volume.service", Path: write("pgdata.volume", "[Volume]\nVolumeName=pg-main\n")},
		{Name: "demo-web", Kind: quadlet.KindContainer, UnitName: "demo-web.service", Path: write("demo-web.container",
			"[Container]\nImage=docker.io/library/busybox:latest\nAutoUpdate=registry\n")},
	}
	st := map[string]systemd.Status{}
	for _, u := range units {
		st[u.UnitName] = systemd.Status{Id: u.UnitName, LoadState: "loaded", ActiveState: "active", SubState: "running", ActiveSince: time.Now().Add(-26 * time.Hour)}
	}
	st["aegis-app.service"] = systemd.Status{Id: "aegis-app.service", LoadState: "loaded", ActiveState: "failed", SubState: "failed", Result: "exit-code", ExecMainStatus: 1, NRestarts: 4}
	m := New()
	m.whoami = "fahmi@tesseract"
	model, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	model, _ = model.(Model).Update(refreshMsg{
		units: units, statuses: st, linger: true, lingerOK: true, version: "6.1.1",
		images: []string{"ghcr.io/x/aegis:1.4.2", "postgres:17-alpine", "", "", "docker.io/library/busybox:latest"},
		issues: []quadlet.Issue{{File: "aegis-app.container", Severity: quadlet.SeverityError, Message: "requires 'pgdata2.volume' which does not exist"}},
	})
	return model.(Model)
}

func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	}
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		model, _ := m.Update(pressKey(k))
		m = model.(Model)
	}
	return m
}

// --- shared screen skeleton ------------------------------------------------

func TestLayoutColsKeepsCellsAlignedWhenAMiddleColumnHides(t *testing.T) {
	cols := []scrCol{{title: "A", w: 4}, {title: "B", w: 4, hide: breakpointMid}, {title: "C", w: 4}}
	kept, widths := layoutCols(cols, 80)
	if len(kept) != 2 || kept[1].title != "C" || kept[1].src != 2 || widths[1] != 4 {
		t.Fatalf("kept = %+v widths = %v", kept, widths)
	}
	row := renderScrRow(scrRow{cells: []scrCell{cell("aaaa", fgStyle), cell("bbbb", fgStyle), cell("cccc", fgStyle)}}, kept, widths, 80, false)
	if got := ansi.Strip(row); strings.Contains(got, "bbbb") || !strings.Contains(got, "aaaa cccc") {
		t.Errorf("hidden middle column leaked or shifted cells: %q", got)
	}
}

func TestFlexColumnsHonorCaps(t *testing.T) {
	cols := []scrCol{{title: "A", min: 5, max: 10}, {title: "B", min: 5}}
	_, widths := layoutCols(cols, 60)
	if widths[0] != 10 || widths[1] != 60-rowPrefixW-1-10 {
		t.Errorf("capped flex column must give its surplus away: %v", widths)
	}
}

func TestScreenFilterKeepsMatchingGroupsOnly(t *testing.T) {
	m := New()
	rows := []scrRow{
		{heading: "USED"}, {cells: []scrCell{cell("alpha", fgStyle)}},
		{heading: "UNUSED"}, {cells: []scrCell{cell("bravo", fgStyle)}},
	}
	m.scrFilter = "brav"
	got := m.visibleRows(rows)
	if len(got) != 2 || got[0].heading != "UNUSED" || got[1].cells[0].text != "bravo" {
		t.Errorf("filter must keep only matching rows and their heading: %+v", got)
	}
}

// Every key on a secondary screen is consumed there: none may reach the
// unit-list actions behind it (s would start, x would arm a stop, …).
func TestScreenKeysNeverLeakIntoListActions(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.enterScreen(modeRecent)
	for _, k := range []string{"s", "x", "r", "e", "d", "D", "R", "L", "h", "E", "i", "y"} {
		mm := press(m, k)
		if mm.mode != modeRecent && mm.mode != modeList {
			t.Errorf("%s changed mode to %d", k, mm.mode)
		}
		if mm.busy || (mm.pending != nil && mm.pending.unit.Name != "") {
			t.Errorf("%s on the actions screen started a unit action (busy=%v pending=%+v)", k, mm.busy, mm.pending)
		}
	}
}

func TestNavKeysSwitchViewsFromAnyScreen(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m = press(m, "A")
	if m.mode != modeRecent {
		t.Fatalf("A must open actions, mode=%d", m.mode)
	}
	m = press(m, "v")
	if m.mode != modeValidate {
		t.Errorf("v from actions must open problems, mode=%d", m.mode)
	}
	m = press(m, "t")
	if m.mode != modeTree {
		t.Errorf("t from problems must open the tree, mode=%d", m.mode)
	}
	m = press(m, "esc")
	if m.mode != modeList {
		t.Errorf("esc must return to the unit list, mode=%d", m.mode)
	}
}

func TestEnterOpensTheRowsUnitInTheList(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m = press(m, "v", "enter")
	if m.mode != modeList {
		t.Fatalf("enter on a problem must jump to its unit, mode=%d", m.mode)
	}
	if u, _ := m.selected(); u.Name != "aegis-app" {
		t.Errorf("selected = %s, want aegis-app", u.Name)
	}
}

func TestNarrowScreensMoveDetailToFullScreen(t *testing.T) {
	m := sampleModel(t, 80, 24)
	m = press(m, "v")
	if strings.Contains(ansi.Strip(m.View().Content), "HOW TO FIX") {
		t.Fatal("below 100 columns the detail panel must not render under the table")
	}
	m = press(m, "enter")
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "HOW TO FIX") {
		t.Errorf("enter must show the detail full screen:\n%s", v)
	}
	m = press(m, "esc")
	if m.mode != modeValidate || m.scrFull {
		t.Errorf("esc closes the full-screen detail first (mode=%d full=%v)", m.mode, m.scrFull)
	}
}

func TestEveryScreenFitsTheTerminal(t *testing.T) {
	for _, sz := range [][2]int{{60, 16}, {80, 24}, {120, 36}, {170, 40}} {
		base := sampleModel(t, sz[0], sz[1])
		for _, md := range []mode{modeStorage, modeEvents, modeTimers, modeSecrets, modeRecent, modeStats, modeUpdates, modeValidate, modeTree} {
			m := base
			m.treeView = m.computeTree()
			m.probList = m.computeProblems()
			m.enterScreen(md)
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != sz[1] {
				t.Errorf("%dx%d mode %d: %d lines, want exactly %d (footer pinned)", sz[0], sz[1], md, len(lines), sz[1])
			}
			for i, l := range lines {
				if ansi.StringWidth(l) > sz[0] {
					t.Errorf("%dx%d mode %d line %d is %d wide", sz[0], sz[1], md, i, ansi.StringWidth(l))
				}
			}
		}
	}
}

// --- storage ---------------------------------------------------------------

func TestVolumeReferences(t *testing.T) {
	m := sampleModel(t, 120, 36)
	refs := volumeReferences(m.units, m.parseUnitFile)
	if got := refs["pg-main"]; len(got) != 2 || !slices.Contains(got, "pgdata-volume.service") || !slices.Contains(got, "aegis-db.service") {
		t.Errorf("VolumeName= must resolve for both the .volume unit and its user: %v", got)
	}
	if got := refs["shared"]; len(got) != 1 || got[0] != "aegis-db.service" {
		t.Errorf("a bare named volume is a reference: %v", got)
	}
	if got := refs["systemd-pgdata2"]; len(got) != 1 {
		t.Errorf("an unknown .volume falls back to systemd-<name>: %v", refs)
	}
	for vol := range refs {
		if strings.HasPrefix(vol, "/") {
			t.Errorf("bind mounts are not volumes: %q", vol)
		}
	}
}

func TestImageRefMatches(t *testing.T) {
	names := []string{"docker.io/library/postgres:17-alpine"}
	for ref, want := range map[string]bool{
		"postgres:17-alpine":                   true,
		"docker.io/library/postgres:17-alpine": true,
		"library/postgres:17-alpine":           true,
		"postgres:16":                          false,
		"gres:17-alpine":                       false,
	} {
		if got := imageRefMatches(names, ref); got != want {
			t.Errorf("imageRefMatches(%q) = %v, want %v", ref, got, want)
		}
	}
	if !imageRefMatches([]string{"docker.io/library/busybox:latest"}, "busybox") {
		t.Error("an untagged ref implies :latest")
	}
}

func TestForeignSystemdLabelsAreNotQuadletUnits(t *testing.T) {
	m := sampleModel(t, 120, 36)
	compose := podman.ContainerEntry{Names: []string{"superset"}, Labels: map[string]string{"PODMAN_SYSTEMD_UNIT": "podman-compose@superset.service"}}
	if u := m.ctrUnit(compose); u != "" {
		t.Errorf("podman-compose's label must not link to a unit, got %q", u)
	}
	ours := podman.ContainerEntry{Names: []string{"systemd-demo-web"}}
	if u := m.ctrUnit(ours); u != "demo-web.service" {
		t.Errorf("systemd-<name> must resolve to the quadlet unit, got %q", u)
	}
}

func TestStorageRemoveRefusesWhatAUnitUses(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.storage = storageData{
		images: []podman.ImageEntry{{ID: "img1", Names: []string{"docker.io/library/busybox:latest"}}},
		df:     []podman.DfRow{{Type: "Images", Total: 1}},
	}
	m.scrLoaded |= modeBit(modeStorage)
	m.enterScreen(modeStorage)
	m = press(m, "D")
	if m.pending != nil || !strings.Contains(m.statusLine, "used by demo-web.service") {
		t.Errorf("removing an image a unit uses must be refused with the reason: %q pending=%v", m.statusLine, m.pending)
	}
}

// --- timers ----------------------------------------------------------------

func TestHumanSchedule(t *testing.T) {
	for in, want := range map[string]string{
		"OnCalendar=*-*-* *:00:00":      "hourly",
		"OnCalendar=*-*-* 03:00:00":     "daily 03:00",
		"OnCalendar=Sun *-*-* 04:00:00": "Sun 04:00",
		"OnCalendar=*-*-* 00/12:00:00":  "*-*-* 00/12:00:00",
		"OnUnitActiveUSec=1d":           "every 1d",
		"OnBootUSec=5min":               "5min after boot",
		"OnStartupUSec=5min":            "5min after login",
		"OnUnitInactiveUSec=1h":         "1h after finish",
	} {
		if got := humanSchedule(in); got != want {
			t.Errorf("humanSchedule(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimersFailedFirstWithExitCode(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.timers = timersData{
		list: []systemd.Timer{{Unit: "ok.timer"}, {Unit: "bad.timer"}},
		info: map[string]systemd.TimerInfo{
			"ok.timer":  {Unit: "ok.timer", ActiveState: "active", SubState: "waiting", Last: time.Now(), LastResult: "success"},
			"bad.timer": {Unit: "bad.timer", ActiveState: "active", SubState: "waiting", Last: time.Now(), LastResult: "exit-code", LastStatus: 125},
		},
	}
	m.enterScreen(modeTimers)
	rows := m.screenSpec().rows
	if rows[0].cells[1].text != "bad.timer" || !strings.Contains(rows[0].cells[4].text, "exit 125") {
		t.Errorf("failed timers rank first with their exit code: %+v", rows[0].cells)
	}
}

// --- secrets ---------------------------------------------------------------

func TestSecretUsesAndGrouping(t *testing.T) {
	m := sampleModel(t, 120, 36)
	uses := secretUses(m.units, m.readUnitSources)
	app := uses["db-pass"]
	if len(app) != 2 || app[0].as != "env DB_PASS" || app[0].line != 5 || app[1].as != "file" {
		t.Fatalf("db-pass uses = %+v", app)
	}
	if g := uses["grafana-admin"]; len(g) != 1 || g[0].as != "file /run/secrets/admin" {
		t.Errorf("relative file targets live under /run/secrets: %+v", g)
	}
	m.secrets = secretsData{list: []podman.SecretEntry{{Name: "db-pass", Driver: "file"}, {Name: "old", Driver: "file"}}, uses: uses}
	m.enterScreen(modeSecrets)
	var ids []string
	for _, r := range m.screenSpec().rows {
		ids = append(ids, r.id)
	}
	if strings.Join(ids, ",") != "missing:grafana-admin,secret:old,secret:db-pass" {
		t.Errorf("missing and unused secrets rise to the top: %v", ids)
	}
}

func TestSecretValuesNeverRenderAndCopyIsGated(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.secrets = secretsData{list: []podman.SecretEntry{{Name: "db-pass"}}, uses: secretUses(m.units, m.readUnitSources)}
	m.scrLoaded |= modeBit(modeSecrets)
	m.enterScreen(modeSecrets)
	m = press(m, "down", "down") // onto db-pass
	m.readonly = true
	m = press(m, "c")
	if m.busy || !strings.Contains(m.statusLine, "disabled in readonly") {
		t.Errorf("copying a secret must be refused in readonly: %q", m.statusLine)
	}
	m.readonly = false
	m.clientInfo = "alice@laptop"
	m = press(m, "c")
	if m.busy || !strings.Contains(m.statusLine, "served sessions") {
		t.Errorf("copying a secret must be refused in served sessions: %q", m.statusLine)
	}
	// The rotate prompt masks what is typed.
	m.clientInfo = ""
	m = press(m, "r", "h", "u", "n", "t", "e", "r", "2")
	if v := ansi.Strip(m.View().Content); strings.Contains(v, "hunter2") || !strings.Contains(v, "•••") {
		t.Errorf("the secret value must be masked while typed:\n%s", v)
	}
}

// --- events ----------------------------------------------------------------

func TestManagerEventMapping(t *testing.T) {
	for _, tc := range []struct {
		e    systemd.ManagerEntry
		want string
		sev  glyphKind
	}{
		{systemd.ManagerEntry{Unit: "a.service", JobType: "start", JobResult: "done", Message: "Started a."}, glyph(gActive) + " started", gActive},
		{systemd.ManagerEntry{Unit: "a.service", Message: "a.service: Failed with result 'exit-code'."}, glyph(gFailed) + " failed", gFailed},
		{systemd.ManagerEntry{Unit: "a.service", Message: "a.service: Scheduled restart job, restart counter is at 3."}, "↻ restart", gWarn},
	} {
		r := managerEventRec(tc.e)
		if r.event != tc.want || r.sev != tc.sev || strings.HasPrefix(r.detail, "a.service:") {
			t.Errorf("%q → event %q sev %v detail %q", tc.e.Message, r.event, r.sev, r.detail)
		}
	}
}

func TestEventTimelineFilters(t *testing.T) {
	m := sampleModel(t, 120, 36)
	now := time.Now()
	m.events = eventsData{since: time.Hour, recs: []eventRec{
		{at: now.Add(-2 * time.Hour), typ: "container", event: "old"},
		{at: now.Add(-3 * time.Minute), typ: "container", event: "died", unit: "demo-web.service"},
		{at: now.Add(-2 * time.Minute), typ: "service", event: "started", unit: "aegis-db.service"},
		{at: now.Add(-1 * time.Minute), typ: "image", event: "pull"},
	}}
	if got := m.eventTimeline(); len(got) != 3 || got[0].event != "pull" {
		t.Fatalf("since 1h, newest first: %+v", got)
	}
	m.events.typeIdx = 3 // service
	if got := m.eventTimeline(); len(got) != 1 || got[0].event != "started" {
		t.Errorf("type filter: %+v", got)
	}
	m.events.typeIdx = 0
	m.events.unitOnly = "demo-web.service"
	if got := m.eventTimeline(); len(got) != 1 || got[0].event != "died" {
		t.Errorf("unit filter: %+v", got)
	}
}

func TestEventsHistoryMergeDeduplicates(t *testing.T) {
	m := New()
	at := time.Now()
	live := eventRec{at: at, typ: "container", event: "start", object: "web"}
	m.events.recs = []eventRec{live}
	mm, _ := m.applyEventsHistory(eventsHistoryMsg{recs: []eventRec{live, {at: at.Add(-time.Minute), typ: "image", event: "pull"}}, window: time.Hour})
	got := mm.(Model).events.recs
	if len(got) != 2 || got[0].event != "pull" {
		t.Errorf("history and live stream must merge without duplicates: %+v", got)
	}
}

// --- actions ---------------------------------------------------------------

func TestActionCommandAndRepeat(t *testing.T) {
	m := sampleModel(t, 120, 36)
	if c := m.actionCommand("restart demo-web.service"); c != "systemctl --user restart demo-web.service" {
		t.Errorf("command = %q", c)
	}
	if c := m.actionCommand("enable at boot demo-web.service"); c != "" {
		t.Errorf("non-systemctl actions have no command: %q", c)
	}
	ran := 0
	m.trigger = "key r"
	m.setBusy("restart")
	model, _ := m.Update(actionMsg{desc: "restart demo-web.service", took: time.Second, run: func(context.Context) (string, error) { ran++; return "", nil }})
	m = model.(Model)
	if rec := m.actions[len(m.actions)-1]; rec.how != "key r" || rec.took != time.Second || rec.redo == nil {
		t.Fatalf("record = %+v", rec)
	}
	m = press(m, "A", ".")
	if m.pending == nil || m.pending.verb != "repeat" {
		t.Fatal(". must ask before repeating an action")
	}
	model, cmd := m.Update(pressKey("y"))
	m = model.(Model)
	if !m.busy || cmd == nil {
		t.Fatal("confirming must re-run the action")
	}
}

// --- confirmations ---------------------------------------------------------

func TestStopConfirmModal(t *testing.T) {
	m := sampleModel(t, 120, 36)
	for i, u := range m.filtered {
		if u.Name == "demo-web" {
			m.table.SetCursor(i)
		}
	}
	m = press(m, "x")
	if m.pending == nil {
		t.Fatal("x must arm a stop")
	}
	v := ansi.Strip(m.View().Content)
	for _, want := range []string{"Stop demo-web.service?", "THIS WILL", "remove its container", "KEEPS", "demo-web.container", "y/N"} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q:\n%s", want, v)
		}
	}
	if m.confirm.focus != 0 {
		t.Error("focus must start on Cancel")
	}
	// enter on Cancel cancels.
	mm := press(m, "enter")
	if mm.pending != nil || mm.busy {
		t.Error("enter with focus on Cancel must cancel")
	}
	// tab, then enter confirms.
	mm = press(m, "tab", "enter")
	if mm.pending != nil || !mm.busy {
		t.Error("tab to Stop, enter must run it")
	}
}

func TestBulkStopModalListsUnitsAndDependents(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.marks = map[string]bool{"aegis-db.service": true, "demo-web.service": true}
	m.buildRows()
	m = press(m, "x")
	if m.pending == nil || !strings.HasPrefix(m.pending.title, "Stop 2 units") {
		t.Fatalf("pending = %+v", m.pending)
	}
	body := strings.Join(m.pending.lines, "\n")
	if !strings.Contains(body, "aegis-db.service") || !strings.Contains(body, "depend on them may stop or fail: aegis-app") {
		t.Errorf("bulk stop must list units and dependents:\n%s", body)
	}
}

// --- generate ----------------------------------------------------------------

func TestSplitGeneratedMultiFile(t *testing.T) {
	out := "# FileName=web\n[Container]\nImage=nginx\nPublishPort=8080:80\n\n---\n\n# FileName=db.container\n[Container]\nImage=postgres:16\n"
	files, err := splitGenerated(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].name != "web.container" || files[1].name != "db.container" || strings.Contains(files[0].content, "---") {
		t.Errorf("files = %+v", files)
	}
}

func TestNormalizeRunArgs(t *testing.T) {
	for in, want := range map[string]string{
		"podman run -d --name web nginx":         "podman run --detach=true --name web nginx",
		"podman run -dit --name web nginx":       "podman run --detach=true -it --name web nginx",
		"docker run --detach -p 80:80 nginx":     "docker run --detach=true -p 80:80 nginx",
		"podman run --name web nginx redis -d x": "podman run --name web nginx redis -d x",
	} {
		if got := strings.Join(normalizeRunArgs(strings.Fields(in)), " "); got != want {
			t.Errorf("normalizeRunArgs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHostPortsAndPortClash(t *testing.T) {
	if got := hostPorts([]string{"8080:80", "127.0.0.1:9090:90/tcp", "53"}); strings.Join(got, ",") != "8080,9090" {
		t.Errorf("hostPorts = %v", got)
	}
	m := sampleModel(t, 120, 36)
	checks := m.localChecks([]genFile{{name: "new.container", content: "[Container]\nPublishPort=8443:443\n"}})
	if len(checks) == 0 || !strings.Contains(checks[0].text, "port 8443 used by aegis-app") {
		t.Errorf("a host port another unit publishes must be flagged: %+v", checks)
	}
}

func TestInstallGeneratedWritesEveryFile(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	m := New()
	m.sys.Bin = writeScript(t, "systemctl", "exit 0\n")
	m.genFiles = []genFile{{name: "web.container", content: "[Container]\nImage=nginx\n"}, {name: "db.container", content: "[Container]\nImage=pg\n"}}
	m.genBoot = true
	m.mode = modeGenerate
	model, cmd := m.installGenerated()
	if model.(Model).mode != modeList || cmd == nil {
		t.Fatal("y must leave the preview and start writing")
	}
	for _, c := range cmd().(tea.BatchMsg) {
		if c == nil {
			continue
		}
		if am, ok := c().(actionMsg); ok && am.err != nil {
			t.Fatalf("write failed: %v", am.err)
		}
	}
	for _, name := range []string{"web.container", "db.container"} {
		data, err := os.ReadFile(filepath.Join(cfg, "containers", "systemd", name))
		if err != nil {
			t.Fatalf("%s not written: %v", name, err)
		}
		if !strings.Contains(string(data), "WantedBy=default.target") {
			t.Errorf("%s: boot toggle must add [Install]:\n%s", name, data)
		}
	}
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- detail, journal, problems, editor, splash ---------------------------------

func TestFormatLogLine(t *testing.T) {
	line := "2026-10-11T16:14:02+0700 host systemd[1]: demo-web.service: Main process exited, code=exited, status=137/n/a, failed"
	got := ansi.Strip(formatLogLine(line))
	if !strings.HasPrefix(got, "16:14:02  systemd") || !strings.Contains(got, glyph(gFailed)+" demo-web.service") {
		t.Errorf("formatted = %q", got)
	}
	id := strings.Repeat("ab", 32)
	if got := ansi.Strip(formatLogLine("2026-10-11T16:14:05+0700 host podman[9]: container died " + id)); !strings.Contains(got, "abababab…") || strings.Contains(got, id) {
		t.Errorf("64-hex IDs are shortened: %q", got)
	}
	if got := ansi.Strip(formatLogLine("-- No entries --")); got != "-- No entries --" {
		t.Errorf("non-journal lines pass through: %q", got)
	}
}

func TestNearestUnitSuggestion(t *testing.T) {
	m := sampleModel(t, 120, 36)
	if got := m.nearestUnit("pgdata2"); got != "pgdata" {
		t.Errorf("nearestUnit = %q", got)
	}
	if got := m.nearestUnit("completely-different"); got != "" {
		t.Errorf("far names get no suggestion: %q", got)
	}
}

func TestEditorArgvAtLine(t *testing.T) {
	for editor, want := range map[string]string{
		"vim":         "vim +9 /f",
		"nano":        "nano +9 /f",
		"code --wait": "code --wait --goto /f:9",
		"hx":          "hx /f:9",
		"ed":          "ed /f",
	} {
		argv, err := editorArgvAt(editor, "/f", 9)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(argv, " "); got != want {
			t.Errorf("%s → %q, want %q", editor, got, want)
		}
	}
}

func TestSplash(t *testing.T) {
	full := ansi.Strip(Splash("v1.2.3", "podman 6.1.1 · systemd 259", 120))
	if !strings.Contains(full, "████  ████") || !strings.Contains(full, "quadman v1.2.3") || !strings.Contains(full, "systemd 259") {
		t.Errorf("full splash:\n%s", full)
	}
	compact := ansi.Strip(Splash("v1.2.3", "x", 60))
	if strings.Contains(compact, "████") || !strings.Contains(compact, "▀▀ ▀▀  quadman v1.2.3") {
		t.Errorf("compact splash below 80 columns:\n%s", compact)
	}
}

func TestDetailHeaderShowsRestarts(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.table.SetCursor(0) // aegis-app: failed, 4 restarts
	h := ansi.Strip(m.detailHeader(120))
	if !strings.Contains(h, "aegis-app.service") || !strings.Contains(h, "↻ 4 restarts") || !strings.Contains(h, "crash-loop") {
		t.Errorf("header = %q", h)
	}
}

func TestStatsSortAndIdle(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.stats = statsData{entries: []podman.StatEntry{
		{Name: "small", CPU: "0.50%", Mem: "900MB / 16GB"},
		{Name: "big", CPU: "2.00%", Mem: "10MB / 16GB"},
		{Name: "idle", CPU: "0.00%", Mem: "1MB / 16GB"},
	}}
	m.scrLoaded |= modeBit(modeStats)
	m.enterScreen(modeStats)
	rows := m.screenSpec().rows
	if rows[0].cells[0].text != "big" || rows[2].heading == "" || len(rows) != 3 {
		t.Fatalf("CPU sort with idle folded: %+v", rows)
	}
	m = press(m, "right") // MEM
	if rows := m.screenSpec().rows; rows[0].cells[0].text != "small" {
		t.Errorf("→ sorts by memory: %+v", rows[0].cells[0])
	}
	m = press(m, "i")
	if rows := m.screenSpec().rows; len(rows) != 4 || !rows[3].dim {
		t.Errorf("i shows the idle containers, dimmed: %+v", rows)
	}
}

func TestUpdatesApplyNeedsPendingAndConfirmation(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m.updateEntries = []podman.AutoUpdateEntry{{Unit: "demo-web.service", Updated: "false"}}
	m.scrLoaded |= modeBit(modeUpdates)
	m.enterScreen(modeUpdates)
	if mm := press(m, "a"); mm.pending != nil {
		t.Error("with nothing pending, a must not arm anything")
	}
	m.updateEntries[0].Updated = "pending"
	mm := press(m, "a")
	if mm.pending == nil || !strings.Contains(strings.Join(mm.pending.lines, " "), "restart demo-web.service") {
		t.Errorf("a must ask before applying: %+v", mm.pending)
	}
}

func TestTreeFoldAndProblemsOnly(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m = press(m, "t")
	rows := m.screenSpec().rows
	n := len(rows)
	m = press(m, "f")
	if got := len(m.screenSpec().rows); got >= n {
		t.Errorf("f must hide healthy units: %d → %d rows", n, got)
	}
	m = press(m, "f", "z")
	all := len(m.screenSpec().rows)
	if all <= n {
		t.Errorf("the first z opens every fold: %d → %d rows", n, all)
	}
	m = press(m, "z")
	if got := len(m.screenSpec().rows); got >= all {
		t.Errorf("the second z closes every fold: %d → %d rows", all, got)
	}
}

// Typing into the screen filter goes to the input, never to the screen's
// own keys (d, e, m would otherwise be actions).
func TestScreenFilterTyping(t *testing.T) {
	m := sampleModel(t, 120, 36)
	m = press(m, "v", "/", "d", "e", "m", "o")
	if !m.scrFiltering || m.scrFilter != "demo" {
		t.Fatalf("filter = %q (filtering %v)", m.scrFilter, m.scrFiltering)
	}
	rows := m.visibleRows(m.screenSpec().rows)
	if len(rows) != 1 || !strings.Contains(rows[0].cells[1].text, "demo-web") {
		t.Errorf("rows = %+v", rows)
	}
	m = press(m, "enter")
	if m.scrFiltering || m.scrFilter != "demo" || m.mode != modeValidate {
		t.Errorf("enter keeps the filter and closes the input (filter %q mode %d)", m.scrFilter, m.mode)
	}
	m = press(m, "esc")
	if m.scrFilter != "" || m.mode != modeValidate {
		t.Errorf("esc clears the filter before leaving (filter %q mode %d)", m.scrFilter, m.mode)
	}
}
