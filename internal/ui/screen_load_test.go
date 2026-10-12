package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/remote"
)

// asyncScreenKeys are the nav keys whose screens load in the background.
var asyncScreenKeys = map[string]mode{
	"u": modeUpdates, "g": modeStorage, "T": modeTimers, "K": modeSecrets, "o": modeStats,
}

func plainView(m Model) string { return ansi.Strip(m.View().Content) }

// TestAsyncScreensOpenWithoutListFlash: switching to a screen whose data
// loads in the background shows that screen at once, in a loading state —
// never the unit list's dashboard (GENERATOR, LINGER, …) in between, from
// the list or from any other screen.
func TestAsyncScreensOpenWithoutListFlash(t *testing.T) {
	from := map[string]string{"list": "", "problems": "v", "tree": "t", "actions": "A", "storage": "g"}
	for fromName, fromKey := range from {
		for k, want := range asyncScreenKeys {
			m := sampleModel(t, 120, 34)
			if fromKey != "" {
				m = press(m, fromKey)
			}
			m = press(m, k)
			if m.mode != want {
				t.Errorf("%s → %q: mode = %v, want %v right after the key", fromName, k, m.mode, want)
				continue
			}
			v := plainView(m)
			if strings.Contains(v, "GENERATOR") || strings.Contains(v, "LINGER") {
				t.Errorf("%s → %q: the unit-list dashboard flashed while loading:\n%s", fromName, k, v)
			}
			if !strings.Contains(v, "loading") {
				t.Errorf("%s → %q: no loading state shown:\n%s", fromName, k, v)
			}
		}
	}
}

// TestLoadingScreenBlocksItsActions: while a screen has no data, its own
// keys (apply updates, prune, remove …) cannot act on what is not shown;
// navigation and esc still work.
func TestLoadingScreenBlocksItsActions(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "u")
	m = press(m, "a")
	if m.pending != nil {
		t.Fatalf("apply-updates armed before the list loaded: %+v", m.pending)
	}
	if !strings.Contains(m.statusLine, "still loading") {
		t.Errorf("a blocked key must say why: %q", m.statusLine)
	}
	m = press(m, "esc")
	if m.mode != modeList {
		t.Errorf("esc must leave a loading screen, mode = %v", m.mode)
	}
}

// TestLateScreenResultDoesNotPullBack: a result that lands after the user
// left its screen is kept for the next visit, never used to switch back.
func TestLateScreenResultDoesNotPullBack(t *testing.T) {
	late := map[mode]tea.Msg{
		modeStorage: storageMsg{},
		modeTimers:  timersMsg{},
		modeSecrets: secretsMsg{},
		modeStats:   statsMsg{entries: []podman.StatEntry{{Name: "web", CPU: "1.00%"}}},
		modeUpdates: updatesMsg{entries: []podman.AutoUpdateEntry{{Unit: "demo-web.service", Updated: "pending"}}},
	}
	for k, md := range asyncScreenKeys {
		m := press(sampleModel(t, 120, 34), k)
		m = press(m, "esc") // back to the list before the data lands
		model, _ := m.Update(late[md])
		m = model.(Model)
		if m.mode != modeList {
			t.Errorf("%q: late result pulled the user back to %v", k, m.mode)
		}
		if m.busy {
			t.Errorf("%q: the spinner of a finished load must stop", k)
		}
		if !m.screenLoaded(md) {
			t.Errorf("%q: late data must be kept for the next visit", k)
		}
	}
	// Leaving to another screen: the other screen's spinner is untouched.
	m := press(sampleModel(t, 120, 34), "g")
	m = press(m, "K") // storage still loading, secrets now loading
	model, _ := m.Update(storageMsg{})
	m = model.(Model)
	if m.mode != modeSecrets || !m.busy {
		t.Errorf("storage result must not end the secrets load: mode=%v busy=%v", m.mode, m.busy)
	}
}

