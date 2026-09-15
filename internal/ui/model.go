// Package ui implements the quadman terminal interface.
package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kemal-labs/quadman/internal/loginctl"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

type mode int

const (
	modeList mode = iota
	modeFile
	modeLogs
	modeUpdates
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
	health      map[string]string
	stale       []quadlet.Unit
	err         error
}

type actionMsg struct {
	desc string
	hint string
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

type editorFinishedMsg struct {
	changed bool
	err     error
}

type healthMsg struct {
	container string
	ok        bool
	err       error
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
	filterIn textinput.Model

	units       []quadlet.Unit
	filtered    []quadlet.Unit
	images      map[string]string
	status      map[string]systemd.Status
	health      map[string]string
	linger      bool
	lingerKnown bool
	loading     bool
	podmanInfo  map[string]podman.Entry
	podmanTried bool
	stale       []quadlet.Unit

	// filter
	filtering bool
	filterStr string

	// follow logs
	sess      *logSession
	logLines  []string
	following bool

	// updates screen
	updateEntries []podman.AutoUpdateEntry
	timerEnabled  string
	timerActive   string

	// stop confirmation
	confirmStop bool

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
	fi := textinput.New()
	fi.Placeholder = "filter units…"
	return Model{
		sys:      systemd.New(),
		lc:       loginctl.New(),
		table:    t,
		viewport: vp,
		help:     help.New(),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		filterIn: fi,
		status:   map[string]systemd.Status{},
		images:   map[string]string{},
		health:   map[string]string{},
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

		// Optional enrichment via podman (app/pod grouping + container health).
		var pinfo map[string]podman.Entry
		var health map[string]string
		if withPodman && podman.Available() {
			if entries, perr := podman.QuadletList(ctx); perr == nil {
				pinfo = podman.ByUnit(entries)
			}
			if hm, herr := podman.PsHealth(ctx); herr == nil {
				health = hm
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
			health:      health,
			stale:       stale,
		}
	}
}

func actionCmd(desc string, run func(context.Context) (string, error)) tea.Cmd {
	return actionCmdHint(desc, "", run)
}

func actionCmdHint(desc, hint string, run func(context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		out, err := run(context.Background())
		return actionMsg{desc: desc, hint: hint, out: out, err: err}
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
		// Poll: refresh the list; only retry podman enrichment if it never ran.
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
		status := strings.TrimSpace(msg.desc + " ok " + msg.out)
		if msg.hint != "" {
			status += " — " + msg.hint
		}
		m.setStatus(status, false)
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
		m.following = false
		m.resize()
		m.clearStatus()
		return m, nil

	case logLineMsg:
		if msg.sess != m.sess { // a superseded session must not touch the view
			return m, nil
		}
		wasAtBottom := m.viewport.AtBottom()
		m.logLines = append(m.logLines, msg.line)
		if len(m.logLines) > logBufferCap {
			m.logLines = m.logLines[len(m.logLines)-logBufferCap:]
		}
		m.viewport.SetContent(strings.Join(m.logLines, "\n"))
		if m.following && wasAtBottom {
			m.viewport.GotoBottom()
		}
		return m, followLine(m.sess)

	case logsEndMsg:
		if msg.sess == m.sess && msg.err != nil {
			m.setStatus("journal stream ended: "+msg.err.Error(), true)
		}
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

	case updatesMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("auto-update: "+msg.err.Error(), true)
			return m, nil
		}
		m.updateEntries = msg.entries
		m.timerEnabled = msg.timerEnabled
		m.timerActive = msg.timerActive
		m.mode = modeUpdates
		m.viewport.SetContent(m.updatesView())
		m.viewport.GotoTop()
		m.resize()
		m.clearStatus()
		return m, nil

	case timerToggledMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("timer: "+msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("timer toggled", false)
		return m, updatesCmd(m.sys)

	case editorFinishedMsg:
		if msg.err != nil {
			m.setStatus("editor: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.changed {
			m.setStatus("file changed — press R to regenerate", false)
		} else {
			m.setStatus("no changes", false)
		}
		if m.mode == modeFile {
			if u, ok := m.selected(); ok {
				if content, rerr := os.ReadFile(u.Path); rerr == nil {
					m.viewport.SetContent(string(content))
				}
			}
		}
		return m, refreshCmd(m.sys, m.lc, false)

	case healthMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("healthcheck: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.ok {
			m.setStatus("healthcheck "+msg.container+": healthy", false)
		} else {
			m.setStatus("healthcheck "+msg.container+": UNHEALTHY", true)
		}
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
	if msg.String() == "ctrl+c" {
		m.stopLogs()
		return m, tea.Quit
	}

	// Pending stop confirmation swallows the next key.
	if m.confirmStop {
		m.confirmStop = false
		if msg.String() != "y" {
			m.setStatus("stop cancelled", false)
			return m, nil
		}
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		hint := ""
		if u.Kind == quadlet.KindContainer {
			hint = "container removed; state lives in volumes"
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("stop "+u.UnitName),
			actionCmdHint("stop "+u.UnitName, hint, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "stop", u.UnitName)
			}))
	}

	// While the filter input is focused, keys edit the filter.
	if m.filtering {
		switch msg.String() {
		case "enter":
			m.filtering = false
			m.filterIn.Blur()
			return m, nil
		case "esc":
			m.filtering = false
			m.filterStr = ""
			m.filterIn.SetValue("")
			m.filterIn.Blur()
			m.refilter()
			return m, nil
		}
		var cmd tea.Cmd
		m.filterIn, cmd = m.filterIn.Update(msg)
		if v := m.filterIn.Value(); v != m.filterStr {
			m.filterStr = v
			m.refilter()
		}
		return m, cmd
	}

	switch m.mode {
	case modeUpdates:
		switch msg.String() {
		case "esc", "q":
			m.mode = modeList
			m.resize()
			return m, nil
		case "U":
			target := m.timerEnabled != "enabled"
			sys := m.sys
			return m, tea.Batch(m.setBusy("toggle "+autoUpdateTimer), func() tea.Msg {
				ctx := context.Background()
				var err error
				if target {
					_, err = sys.Enable(ctx, autoUpdateTimer, true)
				} else {
					_, err = sys.Disable(ctx, autoUpdateTimer, true)
				}
				return timerToggledMsg{err: err}
			})
		case "r":
			return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys))
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modeFile, modeLogs:
		switch msg.String() {
		case "esc", "q":
			m.stopLogs()
			m.mode = modeList
			m.resize()
			return m, nil
		case "f":
			if m.mode == modeLogs && m.sess != nil {
				m.following = !m.following
				if m.following {
					m.viewport.GotoBottom()
				}
			}
			return m, nil
		case "E":
			return m.editSelected()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, tea.Quit

	case "esc":
		if m.filterStr != "" {
			m.filterStr = ""
			m.filterIn.SetValue("")
			m.refilter()
		}
		return m, nil

	case "?":
		m.showHelp = !m.showHelp
		m.resize()
		return m, nil

	case "/":
		m.filtering = true
		m.filterIn.Focus()
		return m, textinput.Blink

	case "s", "r":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		verb := map[string]string{"s": "start", "r": "restart"}[msg.String()]
		sys := m.sys
		return m, tea.Batch(m.setBusy(verb+" "+u.UnitName),
			actionCmd(verb+" "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, verb, u.UnitName)
			}))

	case "x":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.confirmStop = true
		m.setStatus("stop "+u.UnitName+"? container will be removed (quadlet runs --rm) [y/N]", false)
		return m, nil

	case "e":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("enable --now "+u.UnitName),
			actionCmd("enable --now "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.Enable(ctx, u.UnitName, true)
			}))

	case "d":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("disable "+u.UnitName),
			actionCmd("disable "+u.UnitName, func(ctx context.Context) (string, error) {
				return sys.Disable(ctx, u.UnitName, false)
			}))

	case "E":
		return m.editSelected()

	case "u":
		return m, tea.Batch(m.setBusy("checking auto-updates"), updatesCmd(m.sys))

	case "h":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if u.Kind != quadlet.KindContainer {
			m.setStatus("healthchecks apply to container units", false)
			return m, nil
		}
		container := "systemd-" + u.Name
		return m, tea.Batch(m.setBusy("healthcheck "+container), func() tea.Msg {
			ok, err := podman.HealthcheckRun(context.Background(), container)
			return healthMsg{container: container, ok: ok, err: err}
		})

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
		return m.startLogs(u)

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

