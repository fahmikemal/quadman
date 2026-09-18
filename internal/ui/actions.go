package ui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// instantiateUnit starts an instance of a template unit
// (web@.container -> web@prod.service via systemd's template mechanism).
func (m Model) instantiateUnit(instance string) (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	base := strings.TrimSuffix(u.Name, "@")
	instanceUnit := base + "@" + instance + ".service"
	sys := m.sys
	return m, tea.Batch(m.setBusy("start "+instanceUnit),
		actionCmd("start "+instanceUnit, func(ctx context.Context) (string, error) {
			return sys.UnitAction(ctx, "start", instanceUnit)
		}))
}

// deleteUnit removes the quadlet file (confirm-armed by the caller).
// It prefers `podman quadlet rm` (application-aware, --force stops running
// units) and falls back to stopping the unit, deleting the file, and
// reloading the generator.
func (m Model) deleteUnit(u quadlet.Unit) (tea.Model, tea.Cmd) {
	sys := m.sys
	return m, tea.Batch(m.setBusy("delete "+u.Name),
		actionCmdHint("delete "+u.Name, "file removed; the unit stays until the next boot if it was enabled", func(ctx context.Context) (string, error) {
			if podman.Available() {
				if err := quadletRemove(ctx, u.Path); err == nil {
					return "", nil
				}
			}
			if _, err := sys.UnitAction(ctx, "stop", u.UnitName); err != nil {
				// Not running or already gone — deleting the file still applies.
				_ = err
			}
			if err := os.Remove(u.Path); err != nil {
				return "", fmt.Errorf("remove %s: %w", u.Path, err)
			}
			_, err := sys.DaemonReload(ctx)
			return "", err
		}))
}

// quadletRemove shells out to `podman quadlet rm --force`.
func quadletRemove(ctx context.Context, path string) error {
	return podman.QuadletRm(ctx, path)
}

// installBundle installs a .quadlets bundle via podman quadlet install.
func (m Model) installBundle(u quadlet.Unit) (tea.Model, tea.Cmd) {
	if m.refuseRemoteWrite() {
		return m, nil
	}
	return m, tea.Batch(m.setBusy("install "+u.Name),
		actionCmdHint("install "+u.Name, "installed units appear after the refresh", func(ctx context.Context) (string, error) {
			if !podman.Available() {
				return "", fmt.Errorf("podman is required to install bundles")
			}
			if err := podman.QuadletInstall(ctx, u.Path); err != nil {
				return "", err
			}
			_, err := m.sys.DaemonReload(ctx)
			return "", err
		}))
}

// runPending executes an armed y/N-confirmed action.
func (m Model) runPending(p *pendingAction) (tea.Model, tea.Cmd) {
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
		return m, tea.Batch(m.setBusy("enable at boot "+p.unit.UnitName),
			actionCmdHint("enable at boot "+p.unit.UnitName, "starts on login from now on", func(ctx context.Context) (string, error) {
				changed, err := quadlet.EnsureBootTarget(p.unit.Path, "default.target")
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
			actionCmdHint("disable at boot "+p.unit.UnitName, "still running now", func(ctx context.Context) (string, error) {
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
	case "delete":
		return m.deleteUnit(p.unit)
	default: // "stop"
		hint := ""
		if p.unit.Kind == quadlet.KindContainer {
			hint = "container removed; state lives in volumes"
		}
		return m, tea.Batch(m.setBusy("stop "+p.unit.UnitName),
			actionCmdHint("stop "+p.unit.UnitName, hint, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "stop", p.unit.UnitName)
			}))
	}
}
