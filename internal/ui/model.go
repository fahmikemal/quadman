// Package ui implements the quadman terminal interface.
package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/quadman-dev/quadman/internal/loginctl"
	"github.com/quadman-dev/quadman/internal/podman"
	"github.com/quadman-dev/quadman/internal/quadlet"
	"github.com/quadman-dev/quadman/internal/systemd"
)

type mode int

const (
	modeList mode = iota
	modeFile
	modeLogs
)

// pollInterval is how often the unit list refreshes itself. The cursor stays
// pinned on the same unit (by name) across refreshes.
const pollInterval = 2500 * time.Millisecond

type tickMsg struct{}

type refreshMsg struct {
	units       []quadlet.Unit
	images      []string
	statuses    map[string]systemd.Status
	linger      bool
	lingerOK    bool
	podman      map[string]podman.Entry
	podmanTried bool
	stale       []quadlet.Unit
	err         error
}

type actionMsg struct {
	desc string
	out  string
	err  error
}

type logsMsg struct {
	unit string
	out  string
	err  error
}

type lingerSetMsg struct {
	on    bool
	known bool
	err   error
}

// keyMap groups the keybindings of one mode; help renders it contextually.
type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Enter        key.Binding
	Logs         key.Binding
	Start        key.Binding
	Stop         key.Binding
	Restart      key.Binding
	Enable       key.Binding
	Disable      key.Binding
	DaemonReload key.Binding
	Linger       key.Binding
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
		Start:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
		Stop:         key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Restart:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
		Enable:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "enable now")),
		Disable:      key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "disable boot")),
		DaemonReload: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "daemon-reload")),
		Linger:       key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "toggle linger")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func viewKeys() keyMap {
	k := keyMap{
		Up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll")),
		Down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll")),
		Back: key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "back")),
		Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
	return k
}

func (k keyMap) isList() bool { return k.Enter.Enabled() }

func (k keyMap) ShortHelp() []key.Binding {
	if k.isList() {
		return []key.Binding{k.Enter, k.Logs, k.Start, k.Stop, k.Restart, k.Enable, k.Disable, k.DaemonReload, k.Linger, k.Help, k.Quit}
	}
	return []key.Binding{k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	if k.isList() {
		return [][]key.Binding{
			{k.Up, k.Down, k.Enter, k.Logs},
			{k.Start, k.Stop, k.Restart, k.Enable, k.Disable},
			{k.DaemonReload, k.Linger, k.Help, k.Quit},
		}
	}
	return [][]key.Binding{{k.Up, k.Down, k.Back, k.Quit}}
}

// Model is the quadman Bubble Tea model.
type Model struct {
	sys      *systemd.Systemd
	lc       *loginctl.Loginctl
	mode     mode
	table    table.Model
	viewport viewport.Model
	help     help.Model
	spinner  spinner.Model

	units       []quadlet.Unit
	status      map[string]systemd.Status
	linger      bool
	lingerKnown bool
	loading     bool
	podmanInfo  map[string]podman.Entry
	podmanTried bool
	stale       []quadlet.Unit

	busy     bool
	busyText string

	width, height int
	statusLine    string
	statusErr     bool
	showHelp      bool
}

// New returns the initial quadman model.
func New() Model {
	cols := []table.Column{
		{Title: "QUADLET", Width: 22},
		{Title: "KIND", Width: 10},
		{Title: "SYSTEMD UNIT", Width: 26},
		{Title: "STATE", Width: 11},
		{Title: "SUB", Width: 10},
		{Title: "IMAGE", Width: 34},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	return Model{
		sys:      systemd.New(),
		lc:       loginctl.New(),
		table:    t,
		viewport: vp,
		help:     help.New(),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		status:   map[string]systemd.Status{},
		loading:  true,
	}
}

// Run starts the quadman TUI.
func Run() error {
	_, err := tea.NewProgram(New()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.sys, m.lc, true), pollCmd())
}

func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func refreshCmd(sys *systemd.Systemd, lc *loginctl.Loginctl, withPodman bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		units, err := quadlet.Discover()
		if err != nil {
			return refreshMsg{err: err}
		}
		images := make([]string, len(units))
		for i := range units {
			info := quadlet.Inspect(units[i]) // resolves ServiceName= and image in one parse
			units[i].UnitName = info.UnitName
			images[i] = info.Image
		}
		statuses, err := sys.Show(ctx, unitNames(units))
		if err != nil {
			return refreshMsg{units: units, images: images, err: err}
		}
		linger, lerr := lc.Enabled(ctx, "")

		// Optional enrichment via podman quadlet list (app/pod grouping).
		var pinfo map[string]podman.Entry
		if withPodman && podman.Available() {
			if entries, perr := podman.QuadletList(ctx); perr == nil {
				pinfo = podman.ByUnit(entries)
			}
		}

		stale := quadlet.StaleUnits(units, systemd.UserGeneratorDir())
		return refreshMsg{
			units:       units,
			images:      images,
			statuses:    statuses,
			linger:      linger,
			lingerOK:    lerr == nil,
			podman:      pinfo,
			podmanTried: withPodman,
			stale:       stale,
		}
	}
}

