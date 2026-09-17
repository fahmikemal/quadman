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
	w, h := m.width, m.height
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 30
	}
	m.help.SetWidth(w)
	// Reserve the real rendered height of the help bar — the expanded help
	// block is much taller than the compact legend, and undercounting clips
	// its last lines in short terminals.
	chrome := 3 + strings.Count(m.helpBar(), "\n") + 1 // title + blank + help bar + status
	if m.mode == modeList && len(m.stale) > 0 {
		chrome++ // reload banner
	}
	if m.mode == modeList && (m.filtering || m.filterStr != "") {
		chrome++ // filter line
	}
	if m.mode == modeDetail && m.tab == tabJournal && (m.searching || m.searchStr != "") {
		chrome++ // search line
	}
	if m.mode == modeDetail && m.tab == tabJournal && (m.filteringLogs || m.logFilter != "") {
		chrome++ // log filter line
	}
	if m.pickingEditor {
		chrome++ // editor picker prompt
	}
	if m.instancing {
		chrome++ // instance-name input
	}
	if m.generating {
		chrome++ // generate input
	}
	body := h - chrome
	if body < 3 {
		body = 3
	}
	switch m.mode {
	case modeList:
		m.table.SetWidth(w)
		m.table.SetHeight(body)
	case modeTree:
		m.treeModel.SetWidth(w)
		m.treeModel.SetHeight(body)
	default:
		m.viewport.SetWidth(w)
		m.viewport.SetHeight(body)
	}
}
