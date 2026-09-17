package ui

import (
	tea "charm.land/bubbletea/v2"
)

// modePaletteKeys handles keystrokes while the Command Palette overlay is active.
func (m Model) modePaletteKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modePalette {
		return m, nil, false
	}

	switch msg.String() {
	case "esc", "ctrl+p":
		m.closePalette()
		return m, nil, true

	case "up", "ctrl+k":
		if m.paletteCursor > 0 {
			m.paletteCursor--
		}
		return m, nil, true

	case "down", "ctrl+j":
		if m.paletteCursor < len(m.paletteFiltered)-1 {
			m.paletteCursor++
		}
		return m, nil, true

	case "enter":
		if len(m.paletteFiltered) == 0 {
			return m, nil, true
		}
		act := m.paletteFiltered[m.paletteCursor]
		if act.enabled != nil {
			if ok, reason := act.enabled(m); !ok {
				if reason != "" {
					m.setStatus("cannot run "+act.title+": "+reason, true)
				} else {
					m.setStatus("cannot run "+act.title+" in current state", true)
				}
				m.closePalette()
				return m, nil, true
			}
		}
		m.closePalette()
		if act.run != nil {
			return act.run(m)
		}
		return m, nil, true
	}

	// Any other key updates the search query input and refilters the list.
	var cmd tea.Cmd
	prevVal := m.paletteIn.Value()
	m.paletteIn, cmd = m.paletteIn.Update(msg)
	if m.paletteIn.Value() != prevVal {
		m.refilterPalette()
	}
	return m, cmd, true
}