func actionCmd(desc string, run func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		out, err := run(context.Background())
		return actionMsg{desc: desc, out: out, err: err}
	}
}

func logsCmd(sys *systemd.Systemd, unit string) tea.Cmd {
	return func() tea.Msg {
		out, err := sys.Journal(context.Background(), unit, 300)
		return logsMsg{unit: unit, out: out, err: err}
	}
}

func unitNames(units []quadlet.Unit) []string {
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.UnitName)
	}
	return names
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		// Poll: refresh the list; only try podman enrichment again if it
		// never ran (a failed run is not retried every tick).
		return m, tea.Batch(refreshCmd(m.sys, m.lc, !m.podmanTried), pollCmd())

	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case refreshMsg:
		return m.applyRefresh(msg)

	case actionMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus(msg.desc+": "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus(strings.TrimSpace(msg.desc+" ok "+msg.out), false)
		return m, refreshCmd(m.sys, m.lc, false)

	case logsMsg:
		m.busy = false
		if msg.err != nil {
			m.viewport.SetContent(
				dimErrStyle.Render("journalctl failed for " + msg.unit + "\n\n" + msg.err.Error() +
					"\n\nHint: the user journal may be missing. Is systemd user session running?"))
		} else {
			m.viewport.SetContent(msg.out)
		}
		m.viewport.GotoBottom()
		m.mode = modeLogs
		m.resize()
		m.clearStatus()
		return m, nil

	case lingerSetMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("linger: "+msg.err.Error(), true)
			return m, nil
		}
		m.linger = msg.on
		m.lingerKnown = true
		m.setStatus("linger "+onOff(msg.on), false)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Everything else (mouse, focus…) goes to the active widget.
	var cmd tea.Cmd
	if m.mode == modeList {
		m.table, cmd = m.table.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return m, cmd
}

// setBusy marks an action in flight; the spinner runs until its result msg lands.
func (m *Model) setBusy(text string) tea.Cmd {
	m.busy = true
	m.busyText = text
	return m.spinner.Tick
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.mode != modeList {
			m.mode = modeList
			m.resize()
			return m, nil
		}
		return m, nil
	}

	if m.mode != modeList {
		// In file/log views only navigation is live; q returns to the list.
		if msg.String() == "q" {
			m.mode = modeList
			m.resize()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "?":
		m.showHelp = !m.showHelp
		m.resize()
		return m, nil

	case "s", "x", "r":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		verb := map[string]string{"s": "start", "x": "stop", "r": "restart"}[msg.String()]
		sys := m.sys
		return m, tea.Batch(m.setBusy(verb+" "+u.UnitName),
			actionCmd(verb+" "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, verb, u.UnitName)
			}))

	case "e", "d":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		if msg.String() == "e" {
			return m, tea.Batch(m.setBusy("enable --now "+u.UnitName),
				actionCmd("enable --now "+u.UnitName, func(ctx context.Context) (string, error) {
					return sys.Enable(ctx, u.UnitName, true)
				}))
		}
		return m, tea.Batch(m.setBusy("disable "+u.UnitName),
			actionCmd("disable "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.Disable(ctx, u.UnitName)
			}))

	case "R":
		m.podmanTried = false // re-enrich app/pod grouping after regeneration
		return m, tea.Batch(m.setBusy("daemon-reload"), actionCmd("daemon-reload", m.sys.DaemonReload))

	case "L":
		target := !m.linger
		lc := m.lc
		return m, tea.Batch(m.setBusy("toggle linger"), func() tea.Msg {
			if err := lc.Set(context.Background(), "", target); err != nil {
				return lingerSetMsg{on: !target, err: err}
			}
			return lingerSetMsg{on: target, known: true}
		})

	case "l":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		return m, tea.Batch(m.setBusy("loading logs "+u.UnitName), logsCmd(m.sys, u.UnitName))

	case "enter":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		content, err := os.ReadFile(u.Path)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		m.viewport.SetContent(string(content))
		m.viewport.GotoTop()
		m.mode = modeFile
		m.resize()
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) applyRefresh(msg refreshMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		m.clearStatus()
	}
	if msg.err != nil {
		m.setStatus(msg.err.Error(), true)
	}
	if msg.units == nil {
		return m, nil
	}

	// Pin the cursor on the same unit across refreshes.
	pin := ""
	if c := m.table.Cursor(); c >= 0 && c < len(m.units) {
		pin = m.units[c].UnitName
	}

	m.units = msg.units
	m.status = msg.statuses
	m.linger = msg.linger
	m.lingerKnown = msg.lingerOK
	m.stale = msg.stale
	if msg.podmanTried {
		m.podmanTried = true
	}
	if msg.podman != nil {
		m.podmanInfo = msg.podman
	}

	rows := make([]table.Row, 0, len(msg.units))
	for i, u := range msg.units {
		state, sub := "-", "-"
		if msg.err == nil {
			state, sub = msg.statuses[u.UnitName].Display()
		}
		rows = append(rows, table.Row{u.Name, string(u.Kind), u.UnitName, state, sub, msg.images[i]})
	}
	m.table.SetRows(rows)
	if pin != "" {
		for i, u := range m.units {
			if u.UnitName == pin {
				m.table.SetCursor(i)
				break
			}
		}
	}
	m.loading = false
	return m, nil
}