// TestScreenLoadErrorStaysOnScreen: a failed first load keeps the screen,
// says what failed and how to retry, instead of bouncing to the list.
func TestScreenLoadErrorStaysOnScreen(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "g")
	model, _ := m.Update(storageMsg{err: errors.New("podman not found")})
	m = model.(Model)
	if m.mode != modeStorage || m.busy {
		t.Fatalf("mode=%v busy=%v", m.mode, m.busy)
	}
	v := plainView(m)
	if !strings.Contains(v, "could not load: podman not found") || !strings.Contains(v, "r retry") {
		t.Errorf("error state must name the failure and the retry key:\n%s", v)
	}
	if v := plainView(m); strings.Contains(v, "Images 0") || !strings.Contains(v, "Images") {
		t.Errorf("sub-tabs keep their names but drop counts of nothing:\n%s", v)
	}
	m = press(m, "r")
	if !strings.Contains(plainView(m), "loading") || !m.busy {
		t.Error("r must retry the load")
	}
	// Secrets refreshes with ctrl+r (r rotates), and says so.
	s := press(sampleModel(t, 120, 34), "K")
	model, _ = s.Update(secretsMsg{err: errors.New("boom")})
	if v := plainView(model.(Model)); !strings.Contains(v, "ctrl+r retry") {
		t.Errorf("secrets must offer ctrl+r:\n%s", v)
	}
}

// TestRevisitShowsCachedScreen: reopening a screen shows its last data at
// once while the refresh runs, instead of a blank loading state.
func TestRevisitShowsCachedScreen(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "u")
	model, _ := m.Update(updatesMsg{entries: []podman.AutoUpdateEntry{{Container: "systemd-demo-web", Unit: "demo-web.service", Updated: "pending"}}})
	m = press(model.(Model), "esc")
	m = press(m, "u")
	v := plainView(m)
	if strings.Contains(v, "loading…") || !strings.Contains(v, "demo-web") {
		t.Errorf("revisit must show the cached rows while refreshing:\n%s", v)
	}
}

// TestScreenResultFromOldSessionDropped: data fetched for the previous
// target never lands after a session switch, and the switch itself drops
// every screen's cached data and returns to the new target's list.
func TestScreenResultFromOldSessionDropped(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "g")
	old := m.epoch
	model, _ := m.Update(storageMsg{})
	m = model.(Model)
	m.setSession(remote.Runner{As: "svc"}, false, "")
	if m.mode != modeList || m.screenLoaded(modeStorage) || m.busy {
		t.Fatalf("session switch must reset screens: mode=%v loaded=%v busy=%v", m.mode, m.screenLoaded(modeStorage), m.busy)
	}
	m = press(m, "g")
	model, _ = m.Update(sessionMsg{epoch: old, msg: storageMsg{}})
	m = model.(Model)
	if m.screenLoaded(modeStorage) || !m.busy {
		t.Error("a previous session's result must be dropped")
	}
	model, _ = m.Update(sessionMsg{epoch: m.epoch, msg: storageMsg{}})
	if !model.(Model).screenLoaded(modeStorage) {
		t.Error("the current session's result must apply")
	}
}

// TestEnterScreenStopsLiveStreams: leaving the journal or the events view
// for another screen (palette, nav key) ends its follow process.
func TestEnterScreenStopsLiveStreams(t *testing.T) {
	stopped := 0
	m := sampleModel(t, 120, 34)
	m.mode, m.tab = modeDetail, tabJournal
	m.sess = &logSession{stop: func() { stopped++ }, unit: "demo-web.service"}
	m.enterScreen(modeStorage)
	if m.sess != nil || stopped != 1 {
		t.Errorf("journal follow must stop when a screen opens: sess=%v stopped=%d", m.sess != nil, stopped)
	}
	m.mode = modeEvents
	m.eventSess = &logSession{stop: func() { stopped++ }, unit: "events"}
	m.enterScreen(modeSecrets)
	if m.eventSess != nil || stopped != 2 {
		t.Errorf("events follow must stop when a screen opens: sess=%v stopped=%d", m.eventSess != nil, stopped)
	}
}

