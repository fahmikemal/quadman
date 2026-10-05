package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// keyMap groups the keybindings of one mode; help renders it contextually.
type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Enter        key.Binding
	Logs         key.Binding
	Filter       key.Binding
	Start        key.Binding
	Stop         key.Binding
	Restart      key.Binding
	Enable       key.Binding
	Disable      key.Binding
	Edit         key.Binding
	Updates      key.Binding
	Health       key.Binding
	Exec         key.Binding
	Stats        key.Binding
	Prune        key.Binding
	DaemonReload key.Binding
	Linger       key.Binding
	Follow       key.Binding
	Timer        key.Binding
	Refresh      key.Binding
	Help         key.Binding
	Quit         key.Binding
	Back         key.Binding
}

func listKeys() keyMap {
	return keyMap{
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view file")),
		Logs:         key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
		Filter:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Start:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
		Stop:         key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Restart:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
		Enable:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "enable now")),
		Disable:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "disable boot")),
		Edit:         key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit")),
		Updates:      key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "updates")),
		Health:       key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "healthcheck")),
		Exec:         key.NewBinding(key.WithKeys("X"), key.WithHelp("X", "exec shell")),
		Stats:        key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "stats")),
		Prune:        key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "system prune")),
		DaemonReload: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "daemon-reload")),
		Linger:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "linger")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func detailKeys() keyMap {
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll")),
		Edit:   key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit")),
		Follow: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow/pause")),
		Back:   key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "back")),
		Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

func updatesKeys() keyMap {
	return keyMap{
		Timer:   key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "toggle timer")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Back:    key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "back")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	switch {
	case k.Timer.Enabled(): // updates screen
		return []key.Binding{k.Timer, k.Refresh, k.Back, k.Quit}
	case k.Follow.Enabled(): // logs view
		return []key.Binding{k.Follow, k.Back, k.Quit}
	case k.Back.Enabled(): // file view
		return []key.Binding{k.Edit, k.Back, k.Quit}
	}
	return []key.Binding{
		k.Enter,
		k.Logs,
		key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		key.NewBinding(key.WithKeys("s", "x", "r"), key.WithHelp("s/x/r", "unit")),
		key.NewBinding(key.WithKeys("e", "d"), key.WithHelp("e/d", "boot")),
		key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "upd")),
		key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "reload")),
		k.Help,
		k.Quit,
	}
}

// keyHandler is one link in the handleKey dispatch chain: fn reports
// whether it consumed the key. Links run in slice order, so the order
// below is the precedence — focused inputs first, palette before modes,
// list actions last.
type keyHandler struct {
	name string
	fn   func(Model, tea.KeyPressMsg) (tea.Model, tea.Cmd, bool)
}

