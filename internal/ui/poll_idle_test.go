package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestIdleScreenPollsSlowly: a minute without input, or an unfocused
// terminal, drops the poll cadence; the top bar says so.
func TestIdleScreenPollsSlowly(t *testing.T) {
	m := sampleModel(t, 120, 34)
	if m.idle() || m.pollEvery() != defaultPollInterval {
		t.Fatalf("a fresh screen polls at the normal cadence: idle=%v every=%v", m.idle(), m.pollEvery())
	}
	m.lastInput = time.Now().Add(-2 * idleAfter)
	if !m.idle() || m.pollEvery() != idlePollInterval {
		t.Errorf("unattended: idle=%v every=%v", m.idle(), m.pollEvery())
	}
	if top := plainView(m); !strings.Contains(top, idlePollInterval.String()+" idle") {
		t.Errorf("the top bar must show the slow cadence:\n%s", strings.SplitN(top, "\n", 2)[0])
	}
	// A slower configured interval is kept as is.
	m.pollInterval = time.Minute
	if m.pollEvery() != time.Minute {
		t.Errorf("configured 1m must not speed up: %v", m.pollEvery())
	}

	b := sampleModel(t, 120, 34)
	model, _ := b.Update(tea.BlurMsg{})
	if mm := model.(Model); !mm.idle() || mm.pollEvery() != idlePollInterval {
		t.Error("an unfocused terminal polls slowly at once")
	}
	if !b.View().ReportFocus {
		t.Error("the view must ask the terminal for focus reports")
	}
}

// TestWakeRefreshesAndRestartsTheChain: the first key (or focus) after idle
// refreshes immediately and replaces the slow tick chain.
func TestWakeRefreshesAndRestartsTheChain(t *testing.T) {
	for name, wake := range map[string]tea.Msg{"key": pressKey("down"), "focus": tea.FocusMsg{}} {
		m := sampleModel(t, 120, 34)
		m.lastInput = time.Now().Add(-2 * idleAfter)
		gen := m.tickGen
		model, cmd := m.Update(wake)
		mm := model.(Model)
		if cmd == nil || mm.tickGen != gen+1 || !mm.refreshInFlight {
			t.Errorf("%s: wake must refresh and restart polling: cmd=%v gen=%d inflight=%v", name, cmd != nil, mm.tickGen, mm.refreshInFlight)
		}
		if mm.idle() || mm.pollEvery() != defaultPollInterval {
			t.Errorf("%s: back to the normal cadence: %v", name, mm.pollEvery())
		}
		// A tick of the replaced slow chain does nothing.
		before := mm.pollCount
		model, cmd = mm.Update(tickMsg{gen: gen})
		if cmd != nil || model.(Model).pollCount != before {
			t.Errorf("%s: a stale chain's tick must be dropped", name)
		}
	}
	// Input on an attended screen changes nothing.
	m := sampleModel(t, 120, 34)
	model, _ := m.Update(pressKey("down"))
	if mm := model.(Model); mm.tickGen != m.tickGen || mm.refreshInFlight != m.refreshInFlight {
		t.Error("input on an attended screen must not restart polling")
	}
}

// TestLingerReadEveryFewPolls: linger is read on full refreshes, while
// unknown, and every lingerPollEvery polls — not on every tick; a refresh
// that skips it keeps what the model knows.
func TestLingerReadEveryFewPolls(t *testing.T) {
	m := sampleModel(t, 120, 34)
	if !m.lingerKnown || !m.linger {
		t.Fatal("sample starts with linger on")
	}
	if m.refreshReq(enrichNone).linger {
		t.Error("a known linger must not be re-read on a plain poll")
	}
	if !m.refreshReq(enrichFull).linger {
		t.Error("a full refresh re-reads linger")
	}
	model, _ := m.Update(refreshMsg{units: m.units, images: make([]string, len(m.units)), statuses: m.status, lingerSkipped: true})
	if mm := model.(Model); !mm.linger || !mm.lingerKnown {
		t.Error("a refresh that skipped linger must keep it")
	}
	m.lingerKnown = false
	if !m.refreshReq(enrichNone).linger {
		t.Error("an unknown linger is read on the next poll")
	}
	if lingerPollEvery*defaultPollInterval > time.Minute {
		t.Errorf("linger must still refresh within a minute: every %v", lingerPollEvery*defaultPollInterval)
	}
}