// TestActionThenReloadIsSequenced: a screen action and the reload after it
// run one after the other (not concurrently), so the reloaded list shows
// the action's result.
func TestActionThenReloadIsSequenced(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "K")
	model, _ := m.Update(secretsMsg{data: secretsData{list: []podman.SecretEntry{{Name: "db-pass"}}}})
	m = model.(Model)
	var order []string
	action := m.actionCmd("remove secret db-pass", func(context.Context) (string, error) {
		order = append(order, "action")
		return "", nil
	})
	reload := func(context.Context) tea.Msg { order = append(order, "reload"); return secretsMsg{} }
	msg := m.actionThenReload(action, reload)()
	if got := fmt.Sprintf("%T", msg); got != "tea.sequenceMsg" {
		t.Fatalf("action and reload must be sequenced, got %s", got)
	}
	for _, c := range reflect.ValueOf(msg).Convert(reflect.TypeOf([]tea.Cmd(nil))).Interface().([]tea.Cmd) {
		c()
	}
	if strings.Join(order, ",") != "action,reload" {
		t.Errorf("order = %v, want the action before the reload", order)
	}
}

// TestReopenWithinFreshnessUsesCache: going back to a screen whose data is
// recent shows it at once, runs nothing, and says how old it is.
func TestReopenWithinFreshnessUsesCache(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "g")
	model, _ := m.Update(storageMsg{data: storageData{df: []podman.DfRow{{Type: "Images", Total: 1}}}})
	m = press(model.(Model), "esc")
	model, cmd, _ := m.storageKeys(pressKey("g"))
	m = model.(Model)
	if cmd != nil || m.busy || m.scrLoading != modeList {
		t.Errorf("fresh data must not reload: cmd=%v busy=%v loading=%v", cmd != nil, m.busy, m.scrLoading)
	}
	if !strings.Contains(m.statusLine, "data from") || !strings.Contains(m.statusLine, "r refreshes") {
		t.Errorf("the status must give the data's age and the refresh key: %q", m.statusLine)
	}
	// Past its freshness the same key reloads.
	m = press(m, "esc")
	m.scrLoadedAt[modeStorage] = time.Now().Add(-time.Minute)
	if _, cmd, _ := m.storageKeys(pressKey("g")); cmd == nil {
		t.Error("stale data must reload")
	}
	// Updates stay fresh for minutes: a dry run asks every registry.
	if screenTTL(modeUpdates) < time.Minute || screenTTL(modeStats) != 0 {
		t.Errorf("ttl updates=%v stats=%v", screenTTL(modeUpdates), screenTTL(modeStats))
	}
}

// TestLeavingCancelsTheLoad: a load the user walks away from is cancelled
// (its commands killed) and its late result never lands.
func TestLeavingCancelsTheLoad(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "esc")
	m.enterScreen(modeStorage)
	started := make(chan context.Context, 1)
	cmd := m.loadScreen(modeStorage, "system df", func(ctx context.Context) tea.Msg {
		started <- ctx
		<-ctx.Done()
		return storageMsg{err: ctx.Err()}
	})
	seq := m.scrSeq
	m = press(m, "esc") // leave before it lands
	if m.busy || m.scrLoading != modeList {
		t.Errorf("leaving must end the load: busy=%v loading=%v", m.busy, m.scrLoading)
	}
	done := make(chan tea.Msg, 1)
	go func() {
		for _, c := range reflect.ValueOf(cmd()).Convert(reflect.TypeOf([]tea.Cmd(nil))).Interface().([]tea.Cmd) {
			if msg := c(); msg != nil {
				if sm, ok := msg.(sessionMsg); ok {
					done <- sm.msg
				}
			}
		}
	}()
	select {
	case msg := <-done:
		lm, ok := msg.(loadMsg)
		if !ok || lm.seq != seq {
			t.Fatalf("got %T %+v", msg, msg)
		}
		model, _ := m.Update(lm)
		if mm := model.(Model); mm.mode != modeList || strings.Contains(mm.statusLine, "canceled") {
			t.Errorf("a cancelled load must land silently: mode=%v status=%q", mm.mode, mm.statusLine)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("leaving the screen must cancel its load")
	}
	if ctx := <-started; ctx.Err() == nil {
		t.Error("the load's context must be cancelled")
	}
}

