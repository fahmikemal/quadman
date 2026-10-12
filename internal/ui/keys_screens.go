package ui

import (
	tea "charm.land/bubbletea/v2"
)

// Secondary-screen key handlers. Each screen's own keys run first; the
// shared navigation (screen.go) then consumes everything else so no key
// ever reaches the unit-list actions behind the screen.

func (m Model) screenHandler(md mode, own func(Model, tea.KeyPressMsg) (tea.Model, tea.Cmd, bool), msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != md {
		return m, nil, false
	}
	// An async screen without data yet only takes its retry key: nothing
	// may act on rows that are not shown. Navigation and esc stay live.
	waiting := isAsyncScreen(md) && !m.screenLoaded(md) && msg.String() != retryKey(md)
	if own != nil && !waiting {
		if mm, cmd, ok := own(m, msg); ok {
			return mm, cmd, true
		}
	}
	mm, cmd := m.screenKeys(msg)
	return mm, cmd, true
}

func (m Model) modeStorageKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeStorage, Model.storageKeysIn, msg)
}

func (m Model) modeTimersKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeTimers, Model.timersKeysIn, msg)
}

func (m Model) modeSecretsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeSecrets, Model.secretsKeysIn, msg)
}

func (m Model) modeEventsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeEvents, Model.eventsKeysIn, msg)
}

func (m Model) modeRecentKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeRecent, Model.actionsKeysIn, msg)
}

func (m Model) modeStatsKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeStats, Model.statsKeysIn, msg)
}

func (m Model) modeUpdatesKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeUpdates, Model.updatesKeysIn, msg)
}

func (m Model) modeValidateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeValidate, Model.problemsKeysIn, msg)
}

func (m Model) modeTreeKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	return m.screenHandler(modeTree, Model.treeKeysIn, msg)
}

// openRecentKeys opens the actions screen (A).
func (m Model) openRecentKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "A" {
		return m, nil, false
	}
	m.enterScreen(modeRecent)
	return m, nil, true
}
