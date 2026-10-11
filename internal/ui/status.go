package ui

import (
	"strings"
	"time"
)

func (m *Model) setStatus(text string, isErr bool) {
	m.statusLine = text
	m.statusErr = isErr
	m.statusAt = time.Now()
}

func (m *Model) clearStatus() {
	m.statusLine = ""
	m.statusErr = false
}

func (m *Model) resize() {
	w, h := m.size()
	m.help.SetWidth(w)
	// Reserve the real rendered height of every chrome block — the
	// expanded help block is much taller than the compact legend, and
	// undercounting clips its last lines in short terminals.
	chrome := 2 + lineCount(m.helpBar()) + 1 // top bar + nav strip, footer, status line
	switch m.mode {
	case modeList:
		chrome = h - m.listRowsAvail()
	case modeDetail:
		chrome += 2 // header + tab bar
		if m.tab == tabJournal && (m.searching || m.searchStr != "") {
			chrome++ // search line
		}
		if m.tab == tabJournal && (m.filteringLogs || m.logFilter != "") {
			chrome++ // log filter line
		}
	case modeGenerate:
		chrome += 4 // input, file tabs, checks, target
	}
	body := max(h-chrome, 1)
	switch m.mode {
	case modeList:
		m.table.SetWidth(m.tableWidth())
		m.table.SetHeight(body + 1) // the widget subtracts its header row
		m.buildRows()
		m.syncListOffset()
	default:
		m.viewport.SetWidth(w)
		m.viewport.SetHeight(body)
	}
}

// listRowsAvail is how many unit rows fit on screen right now: the
// height left after the top bar, nav strip, dashboard preamble, table
// header, sort hint, empty-state notice, footer and status line. The list
// renderer calls it every frame, so prompts that open or close never
// push the footer off screen even before the next resize.
func (m Model) listRowsAvail() int {
	w, h := m.size()
	used := 2 + lineCount(m.listPreamble(w)) + 2 + lineCount(m.helpBar()) + 1
	if len(m.units) == 0 && !m.loading {
		used += 2
	}
	return max(h-used, 1)
}

// lineCount is the number of screen rows s occupies ("" is zero).
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