// editSelected opens the unit file in $EDITOR via tea.ExecProcess.
func (m Model) editSelected() (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	before := fileMtime(u.Path)
	cmd := exec.Command(editor, u.Path)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{changed: fileMtime(u.Path) != before, err: err}
	})
}

func fileMtime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
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
	if c := m.table.Cursor(); c >= 0 && c < len(m.filtered) {
		pin = m.filtered[c].UnitName
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
	if msg.health != nil {
		m.health = msg.health
	}
	m.images = make(map[string]string, len(msg.units))
	for i, u := range msg.units {
		m.images[u.UnitName] = msg.images[i]
	}

	m.filtered = filterUnits(m.units, m.filterStr)
	m.buildRows()
	if pin != "" {
		for i, u := range m.filtered {
			if u.UnitName == pin {
				m.table.SetCursor(i)
				break
			}
		}
	}
	m.loading = false
	return m, nil
}

// refilter recomputes the visible unit list after the filter changed,
// keeping the cursor pinned on the same unit when it survives the filter.
func (m *Model) refilter() {
	pin := ""
	if c := m.table.Cursor(); c >= 0 && c < len(m.filtered) {
		pin = m.filtered[c].UnitName
	}
	m.filtered = filterUnits(m.units, m.filterStr)
	m.buildRows()
	for i, u := range m.filtered {
		if u.UnitName == pin {
			m.table.SetCursor(i)
			break
		}
	}
}