// keyChain lists every key handler in precedence order. It mirrors the
// old linear if-chain one-to-one; adding a screen means appending one
// entry here instead of extending handleKey.
var keyChain = []keyHandler{
	// Focused text inputs own their keys while open.
	{"search", Model.searchKeys},
	{"instance", Model.instanceKeys},
	{"generate-input", Model.generateKeys},
	{"exec-input", Model.execKeys},
	{"filter-input", Model.filterKeys},
	// Command palette modal handler.
	{"palette", Model.modePaletteKeys},
	{"palette-open", openPaletteKeys},
	// One small handler per mode.
	{"mode-updates", Model.modeUpdatesKeys},
	{"mode-storage", Model.modeStorageKeys},
	{"mode-events", Model.modeEventsKeys},
	{"mode-generate", Model.modeGenerateKeys},
	{"mode-validate", Model.modeValidateKeys},
	{"mode-tree", Model.modeTreeKeys},
	{"mode-detail", Model.modeDetailKeys},
	{"mode-recent", Model.modeRecentKeys},
	{"mode-timers", Model.modeTimersKeys},
	{"mode-secrets", Model.modeSecretsKeys},
	{"mode-stats", Model.modeStatsKeys},
	// .quadlets bundles get their own small action set: preview + install.
	{"bundle", Model.bundleKeys},
	// List-mode actions, one handler per key group.
	{"quit", Model.quitKeys},
	{"esc", Model.escKeys},
	{"help", Model.helpKeys},
	{"filter-open", Model.filterOpenKeys},
	{"start-restart", Model.startRestartKeys},
	{"stop", Model.stopKeys},
	{"enable", Model.enableKeys},
	{"disable", Model.disableKeys},
	{"edit", Model.editKeys},
	{"updates-open", Model.updatesOpenKeys},
	{"health", Model.healthKeys},
	{"storage-open", Model.storageKeys},
	{"timers-open", Model.timersKeys},
	{"secrets-open", Model.secretsKeys},
	{"events-open", Model.eventsKeys},
	{"generate-open", Model.generateOpenKeys},
	{"problems", Model.problemsKeys},
	{"recent-open", Model.openRecentKeys},
	{"tree-open", Model.treeKeysOpen},
	{"instantiate-open", Model.instantiateOpenKeys},
	{"delete", Model.deleteKeys},
	{"copy-name", Model.copyNameKeys},
	{"copy-image", Model.copyImageKeys},
	{"reload", Model.reloadKeys},
	{"linger", Model.lingerKeys},
	{"logs-open", Model.openLogsKeys},
	{"file-open", Model.openFileKeys},
	{"mark", Model.markKeys},
	{"exec-open", Model.execOpenKeys},
	{"stats-open", Model.statsOpenKeys},
	{"prune", Model.pruneKeys},
}

// openPaletteKeys launches the command palette (Ctrl+P). It is a table
// entry (not a *Keys method) because it takes no mode/input precondition.
func openPaletteKeys(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "ctrl+p" {
		return m, nil, false
	}
	m.openPalette()
	return m, nil, true
}

// handleKey routes one keypress: global interrupts first, then modal
// states (editor picker, armed confirmation), then the keyChain table,
// custom commands, and finally the list widget as fallback.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.stopLogs()
		return m, tea.Quit
	}

	// The first-use editor picker owns all keys until answered or cancelled.
	if m.pickingEditor {
		mm, cmd := m.pickEditor(msg)
		return mm, cmd
	}

	// An armed confirmation (stop/enable/disable/...) swallows the next key.
	if m.pending != nil {
		p := m.pending
		m.pending = nil
		if msg.String() != "y" {
			m.setStatus(p.verb+" cancelled", false)
			return m, nil
		}
		mm, cmd := m.runPending(p)
		return mm, cmd
	}

	// Dispatch through the keyChain table: first handler that
	// consumes the key wins. Table order is the precedence.
	for _, h := range keyChain {
		if mm, cmd, ok := h.fn(m, msg); ok {
			return mm, cmd
		}
	}

	// User-defined custom commands from config.yaml (single-char keys).
	if mm, cmd, ok := m.runCustom(msg.String()); ok {
		return mm, cmd
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (k keyMap) FullHelp() [][]key.Binding {
	switch {
	case k.Timer.Enabled():
		return [][]key.Binding{{k.Timer, k.Refresh, k.Back, k.Quit}}
	case k.Follow.Enabled():
		return [][]key.Binding{{k.Up, k.Down, k.Follow, k.Back, k.Quit}}
	case k.Back.Enabled():
		return [][]key.Binding{{k.Up, k.Down, k.Edit, k.Back, k.Quit}}
	}
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Logs, k.Filter},
		{k.Start, k.Stop, k.Restart, k.Enable, k.Disable},
		{k.Edit, k.Updates, k.Health, k.DaemonReload, k.Linger, k.Help, k.Quit},
		{k.Exec, k.Stats, k.Prune},
		{
			key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("^P", "command palette")),
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "problems")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "dep tree")),
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "instantiate")),
			key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete unit")),
			key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y/Y", "copy name/image")),
		},
	}
}
