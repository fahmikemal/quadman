package ui

import (
	"context"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
)

// selectedName returns the selected unit's name, or "" when none.
func (m Model) selectedName() string {
	if u, ok := m.selected(); ok {
		return u.Name
	}
	return ""
}

// cycleTab moves the detail view one tab in the given direction (+1/-1),
// loading each tab's content lazily.
func (m Model) cycleTab(dir int) (tea.Model, tea.Cmd) {
	m.tab = (m.tab + dir + 4) % 4
	m.clearSearch()
	switch m.tab {
	case tabSource:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		content, err := os.ReadFile(u.Path)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		m.viewport.SetContent(withDropins(u, content))
		m.viewport.GotoTop()
		return m, nil
	case tabStatus:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("status "+u.UnitName), func() tea.Msg {
			text, _ := sys.StatusText(context.Background(), u.UnitName)
			return statusMsg{content: text}
		})
	case tabJournal:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.sess == nil || m.sess.unit != u.UnitName {
			return m.startLogs(u)
		}
		return m, nil
	case tabInspect:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if u.Kind != quadlet.KindContainer {
			m.viewport.SetContent("podman inspect only applies to container units\n")
			m.viewport.GotoTop()
			return m, nil
		}
		container := "systemd-" + u.Name
		return m, tea.Batch(m.setBusy("inspect "+container), func() tea.Msg {
			text, err := podman.Inspect(context.Background(), container)
			if err != nil {
				text = err.Error()
			}
			return inspectMsg{content: text}
		})
	}
	return m, nil
}

// tabBar renders the detail view's tab strip with the active tab highlighted
// and the unit name next to it.
func (m Model) tabBar(unit string) string {
	tabs := []string{"source", "status", "journal", "inspect"}
	var b strings.Builder
	for i, t := range tabs {
		if i == m.tab {
			b.WriteString(tabActiveStyle.Render(" " + t + " "))
		} else {
			b.WriteString(tabInactiveStyle.Render(" " + t + " "))
		}
		if i < len(tabs)-1 {
			b.WriteString(" ")
		}
	}
	if unit != "" {
		b.WriteString("  " + helpStyle.Render(unit))
	}
	if m.tab == tabJournal {
		state := "live"
		if m.sess == nil {
			state = "snapshot"
		} else if !m.following {
			state = "paused"
		}
		b.WriteString(helpStyle.Render("  (" + state + ")"))
	}
	return b.String()
}