func (m Model) selected() (quadlet.Unit, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.units) {
		return quadlet.Unit{}, false
	}
	return m.units[c], true
}

func (m *Model) setStatus(text string, isErr bool) {
	m.statusLine = text
	m.statusErr = isErr
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
	chrome := 4 // title + blank + help + status
	if m.showHelp {
		chrome += 6
	}
	if len(m.stale) > 0 {
		chrome++ // reload banner
	}
	body := h - chrome
	if body < 3 {
		body = 3
	}
	if m.mode == modeList {
		m.table.SetWidth(w)
		m.table.SetHeight(body)
	} else {
		m.viewport.SetWidth(w)
		m.viewport.SetHeight(body)
	}
}

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" quadman — rootless quadlet manager "))
	b.WriteString("\n")

	switch m.mode {
	case modeList:
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
				"⚠ %d quadlet file(s) changed since last daemon-reload — press R (e.g. %s)",
				len(m.stale), m.stale[0].Name)))
		}
	case modeFile:
		u, ok := m.selected()
		name := ""
		if ok {
			name = u.Path
			if e := m.podmanInfo[u.UnitName]; e.App != "" {
				name += " · app: " + e.App
			}
			if e := m.podmanInfo[u.UnitName]; e.Pod != "" {
				name += " · pod: " + e.Pod
			}
		}
		b.WriteString(headerStyle.Render(" FILE " + name))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeLogs:
		u, ok := m.selected()
		unit := ""
		if ok {
			unit = u.UnitName
			if d := m.status[unit].Description; d != "" {
				unit += " — " + d
			}
		}
		b.WriteString(headerStyle.Render(" LOGS " + unit + "  (last 300 lines, q to go back)"))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	}

	b.WriteString("\n")
	b.WriteString(m.helpBar())

	if m.busy {
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
	return v
}

func (m Model) keys() keyMap {
	if m.mode == modeList {
		return listKeys()
	}
	return viewKeys()
}

func (m Model) helpBar() string {
	linger := lingerUnknownStyle.Render("linger: ?")
	if m.lingerKnown {
		if m.linger {
			linger = lingerOnStyle.Render("linger: on")
		} else {
			linger = lingerOffStyle.Render("linger: off")
		}
	}
	m.help.ShowAll = m.showHelp
	if m.showHelp {
		lines := []string{
			m.help.View(m.keys()) + "  ·  " + linger,
			"Linger keeps rootless containers running after logout — enable it once on every quadlet host (loginctl enable-linger).",
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e enables the unit to start at boot (with --now); d removes it from boot (the container keeps running).",
			"Quadlet search order: " + strings.Join(quadlet.SearchDirs(), " → "),
		}
		return helpStyle.Render(strings.Join(lines, "\n"))
	}
	return helpStyle.Render(m.help.View(m.keys()) + "  ·  " + linger)
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