func filterUnits(units []quadlet.Unit, pattern string) []quadlet.Unit {
	if pattern == "" {
		return units
	}
	var out []quadlet.Unit
	for _, u := range units {
		if fuzzyMatch(u.Name+" "+u.UnitName+" "+string(u.Kind), pattern) {
			out = append(out, u)
		}
	}
	return out
}

// buildRows renders the filtered unit list into the table. A unit whose
// container podman reports as unhealthy surfaces that in the STATE column.
func (m *Model) buildRows() {
	rows := make([]table.Row, 0, len(m.filtered))
	for _, u := range m.filtered {
		state, sub := m.status[u.UnitName].Display()
		if m.health["systemd-"+u.Name] == "unhealthy" {
			state = "unhealthy"
			sub = "health"
		}
		rows = append(rows, table.Row{u.Name, string(u.Kind), u.UnitName, state, sub, m.images[u.UnitName]})
	}
	m.table.SetRows(rows)
}

func (m Model) selected() (quadlet.Unit, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.filtered) {
		return quadlet.Unit{}, false
	}
	return m.filtered[c], true
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
	chrome := 4 // title + blank + legend + status
	if m.mode == modeList {
		chrome++ // the list legend is two lines (views + actions)
	}
	if m.showHelp {
		chrome += 6
	}
	if m.mode == modeList && len(m.stale) > 0 {
		chrome++ // reload banner
	}
	if m.mode == modeList && (m.filtering || m.filterStr != "") {
		chrome++ // filter line
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
		state := "live"
		if m.sess == nil {
			state = "snapshot"
		} else if !m.following {
			state = "paused"
		}
		b.WriteString(headerStyle.Render(" LOGS " + unit + "  (" + state + ", q to go back)"))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeUpdates:
		b.WriteString(headerStyle.Render(" AUTO-UPDATE "))
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
	switch m.mode {
	case modeLogs:
		return logsKeys()
	case modeFile:
		return viewKeys()
	case modeUpdates:
		return updatesKeys()
	}
	return listKeys()
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
	if m.showHelp {
		m.help.ShowAll = true
		lines := []string{
			m.help.View(m.keys()) + "  ·  " + linger,
			"Linger keeps rootless containers running after logout — enable it once on every quadlet host (loginctl enable-linger).",
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e enables the unit to start at boot (with --now); d removes it from boot (the container keeps running).",
			"x stops the unit; quadlet runs containers with --rm, so stopping removes the container (state lives in volumes).",
			"Quadlet search order: " + strings.Join(quadlet.SearchDirs(), " → "),
		}
		return helpStyle.Render(clampLines(strings.Join(lines, "\n"), m.help.Width()))
	}

	// Compact legend: full words, one line for views + one line for actions.
	var bar string
	switch m.mode {
	case modeFile:
		bar = "E edit · ↑/↓ scroll · esc/q back"
	case modeLogs:
		bar = "f pause/resume · ↑/↓ scroll · esc/q back"
	case modeUpdates:
		bar = "U toggle timer · r refresh · esc/q back"
	default:
		bar = "enter file · l logs · / filter · E edit · u updates · ? all keys\n" +
			"s start · x stop · r restart · e enable · d disable · R reload · L linger · q quit"
	}
	bar += "  ·  " + linger
	return helpStyle.Render(clampLines(bar, m.help.Width()))
}

// clampLines truncates every line to w columns; bubbles/help only truncates
// while an ellipsis still fits and then happily overflows, so we clamp here.
func clampLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	ls := strings.Split(s, "\n")
	for i, l := range ls {
		if ansi.StringWidth(l) > w {
			ls[i] = ansi.Truncate(l, w, "…")
		}
	}
	return strings.Join(ls, "\n")
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
