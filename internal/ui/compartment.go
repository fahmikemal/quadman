package ui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/compartment"
	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
)

// applyCompartment re-points every CLI client at the target OS user's
// session via non-interactive sudo, then aims discovery and staleness at
// that user's own directories. It returns false (with a status explaining
// why) when sudo cannot reach the user, so the TUI keeps showing the
// operator's own session instead of a half-switched one.
func (m *Model) applyCompartment(username string) bool {
	if err := compartment.ProbeSudo(context.Background(), username); err != nil {
		m.setStatus("compartment "+username+": "+err.Error(), true)
		return false
	}
	c, err := compartment.Resolve(username)
	if err != nil {
		m.setStatus("compartment: "+err.Error(), true)
		return false
	}
	if !c.RuntimeReady() {
		m.setStatus(fmt.Sprintf("compartment %s: no runtime dir %s (linger the user or log in once)", username, c.RuntimeDir), true)
		return false
	}
	r := c.Runner()
	m.ssh = r
	m.sys.Remote = r
	m.lc.Remote = r
	podman.DefaultRunner = r
	podlet.DefaultRunner = r
	m.sys.User = true
	m.sys.GenDir = c.GeneratorDir()
	m.comp = c
	m.compOn = true
	m.setStatus("compartment: now managing "+username+" (file edits disabled, like SSH mode)", false)
	return true
}

// leaveCompartment returns all clients to the operator's own session.
func (m *Model) leaveCompartment() {
	m.ssh = remote.Runner{}
	m.sys.Remote = m.ssh
	m.lc.Remote = m.ssh
	podman.DefaultRunner = m.ssh
	podlet.DefaultRunner = m.ssh
	m.sys.GenDir = ""
	m.compOn = false
	m.setStatus("compartment: back to own session", false)
}

// compDirs returns the Quadlet search path of the active compartment.
func (m Model) compDirs() []string {
	return quadlet.SearchDirsFor(m.comp.Home, m.comp.UID, m.comp.RuntimeDir)
}

// compartmentPaletteActions builds one action per configured compartment
// plus a way back to the operator's own session.
func (m Model) compartmentPaletteActions() []paletteAction {
	var actions []paletteAction
	for _, name := range m.compList {
		if m.compOn && m.comp.User == name {
			continue
		}
		username := name
		actions = append(actions, paletteAction{
			id:          "compartment-" + username,
			title:       "Use Compartment " + username,
			shortcut:    "",
			category:    "Compartments",
			description: "Manage " + username + "'s Quadlet units via sudo (needs NOPASSWD or root)",
			enabled: func(m Model) (bool, string) {
				if m.readonly {
					return false, "readonly"
				}
				if m.ssh.IsRemote() {
					return false, "unavailable over SSH"
				}
				return true, ""
			},
			run: func(m Model) (tea.Model, tea.Cmd, bool) {
				if m.applyCompartment(username) {
					return m, m.refresh(enrichFull), true
				}
				return m, nil, true
			},
		})
	}
	if m.compOn {
		actions = append(actions, paletteAction{
			id:          "compartment-own",
			title:       "Use Own Session",
			shortcut:    "",
			category:    "Compartments",
			description: "Leave the compartment, back to your own units",
			enabled:     func(m Model) (bool, string) { return true, "" },
			run: func(m Model) (tea.Model, tea.Cmd, bool) {
				m.leaveCompartment()
				return m, m.refresh(enrichFull), true
			},
		})
	}
	return actions
}
