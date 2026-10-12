package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/compartment"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
)

// TestPolicyMatrix pins the capability policy for every session kind: it
// is the one table both the key handlers and the palette consult.
func TestPolicyMatrix(t *testing.T) {
	local := New()
	readonly := New()
	readonly.readonly = true
	ssh := New()
	ssh.applySSH("u@h")
	comp := New()
	comp.setSession(remote.Runner{As: "svc"}, false, "")
	comp.compOn, comp.comp = true, compartment.Compartment{User: "svc"}
	served := New()
	served.clientInfo, served.noEditor = "guest@10.0.0.9", true
	system := New()
	system.system = true

	type row struct {
		name string
		m    Model
		want map[capability]bool
	}
	all := func(v bool) map[capability]bool {
		return map[capability]bool{capMutate: v, capTargetFiles: v, capLocalFiles: v, capTerminal: v, capSecretValue: v, capSwitchSession: v}
	}
	rows := []row{
		{"local", local, all(true)},
		{"readonly", readonly, all(false)},
		{"ssh", ssh, map[capability]bool{capMutate: true, capTargetFiles: false, capLocalFiles: true, capTerminal: false, capSecretValue: true, capSwitchSession: false}},
		{"compartment", comp, map[capability]bool{capMutate: true, capTargetFiles: false, capLocalFiles: true, capTerminal: true, capSecretValue: true, capSwitchSession: true}},
		{"served", served, map[capability]bool{capMutate: true, capTargetFiles: true, capLocalFiles: false, capTerminal: false, capSecretValue: false, capSwitchSession: false}},
		{"system", system, map[capability]bool{capMutate: true, capTargetFiles: true, capLocalFiles: true, capTerminal: true, capSecretValue: true, capSwitchSession: false}},
	}
	for _, r := range rows {
		for c, want := range r.want {
			if got, why := r.m.allowed(c); got != want {
				t.Errorf("%s: allowed(%b) = %v (%q), want %v", r.name, c, got, why, want)
			}
		}
	}
	if ok, _ := local.allowed(0); !ok {
		t.Error("an action without capabilities always runs")
	}
}

// TestServedCompartmentStaysPinned: a client of `quadman --as svc serve`
// must never reach the operator's own session or another compartment.
func TestServedCompartmentStaysPinned(t *testing.T) {
	m := New()
	m.setSession(remote.Runner{As: "svc-web"}, false, "")
	m.compOn, m.comp = true, compartment.Compartment{User: "svc-web"}
	m.compList = []string{"svc-db"}
	m.clientInfo, m.noEditor = "guest@10.0.0.9", true
	for _, a := range m.compartmentPaletteActions() {
		if ok, _ := a.enabled(m); ok {
			t.Errorf("%q must be disabled in a served session", a.title)
		}
		mm, _, _ := a.run(m)
		if got := mm.(Model); got.ssh.As != "svc-web" || !got.compOn {
			t.Errorf("%q switched a served session away from its compartment", a.title)
		}
	}
}

// TestReadonlyBundleInstallRefused closes the readonly bypass: I used to
// arm the install confirmation without any readonly check.
func TestReadonlyBundleInstallRefused(t *testing.T) {
	m := New()
	units := []quadlet.Unit{{Name: "stack", Kind: quadlet.KindQuadlets, Path: "/q/stack.quadlets"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{""}, lingerOK: true})
	m = model.(Model)
	m.readonly = true
	model, _ = m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if mm := model.(Model); mm.pending != nil || !strings.Contains(mm.statusLine, "readonly") {
		t.Errorf("readonly must refuse bundle install: pending=%v status=%q", mm.pending, mm.statusLine)
	}
}

