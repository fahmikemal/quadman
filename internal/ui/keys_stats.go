package ui

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
)

// statsMsg carries rendered `podman stats` output to the stats screen.
type statsMsg struct {
	content string
	err     error
}

// statsCmd fetches resource usage for every container (running or stopped;
// stopped rows read zero, so the screen is never misleadingly empty).
func statsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := podman.Stats(context.Background())
		if err != nil {
			return statsMsg{err: err}
		}
		return statsMsg{content: renderStats(entries)}
	}
}

// renderStats formats stat entries as an aligned table. It is a pure
// function so tests can pin the layout without podman.
func renderStats(entries []podman.StatEntry) string {
	if len(entries) == 0 {
		return "No containers (podman stats --all is empty).\n"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tCPU%\tMEM\tMEM%\tNET\tBLOCK\tPIDS")
	for _, e := range entries {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Name, e.CPU, e.Mem, e.MemPerc, e.Net, e.Block, e.PIDs)
	}
	_ = w.Flush()
	return b.String()
}

// statsOpenKeys opens the resource screen (o). Read-only by nature: no
// readonly/remote gates, like storage (g) and events (w).
func (m Model) statsOpenKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "o" {
		return m, nil, false
	}
	return m, tea.Batch(m.setBusy("stats"), statsCmd()), true
}

// modeStatsKeys scrolls (viewport), refreshes, or leaves the stats screen.
func (m Model) modeStatsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeStats {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.resize()
		return m, nil, true
	case "r":
		return m, tea.Batch(m.setBusy("stats"), statsCmd()), true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true
}
