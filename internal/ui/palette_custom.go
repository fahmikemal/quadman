package ui

import (
	tea "charm.land/bubbletea/v2"
)

// customPaletteActions exposes one palette entry per user-defined command
// from config.yaml.
func (m Model) customPaletteActions() []paletteAction {
	var actions []paletteAction

	for _, cc := range m.custom {
		cmdObj := cc
		actions = append(actions, paletteAction{
			id:          "custom-" + cmdObj.Name,
			title:       "Custom: " + cmdObj.Name,
			shortcut:    cmdObj.Key,
			category:    "Custom",
			description: cmdObj.Run,
			enabled: func(m Model) (bool, string) {
				if m.readonly {
					return false, "readonly"
				}
				_, ok := m.selected()
				if !ok {
					return false, "no selection"
				}
				return true, ""
			},
			run: func(m Model) (tea.Model, tea.Cmd, bool) {
				u, ok := m.selected()
				if !ok {
					return m, nil, true
				}
				return m, tea.Batch(m.setBusy("custom "+cmdObj.Name), customCmd(cmdObj, u, m.images)), true
			},
		})
	}

	return actions
}
