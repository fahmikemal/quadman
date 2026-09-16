package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

func (m Model) View() tea.View {
	var b strings.Builder
	title := " quadman - rootless quadlet manager "
	if m.ssh.IsRemote() {
		title = " quadman @ " + m.ssh.Target + " "
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")

	switch m.mode {
	case modeList:
		if m.generating {
			b.WriteString(filterStyle.Render("generate from: " + m.genIn.View()))
			b.WriteString("\n")
		}
		if m.instancing {
			b.WriteString(filterStyle.Render("instance name for " + m.selectedName() + " @: " + m.instanceIn.View()))
			b.WriteString("\n")
		}
		if m.filtering || m.filterStr != "" {
			b.WriteString(filterStyle.Render("/ " + m.filterIn.View()))
			b.WriteString("\n")
		}
		b.WriteString(m.table.View())
		if len(m.units) == 0 && !m.loading {
			b.WriteString("\n")
			b.WriteString(helpStyle.Render(
				"No quadlet units found. Drop *.container / *.pod / *.kube / *.volume / *.image / *.build files into"))
			b.WriteString("\n")
			b.WriteString(helpStyle.Render(strings.Join(quadlet.SearchDirs(), "  or  ")))
		}
		if len(m.stale) > 0 && !m.loading {
			b.WriteString("\n")
			b.WriteString(warnStyle.Render(fmt.Sprintf(
				"⚠ %d quadlet file(s) changed since last daemon-reload - press R (e.g. %s)",
				len(m.stale), m.stale[0].Name)))
		}
	case modeDetail:
		u, ok := m.selected()
		name := ""
		if ok {
			name = u.UnitName
			if e := m.podmanInfo[u.UnitName]; e.App != "" {
				name += " · app: " + e.App
			}
			if e := m.podmanInfo[u.UnitName]; e.Pod != "" {
				name += " · pod: " + e.Pod
			}
		}
		b.WriteString(m.tabBar(name))
		b.WriteString("\n")
		if m.tab == tabJournal && (m.searching || m.searchStr != "") {
			info := ""
			if m.searchStr != "" {
				info = fmt.Sprintf("  (%d/%d matches)", m.matchPos, m.searchMatches)
				if m.searchMatches == 0 {
					info = "  (no matches)"
				}
			}
			b.WriteString(filterStyle.Render("/ " + m.searchIn.View() + info))
			b.WriteString("\n")
		}
		b.WriteString(m.viewport.View())
	case modeStorage:
		b.WriteString(headerStyle.Render(" STORAGE — podman system df "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeEvents:
		b.WriteString(headerStyle.Render(" EVENTS — podman events (live, f pause, q back) "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeGenerate:
		b.WriteString(headerStyle.Render(" GENERATE — preview (y write & reload, esc cancel) "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeValidate:
		b.WriteString(headerStyle.Render(" PROBLEMS "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeTree:
		b.WriteString(headerStyle.Render(" TREE — quadlet dependencies "))
		b.WriteString("\n")
		b.WriteString(m.treeModel.View())
	case modeUpdates:
		b.WriteString(headerStyle.Render(" AUTO-UPDATE "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeRecent:
		b.WriteString(headerStyle.Render(" ACTIONS — recent results this session "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	}

	b.WriteString("\n")
	b.WriteString(m.helpBar())

	if m.pickingEditor {
		b.WriteString("\n")
		b.WriteString(warnStyle.Render(m.pickerLine()))
	} else if m.busy {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(m.spinner.View() + " " + m.busyText))
	} else if m.statusLine != "" {
		b.WriteString("\n")
		if m.statusErr {
			b.WriteString(errStyle.Render("✗ " + m.statusLine))
		} else {
			b.WriteString(okStyle.Render("✓ " + m.statusLine))
		}
	}
	v := tea.NewView(b.String())
	v.AltScreen = true
	if m.mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
