package ui

import (
	"charm.land/bubbles/v2/key"
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
		DaemonReload: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "daemon-reload")),
		Linger:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "linger")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func viewKeys() keyMap {
	return keyMap{
		Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll")),
		Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll")),
		Edit: key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit")),
		Back: key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "back")),
		Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
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

func (k keyMap) isList() bool { return k.Enter.Enabled() }

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
		{
			key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "problems")),
			key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "dep tree")),
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "instantiate")),
			key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete unit")),
			key.NewBinding(key.WithKeys("y", "Y"), key.WithHelp("y/Y", "copy name/image")),
		},
	}
}