// TestNewerLoadReplacesOlder: only the newest load of a screen lands.
func TestNewerLoadReplacesOlder(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "g")
	old := m.scrSeq
	m = press(m, "r")
	model, _ := m.Update(loadMsg{seq: old, msg: storageMsg{err: errors.New("stale")}})
	if mm := model.(Model); !mm.busy || strings.Contains(mm.statusLine, "stale") {
		t.Errorf("a replaced load must be dropped: busy=%v status=%q", mm.busy, mm.statusLine)
	}
	model, _ = m.Update(loadMsg{seq: m.scrSeq, msg: storageMsg{}})
	if mm := model.(Model); mm.busy || !mm.screenLoaded(modeStorage) {
		t.Error("the newest load must land")
	}
}

// TestStatsSamplerOneAtATime: the poll tick never stacks podman stats
// runs, and host info is read once, not on every sample.
func TestStatsSamplerOneAtATime(t *testing.T) {
	m := press(sampleModel(t, 120, 34), "o")
	model, _ := m.Update(statsMsg{entries: []podman.StatEntry{{Name: "web"}}, host: podman.HostInfo{CPUs: 4}})
	m = model.(Model)
	model, _ = m.Update(tickMsg{})
	m = model.(Model)
	if !m.statsInFlight {
		t.Fatal("the tick must start a sample")
	}
	m.pollCount, m.refreshInFlight = 0, true // isolate the stats sampler
	before := m.statsInFlight
	model, _ = m.Update(tickMsg{})
	if mm := model.(Model); !before || !mm.statsInFlight {
		t.Error("a sample in flight must block the next one")
	}
	model, _ = m.Update(statsMsg{entries: []podman.StatEntry{{Name: "web"}}})
	if model.(Model).statsInFlight {
		t.Error("a landed sample frees the sampler")
	}
	if m.stats.host.CPUs == 0 {
		t.Fatal("host info must be kept from the first sample")
	}
}

// TestLoadersReadInParallel runs every screen loader against a remote
// whose each command takes 300ms: independent reads must overlap (and
// the race detector checks they share nothing unsafely).
func TestLoadersReadInParallel(t *testing.T) {
	script := "#!/bin/sh\nsleep 0.3\n" +
		"case \"$*\" in\n" +
		"*'system df --verbose'*) printf 'Local Volumes space usage:\\n\\nVOLUME NAME LINKS SIZE\\ndata 1 2MB\\n' ;;\n" +
		"*'system df'*) echo '[{\"Type\":\"Images\",\"Total\":1}]' ;;\n" +
		"*'podman stats'*) echo '[{\"name\":\"web\",\"cpu_percent\":\"1.00%\"}]' ;;\n" +
		"*'podman info'*) echo '{\"host\":{\"cpus\":4}}' ;;\n" +
		"*'--version'*) echo 'podman version 5.6.2' ;;\n" +
		"*) echo '[]' ;;\n" +
		"esac\n"
	m := sampleModel(t, 120, 34)
	m.applySSH("user@remote")
	useFakeSSH(&m, fakeSSHBin(t, script))
	m.units = nil // no unit sources to read: only podman timing counts
	for name, c := range map[string]struct {
		load screenLoad
		max  time.Duration // sequential would take calls × 300ms
	}{
		"storage": {m.storageLoad(), 1100 * time.Millisecond},  // 5 calls
		"stats":   {m.statsLoad(true), 750 * time.Millisecond}, // 3 calls
	} {
		start := time.Now()
		msg := c.load(context.Background())
		if el := time.Since(start); el > c.max {
			t.Errorf("%s took %v: its reads must run side by side", name, el)
		}
		if sm, ok := msg.(screenMsg); !ok || sm.screenErr() != nil {
			t.Errorf("%s: %T %v", name, msg, sm.screenErr())
		}
	}
}
