package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// instantiateUnit starts an instance of a template unit
// (web@.container -> web@prod.service via systemd's template mechanism).
// The instance is escaped the way systemd-escape does, so "my app" or a
// path becomes a valid unit name instead of a confusing systemctl error.
func (m Model) instantiateUnit(instance string) (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	tmpl := u.UnitName
	if tmpl == "" || !strings.HasSuffix(tmpl, "@.service") {
		tmpl = quadlet.UnitFileName(u.Name, u.Kind)
	}
	instanceUnit := strings.TrimSuffix(tmpl, "@.service") + "@" + systemdEscape(instance) + ".service"
	sys := m.sys
	return m, tea.Batch(m.setBusy("start "+instanceUnit),
		m.actionCmd("start "+instanceUnit, func(ctx context.Context) (string, error) {
			return sys.UnitAction(ctx, "start", instanceUnit)
		}))
}

// systemdEscape escapes s for use in a unit name like `systemd-escape`:
// "/" becomes "-", ASCII letters, digits, ":", "_" and "." stay (a leading
// "." is escaped), and every other byte becomes a \xNN escape.
func systemdEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '/':
			b.WriteByte('-')
		case c == '.' && i == 0:
			fmt.Fprintf(&b, `\x%02x`, c)
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == ':', c == '_', c == '.':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	return b.String()
}

// deleteUnit removes the quadlet file (confirm-armed by the caller with
// the typed unit name). When podman reports this very file as an installed
// quadlet, `podman quadlet rm` removes it (keeping podman's application
// bookkeeping consistent); otherwise — or if that fails — the unit is
// stopped, the file removed, and the generator reloaded. Drop-in
// directories are kept: they may be shared with other units.
func (m Model) deleteUnit(u quadlet.Unit) (tea.Model, tea.Cmd) {
	sys, pm := m.sys, m.pm
	viaPodman := false
	if e, ok := m.podmanInfo[u.UnitName]; ok && e.Path == u.Path {
		viaPodman = true
	}
	name := filepath.Base(u.Path)
	isTemplate := strings.HasSuffix(u.Name, "@")
	return m, tea.Batch(m.setBusy("delete "+u.Name),
		m.actionCmdHint("delete "+u.Name, "file removed and generator reloaded", func(ctx context.Context) (string, error) {
			if viaPodman {
				if err := pm.QuadletRm(ctx, name); err == nil {
					if _, err := os.Stat(u.Path); errors.Is(err, os.ErrNotExist) {
						return "", nil
					}
				}
			}
			if u.UnitName != "" && !isTemplate {
				// Not running or already gone: deleting the file still applies.
				_, _ = sys.UnitAction(ctx, "stop", u.UnitName)
			}
			if err := os.Remove(u.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("remove %s: %w", u.Path, err)
			}
			_, err := sys.DaemonReload(ctx)
			return "", err
		}))
}

// installBundle installs a .quadlets bundle via `podman quadlet install`,
// through the session runner: podman runs as the session's user on the
// session's host and writes into that user's own quadlet directory.
func (m Model) installBundle(u quadlet.Unit) (tea.Model, tea.Cmd) {
	sys, pm := m.sys, m.pm
	return m, tea.Batch(m.setBusy("install "+u.Name),
		m.actionCmdHint("install "+u.Name, "installed units appear after the refresh", func(ctx context.Context) (string, error) {
			if !pm.Reachable(ctx) {
				return "", fmt.Errorf("podman is required to install bundles")
			}
			if err := pm.QuadletInstall(ctx, u.Path); err != nil {
				return "", err
			}
			_, err := sys.DaemonReload(ctx)
			return "", err
		}))
}

// guardOf is the confirmation level p asks for.
func guardOf(p *pendingAction) *confirmGuard {
	return &confirmGuard{title: p.title, lines: p.lines, keeps: p.keeps, danger: p.danger, typed: p.typed}
}

// runPending executes an armed, confirmed action. The commands it builds
// record the confirmation they passed (m.confirming), so a later repeat
// from the actions screen must pass the same one. Every built-in verb is
// listed explicitly: an unknown verb is a programming error and runs
// nothing, instead of falling through to a destructive default.
func (m Model) runPending(p *pendingAction) (tea.Model, tea.Cmd) {
	m.confirming = guardOf(p)
	mm, cmd := m.dispatchPending(p)
	if done, ok := mm.(Model); ok {
		done.confirming = nil
		mm = done
	}
	return mm, cmd
}

func (m Model) dispatchPending(p *pendingAction) (tea.Model, tea.Cmd) {
	if p.run != nil {
		return p.run(m)
	}
	sys := m.sys
	if verb, ok := strings.CutPrefix(p.verb, "bulk-"); ok {
		marked := m.markedUnits()
		if len(marked) == 0 {
			m.setStatus("no marked units", false)
			return m, nil
		}
		return m, tea.Batch(m.setBusy("bulk "+verb+" "+bulkNoun(marked)), m.bulkCmd(verb, marked))
	}
	switch p.verb {
	case "enable":
		target := m.bootTarget()
		return m, tea.Batch(m.setBusy("enable at boot "+p.unit.UnitName),
			m.actionCmdHint("enable at boot "+p.unit.UnitName, m.bootHint(), func(ctx context.Context) (string, error) {
				changed, err := quadlet.EnsureBootTarget(p.unit.Path, target)
				if err != nil {
					return "", err
				}
				if changed {
					if _, err := sys.DaemonReload(ctx); err != nil {
						return "", err
					}
				}
				return sys.UnitAction(ctx, "start", p.unit.UnitName)
			}))
	case "disable":
		return m, tea.Batch(m.setBusy("disable at boot "+p.unit.UnitName),
			m.actionCmdHint("disable at boot "+p.unit.UnitName, "still running now", func(ctx context.Context) (string, error) {
				changed, err := quadlet.RemoveBootTarget(p.unit.Path)
				if err != nil {
					return "", err
				}
				if changed {
					if _, err := sys.DaemonReload(ctx); err != nil {
						return "", err
					}
				}
				return "", nil
			}))
	case "install":
		return m.installBundle(p.unit)
	case "prune":
		return m, tea.Batch(m.setBusy("system prune"), m.pruneCmd())
	case "delete":
		return m.deleteUnit(p.unit)
	case "stop":
		hint := ""
		if p.unit.Kind == quadlet.KindContainer {
			hint = "container removed; state lives in volumes"
		}
		return m, tea.Batch(m.setBusy("stop "+p.unit.UnitName),
			m.actionCmdHint("stop "+p.unit.UnitName, hint, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "stop", p.unit.UnitName)
			}))
	}
	m.setStatus("internal error: no action for confirmed verb "+p.verb+" (nothing ran)", true)
	return m, nil
}

// bootHint says when an enabled unit starts: at boot for system units and
// lingering users, at login otherwise.
func (m Model) bootHint() string {
	switch {
	case m.system:
		return "starts at boot from now on"
	case m.lingerKnown && m.linger:
		return "starts at boot from now on (linger is on)"
	}
	return "starts at login from now on (enable linger with L to start at boot)"
}
