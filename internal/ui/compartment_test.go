package ui

import (
	"os"
	"os/user"
	"strings"
	"testing"
)

// compSudo fakes passwordless sudo for compartment tests.
func compSudo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/sudo", []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestApplyCompartmentSelf(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	compSudo(t)
	m := New()
	if !m.applyCompartment(u.Username) {
		t.Fatalf("self compartment failed: %q", m.statusLine)
	}
	mm := m
	if !mm.compOn || mm.comp.User != u.Username {
		t.Errorf("compOn=%v comp=%+v", mm.compOn, mm.comp)
	}
	if mm.ssh.As != u.Username {
		t.Errorf("runner As = %q", mm.ssh.As)
	}
	if mm.sys.GenDir == "" || !strings.Contains(mm.sys.GenDir, u.Uid) {
		t.Errorf("GenDir = %q", mm.sys.GenDir)
	}
	dirs := mm.searchDirs()
	if len(dirs) == 0 || !strings.Contains(dirs[0], u.Uid) && !strings.Contains(dirs[1], u.HomeDir) {
		t.Errorf("comp dirs = %v", dirs)
	}
	if !strings.Contains(mm.View().Content, "["+u.Username+"]") {
		t.Error("title must show the compartment user")
	}
	resetRunners()
}

func TestApplyCompartmentProbeFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/sudo", []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	m := New()
	if m.applyCompartment("svc") {
		t.Error("failing sudo must not switch")
	}
	if m.compOn {
		t.Error("compOn must stay false")
	}
	if !strings.Contains(m.statusLine, "sudo -n -u svc") {
		t.Errorf("status = %q", m.statusLine)
	}
}

func TestApplyCompartmentUnknownUser(t *testing.T) {
	compSudo(t)
	m := New()
	if m.applyCompartment("quadman-nosuch-user-xyz") {
		t.Error("unknown user must not switch")
	}
}

func TestLeaveCompartment(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	compSudo(t)
	m := New()
	if !m.applyCompartment(u.Username) {
		t.Skipf("self compartment unavailable: %q", m.statusLine)
	}
	m.leaveCompartment()
	if m.compOn || m.ssh.As != "" || m.sys.GenDir != "" {
		t.Errorf("leave must reset: compOn=%v As=%q GenDir=%q", m.compOn, m.ssh.As, m.sys.GenDir)
	}
	resetRunners()
}

func TestCompartmentPaletteActions(t *testing.T) {
	m := New()
	m.compList = []string{"svc-a", "svc-b"}
	var ids []string
	for _, a := range m.compartmentPaletteActions() {
		ids = append(ids, a.id)
	}
	if len(ids) != 2 || ids[0] != "compartment-svc-a" {
		t.Errorf("actions = %v", ids)
	}
	m.compOn = true
	m.comp.User = "svc-a"
	ids = nil
	for _, a := range m.compartmentPaletteActions() {
		ids = append(ids, a.id)
	}
	if len(ids) != 2 || ids[0] != "compartment-svc-b" || ids[1] != "compartment-own" {
		t.Errorf("switched actions = %v", ids)
	}
}

func TestCompartmentPaletteDisabledInSystemMode(t *testing.T) {
	m := New()
	m.compList = []string{"svc-a"}
	m.system = true
	for _, a := range m.compartmentPaletteActions() {
		if a.id != "compartment-svc-a" {
			continue
		}
		ok, reason := a.enabled(m)
		if ok || reason != "unavailable in system mode" {
			t.Errorf("system mode must disable compartment switch, ok=%v reason=%q", ok, reason)
		}
	}
}

func TestIsolatedReason(t *testing.T) {
	m := New()
	if bad, _ := m.isolatedReason(); bad {
		t.Error("local session is not isolated")
	}
}
