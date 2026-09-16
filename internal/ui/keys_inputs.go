package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m Model) searchKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// While the log search input is focused, keys edit the search query.
	if !m.searching {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		m.searching = false
		m.searchIn.Blur()
		return m, nil, true
	case "esc":
		m.clearSearch()
		return m, nil, true
	}
	var cmd tea.Cmd
	m.searchIn, cmd = m.searchIn.Update(msg)
	if v := m.searchIn.Value(); v != m.searchStr {
		m.searchStr = v
		m.applySearch()
	}
	return m, cmd, true
}

func (m Model) instanceKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// While the instance-name input is focused, keys edit the name.
	if !m.instancing {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		m.instancing = false
		m.instanceIn.Blur()
		name := strings.TrimSpace(m.instanceIn.Value())
		m.instanceIn.SetValue("")
		if name == "" {
			m.setStatus("instantiate cancelled (empty name)", false)
			return m, nil, true
		}
		mm, cmd := m.instantiateUnit(name)
		return mm, cmd, true
	case "esc":
		m.instancing = false
		m.instanceIn.SetValue("")
		m.instanceIn.Blur()
		m.setStatus("instantiate cancelled", false)
		return m, nil, true
	}
	var cmd tea.Cmd
	m.instanceIn, cmd = m.instanceIn.Update(msg)
	return m, cmd, true
}

func (m Model) generateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// While the generate input is focused, keys edit the command/path.
	if !m.generating {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		m.generating = false
		m.genIn.Blur()
		input := strings.TrimSpace(m.genIn.Value())
		m.genIn.SetValue("")
		if input == "" {
			m.setStatus("generate cancelled (empty input)", false)
			return m, nil, true
		}
		return m, tea.Batch(m.setBusy("podlet generate"), generateCmd(input)), true
	case "esc":
		m.generating = false
		m.genIn.SetValue("")
		m.genIn.Blur()
		m.setStatus("generate cancelled", false)
		return m, nil, true
	}
	var cmd tea.Cmd
	m.genIn, cmd = m.genIn.Update(msg)
	return m, cmd, true
}

func (m Model) filterKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// While the filter input is focused, keys edit the filter.
	if !m.filtering {
		return m, nil, false
	}
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.filterIn.Blur()
		return m, nil, true
	case "esc":
		m.filtering = false
		m.filterStr = ""
		m.filterIn.SetValue("")
		m.filterIn.Blur()
		m.refilter()
		return m, nil, true
	}
	var cmd tea.Cmd
	m.filterIn, cmd = m.filterIn.Update(msg)
	if v := m.filterIn.Value(); v != m.filterStr {
		m.filterStr = v
		m.refilter()
	}
	return m, cmd, true
}
