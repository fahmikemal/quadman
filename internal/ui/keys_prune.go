package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
)

// maxPruneNames caps how many inactive unit names the prune confirmation
// lists; beyond that it reports the remainder as a count.
const maxPruneNames = 8

// inactiveQuadletUnits returns the unit names whose runtime state is known
// and not active: their stopped containers are exactly what `podman system
// prune` would delete. Units with no state yet (pre-refresh) are skipped
// rather than counted, so the warning never cries wolf on a cold start.
func (m Model) inactiveQuadletUnits() []string {
	var out []string
	for _, u := range m.units {
		if u.UnitName == "" {
			continue
		}
		st, ok := m.status[u.UnitName]
		if !ok {
			continue
		}
		if st.ActiveState != "active" {
			out = append(out, u.UnitName)
		}
	}
	sort.Strings(out)
	return out
}

// prunePrompt renders the armed confirmation line for P, naming the
// inactive units at risk.
func prunePrompt(inactive []string) string {
	if len(inactive) == 0 {
		return "system prune: remove stopped containers + unused images/networks? [y/N]"
	}
	shown := inactive
	rest := ""
	if len(inactive) > maxPruneNames {
		shown = inactive[:maxPruneNames]
		rest = fmt.Sprintf(" + %d more", len(inactive)-maxPruneNames)
	}
	return fmt.Sprintf("system prune removes containers of %d inactive unit(s) (%s%s)? [y/N]",
		len(inactive), strings.Join(shown, ", "), rest)
}

// pruneKeys arms the system-prune confirmation (P, host scope). Read-only
// and remote sessions are refused: prune is destructive and remote runs
// cannot show the reclaimed-space report interactively.
func (m Model) pruneKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "P" {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	if m.ssh.IsRemote() {
		m.setStatus("prune unavailable over SSH (run podman system prune on the host)", true)
		return m, nil, true
	}
	// Local sessions need the CLI on PATH; compartment sessions resolve it
	// through sudo on the target, so only gate the local case here. Prune
	// stays available in compartments: like stop, it only touches the
	// target user's own storage.
	if !m.compOn && !podman.Available() {
		m.setStatus("podman not found — prune needs the podman CLI", true)
		return m, nil, true
	}
	m.pending = &pendingAction{verb: "prune"}
	m.setStatus(prunePrompt(m.inactiveQuadletUnits()), false)
	return m, nil, true
}

// pruneCmd runs the confirmed prune and reports reclaimed space.
func pruneCmd() tea.Cmd {
	return actionCmdHint("system prune", "stopped containers and unused data removed", func(ctx context.Context) (string, error) {
		return podman.SystemPrune(ctx)
	})
}