// TestRepeatKeepsConfirmation: repeating a typed-confirmed action asks for
// the same typed name; a bare y never re-runs a delete.
func TestRepeatKeepsConfirmation(t *testing.T) {
	m := New()
	ran := 0
	m.actions = []actionRecord{{
		desc: "remove secret db-pass", ok: true, epoch: m.epoch,
		redo:  func(context.Context) (string, error) { ran++; return "", nil },
		guard: &confirmGuard{title: "Remove secret db-pass?", lines: []string{"remove db-pass"}, danger: true, typed: "db-pass"},
	}}
	m.enterScreen(modeRecent)
	m = press(m, ".")
	p := m.pending
	if p == nil || p.typed != "db-pass" || !p.danger {
		t.Fatalf("repeat must arm the original typed danger confirmation, got %+v", p)
	}
	m = press(m, "y")
	if m.pending == nil || ran != 0 {
		t.Error("y alone must not confirm a typed repeat")
	}
}

// TestRepeatRefusedAcrossSessions: work recorded in one session never
// replays after a switch to another.
func TestRepeatRefusedAcrossSessions(t *testing.T) {
	m := New()
	m.actions = []actionRecord{{
		desc: "stop web.service", ok: true, epoch: m.epoch,
		redo: func(context.Context) (string, error) { return "", nil },
	}}
	m.applySSH("u@h")
	m.enterScreen(modeRecent)
	m = press(m, ".")
	if m.pending != nil || !strings.Contains(m.statusLine, "another session") {
		t.Errorf("cross-session repeat must be refused: pending=%v status=%q", m.pending, m.statusLine)
	}
}

// TestActionRecordsGuard: an action built while a confirmation runs
// records that confirmation, so the actions screen can repeat it safely.
func TestActionRecordsGuard(t *testing.T) {
	m := New()
	p := &pendingAction{verb: "remove", title: "Remove volume data?", danger: true, typed: "data",
		run: func(m Model) (tea.Model, tea.Cmd) {
			return m, m.actionCmd("remove volume data", func(context.Context) (string, error) { return "", nil })
		}}
	model, cmd := m.runPending(p)
	if model.(Model).confirming != nil {
		t.Error("the confirmation scope must end with runPending")
	}
	msg, ok := cmd().(actionMsg)
	if !ok || msg.guard == nil || msg.guard.typed != "data" || msg.epoch != m.epoch {
		t.Errorf("action must carry its guard and epoch: %+v", msg)
	}
}

func TestRunPendingUnknownVerbRunsNothing(t *testing.T) {
	m := New()
	model, cmd := m.runPending(&pendingAction{verb: "frobnicate", unit: quadlet.Unit{UnitName: "web.service"}})
	if cmd != nil || !strings.Contains(model.(Model).statusLine, "nothing ran") {
		t.Errorf("an unknown verb must run nothing: cmd=%v status=%q", cmd != nil, model.(Model).statusLine)
	}
}

func TestSystemdEscape(t *testing.T) {
	for in, want := range map[string]string{
		"prod":     "prod",
		"my app":   `my\x20app`,
		"a-b":      `a\x2db`,
		"/srv/web": `-srv-web`,
		".hidden":  `\x2ehidden`,
		"v1.2:x_y": "v1.2:x_y",
	} {
		if got := systemdEscape(in); got != want {
			t.Errorf("systemdEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestContainerNameResolution(t *testing.T) {
	m := New()
	u := quadlet.Unit{Name: "web", Kind: quadlet.KindContainer, UnitName: "web.service"}
	if got := m.containerName(u); got != "systemd-web" {
		t.Errorf("default = %q", got)
	}
	m.ctrOverride = map[string]string{"web.service": "web-main"}
	if got := m.containerName(u); got != "web-main" {
		t.Errorf("ContainerName= = %q", got)
	}
	m.ctrNames = map[string]string{"web.service": "web-live"}
	if got := m.containerName(u); got != "web-live" {
		t.Errorf("running container label must win, got %q", got)
	}
	m.health = map[string]string{"web-live": "unhealthy"}
	if m.stateOf(u).word != "unhealthy" {
		t.Error("health must follow the resolved container name")
	}
}
