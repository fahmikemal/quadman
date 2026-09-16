// Package ui implements the quadman terminal interface.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/tree"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kemal-labs/quadman/internal/config"
	"github.com/kemal-labs/quadman/internal/loginctl"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/systemd"
)

type mode int

const (
	modeList   mode = iota
	modeDetail      // unified detail view with tabs: source / status / journal / inspect
	modeUpdates
	modeValidate
	modeTree
	modeStorage
	modeEvents
	modeGenerate
)

// Detail tabs, cycled with [ and ].
const (
	tabSource = iota
	tabStatus
	tabJournal
	tabInspect
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
	issues      []quadlet.Issue
	version     string
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

type statusMsg struct {
	content string
}

type inspectMsg struct {
	content string
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

// pendingAction is an armed destructive/config-changing action waiting for
// a y/N confirmation.
type pendingAction struct {
	verb string // "stop", "enable", "disable"
	unit quadlet.Unit
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

	// logs search
	searchIn      textinput.Model
	searching     bool
	searchStr     string
	searchMatches int
	matchPos      int

	// follow logs
	sess      *logSession
	logLines  []string
	following bool
	pollCount int

	// updates screen
	updateEntries []podman.AutoUpdateEntry
	timerEnabled  string
	timerActive   string

	// detail view tab state
	tab int

	// events stream
	eventSess  *logSession
	eventLines []string

	// generate via podlet
	generating bool
	genIn      textinput.Model
	genContent string

	// validation + environment info
	issues        unitIssues
	generator     string
	podmanVersion string

	// dependency tree
	treeModel tree.Model

	// template instantiation
	instancing bool
	instanceIn textinput.Model

	// stop confirmation
	confirmStop bool

	// pending is an armed confirmation (stop / enable / disable) waiting
	// for a y/N answer.
	pending *pendingAction

	// first-use editor picker
	cfg           config.Config
	pickingEditor bool
	editorChoices []editorChoice

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
	vp.HighlightStyle = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0"))
	vp.SelectedHighlightStyle = lipgloss.NewStyle().Background(lipgloss.Color("214")).Foreground(lipgloss.Color("0")).Bold(true)
	vp.LeftGutterFunc = func(ctx viewport.GutterContext) string {
		if ctx.Soft {
			return "     "
		}
		return fmt.Sprintf("%4d ", ctx.Index+1)
	}
	fi := textinput.New()
	fi.Placeholder = "filter units…"
	si := textinput.New()
	si.Placeholder = "search logs (regex ok)…"
	ii := textinput.New()
	ii.Placeholder = "instance name (e.g. prod)…"
	gi := textinput.New()
	gi.Placeholder = "docker run … or compose file path"
	cfg, _ := config.Load()
	return Model{
		sys:        systemd.New(),
		lc:         loginctl.New(),
		table:      t,
		viewport:   vp,
		help:       help.New(),
		spinner:    spinner.New(spinner.WithSpinner(spinner.Dot)),
		filterIn:   fi,
		searchIn:   si,
		cfg:        cfg,
		generator:  quadlet.GeneratorBinary(),
		instanceIn: ii,
		genIn:      gi,
		status:     map[string]systemd.Status{},
		images:     map[string]string{},
		health:     map[string]string{},
		loading:    true,
	}
}

// Run starts the quadman TUI.
func Run() error {
	_, err := tea.NewProgram(New()).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(m.sys, m.lc, enrichFull), pollCmd())
}

func pollCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// enrichLevel controls which podman-powered data a refresh gathers.
type enrichLevel int

const (
	enrichNone   enrichLevel = iota
	enrichHealth             // container health only (periodic poll)
	enrichFull               // quadlet list grouping + health (startup, daemon-reload)
)

// healthPollEvery is how many polls pass between health refreshes — podman
// ps is too heavy to run on every 2.5s tick.
const healthPollEvery = 4

func refreshCmd(sys *systemd.Systemd, lc *loginctl.Loginctl, level enrichLevel) tea.Cmd {
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
		var issues []quadlet.Issue
		var version string
		if level >= enrichHealth && podman.Available() {
			if hm, herr := podman.PsHealth(ctx); herr == nil {
				health = hm
			}
		}
		if level == enrichFull {
			if podman.Available() {
				if entries, perr := podman.QuadletList(ctx); perr == nil {
					pinfo = podman.ByUnit(entries)
				}
			}
			issues, _ = quadlet.Validate(ctx, quadlet.SearchDirs())
			if ver, verr := podman.Version(ctx); verr == nil {
				version = ver
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
			podmanTried: level == enrichFull,
			health:      health,
			stale:       stale,
			issues:      issues,
			version:     version,
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
		if u.UnitName != "" { // .quadlets bundles map to N units, not one
			names = append(names, u.UnitName)
		}
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
		// Poll: refresh the list. Full podman enrichment runs once (or after
		// daemon-reload); health refreshes on a slower cadence.
		m.pollCount++
		level := enrichNone
		if !m.podmanTried {
			level = enrichFull
		} else if m.pollCount%healthPollEvery == 0 {
			level = enrichHealth
		}
		return m, tea.Batch(refreshCmd(m.sys, m.lc, level), pollCmd())

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
			status += " - " + msg.hint
		}
		m.setStatus(status, false)
		return m, refreshCmd(m.sys, m.lc, enrichHealth)

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
		m.mode = modeDetail
		m.tab = tabJournal
		m.following = false
		m.resize()
		m.clearStatus()
		return m, nil

	case logLineMsg:
		if msg.sess == m.eventSess && m.eventSess != nil {
			m.eventLines = append(m.eventLines, msg.line)
			if len(m.eventLines) > logBufferCap {
				m.eventLines = m.eventLines[len(m.eventLines)-logBufferCap:]
			}
			if m.mode == modeEvents {
				wasAtBottom := m.viewport.AtBottom()
				m.viewport.SetContent(strings.Join(m.eventLines, "\n"))
				if m.following && wasAtBottom {
					m.viewport.GotoBottom()
				}
			}
			return m, followLine(m.eventSess)
		}
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

	case storageMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("system df: "+msg.err.Error(), true)
			return m, nil
		}
		m.mode = modeStorage
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		m.resize()
		return m, nil

	case generateMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("generate: "+msg.err.Error(), true)
			return m, nil
		}
		m.genContent = msg.content
		m.mode = modeGenerate
		m.viewport.SetContent(msg.content)
		m.viewport.GotoTop()
		m.resize()
		return m, nil

	case statusMsg:
		m.busy = false
		if m.tab == tabStatus {
			m.viewport.SetContent(msg.content)
			m.viewport.GotoTop()
		}
		return m, nil

	case inspectMsg:
		m.busy = false
		if m.tab == tabInspect {
			m.viewport.SetContent(msg.content)
			m.viewport.GotoTop()
		}
		return m, nil

	case editorFinishedMsg:
		if msg.err != nil {
			m.setStatus("editor: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.changed {
			m.setStatus("file changed - press R to regenerate", false)
		} else {
			m.setStatus("no changes", false)
		}
		if m.mode == modeDetail && m.tab == tabSource {
			if u, ok := m.selected(); ok {
				if content, rerr := os.ReadFile(u.Path); rerr == nil {
					m.viewport.SetContent(withDropins(u, content))
				}
			}
		}
		return m, refreshCmd(m.sys, m.lc, enrichNone)

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

	case tea.MouseClickMsg:
		return m.handleMouse(msg)

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

	// The first-use editor picker owns all keys until answered or cancelled.
	if m.pickingEditor {
		return m.pickEditor(msg)
	}

	// An armed confirmation (stop/enable/disable) swallows the next key.
	if m.pending != nil {
		p := m.pending
		m.pending = nil
		if msg.String() != "y" {
			m.setStatus(p.verb+" cancelled", false)
			return m, nil
		}
		return m.runPending(p)
	}

	// While the log search input is focused, keys edit the search query.
	if m.searching {
		switch msg.String() {
		case "enter":
			m.searching = false
			m.searchIn.Blur()
			return m, nil
		case "esc":
			m.clearSearch()
			return m, nil
		}
		var cmd tea.Cmd
		m.searchIn, cmd = m.searchIn.Update(msg)
		if v := m.searchIn.Value(); v != m.searchStr {
			m.searchStr = v
			m.applySearch()
		}
		return m, cmd
	}

	// While the instance-name input is focused, keys edit the name.
	if m.instancing {
		switch msg.String() {
		case "enter":
			m.instancing = false
			m.instanceIn.Blur()
			name := strings.TrimSpace(m.instanceIn.Value())
			m.instanceIn.SetValue("")
			if name == "" {
				m.setStatus("instantiate cancelled (empty name)", false)
				return m, nil
			}
			return m.instantiateUnit(name)
		case "esc":
			m.instancing = false
			m.instanceIn.SetValue("")
			m.instanceIn.Blur()
			m.setStatus("instantiate cancelled", false)
			return m, nil
		}
		var cmd tea.Cmd
		m.instanceIn, cmd = m.instanceIn.Update(msg)
		return m, cmd
	}

	// While the generate input is focused, keys edit the command/path.
	if m.generating {
		switch msg.String() {
		case "enter":
			m.generating = false
			m.genIn.Blur()
			input := strings.TrimSpace(m.genIn.Value())
			m.genIn.SetValue("")
			if input == "" {
				m.setStatus("generate cancelled (empty input)", false)
				return m, nil
			}
			return m, tea.Batch(m.setBusy("podlet generate"), generateCmd(input))
		case "esc":
			m.generating = false
			m.genIn.SetValue("")
			m.genIn.Blur()
			m.setStatus("generate cancelled", false)
			return m, nil
		}
		var cmd tea.Cmd
		m.genIn, cmd = m.genIn.Update(msg)
		return m, cmd
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

	case modeStorage:
		switch msg.String() {
		case "esc", "q":
			m.mode = modeList
			m.resize()
			return m, nil
		case "r":
			return m, tea.Batch(m.setBusy("system df"), storageCmd())
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modeEvents:
		switch msg.String() {
		case "esc", "q":
			m.stopEvents()
			m.mode = modeList
			m.resize()
			return m, nil
		case "f":
			m.following = !m.following
			if m.following {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modeGenerate:
		switch msg.String() {
		case "esc", "q":
			m.mode = modeList
			m.resize()
			return m, nil
		case "y":
			return m.installGenerated()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modeValidate:
		if msg.String() == "esc" || msg.String() == "q" {
			m.mode = modeList
			m.resize()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modeTree:
		if msg.String() == "esc" || msg.String() == "q" {
			m.mode = modeList
			m.resize()
			return m, nil
		}
		var cmd tea.Cmd
		m.treeModel, cmd = m.treeModel.Update(msg)
		return m, cmd

	case modeDetail:
		switch msg.String() {
		case "esc", "q":
			m.stopLogs()
			m.clearSearch()
			m.mode = modeList
			m.resize()
			return m, nil
		case "[":
			return m.cycleTab(-1)
		case "]":
			return m.cycleTab(1)
		case "l":
			if m.tab != tabJournal {
				return m.cycleTab(tabJournal - m.tab)
			}
			return m, nil
		case "f":
			if m.tab == tabJournal && m.sess != nil {
				m.following = !m.following
				if m.following {
					m.viewport.GotoBottom()
				}
			}
			return m, nil
		case "/":
			if m.tab == tabJournal {
				m.searching = true
				m.searchIn.Focus()
				return m, textinput.Blink
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case "n":
			if m.tab == tabJournal {
				m.nextMatch()
			}
			return m, nil
		case "N":
			if m.tab == tabJournal {
				m.prevMatch()
			}
			return m, nil
		case "E":
			return m.editSelected()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	// .quadlets bundles get their own small action set: preview + install.
	if u, ok := m.selected(); ok && u.Kind == quadlet.KindQuadlets {
		switch msg.String() {
		case "enter":
			content, err := os.ReadFile(u.Path)
			if err != nil {
				m.setStatus(err.Error(), true)
				return m, nil
			}
			m.viewport.SetContent(withQuadletDocs(content))
			m.viewport.GotoTop()
			m.mode = modeDetail
			m.tab = tabSource
			m.resize()
			return m, nil
		case "I":
			m.pending = &pendingAction{verb: "install", unit: u}
			m.setStatus("install "+filepath.Base(u.Path)+" via podman quadlet install? [y/N]", false)
			return m, nil
		case "s", "x", "r", "e", "d", "l", "h", "E":
			m.setStatus("install the bundle first (I) to manage its units", false)
			return m, nil
		}
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
		m.pending = &pendingAction{verb: "stop", unit: u}
		m.setStatus("stop "+u.UnitName+"? container will be removed (quadlet runs --rm) [y/N]", false)
		return m, nil

	case "e":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		// Boot start is declarative in quadlet: [Install] WantedBy= in the
		// file, and the generator wires it up on daemon-reload. Newer
		// systemd refuses `systemctl enable` for generated units.
		if f, err := quadlet.Parse(u.Path); err == nil && f.BootTarget() != "" {
			sys := m.sys
			return m, tea.Batch(m.setBusy("start "+u.UnitName),
				actionCmd("start "+u.UnitName, func(ctx context.Context) (string, error) {
					return sys.UnitAction(ctx, "start", u.UnitName)
				}))
		}
		m.pending = &pendingAction{verb: "enable", unit: u}
		m.setStatus("enable at boot: append [Install] WantedBy=default.target to "+filepath.Base(u.Path)+"? [y/N]", false)
		return m, nil

	case "d":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		f, err := quadlet.Parse(u.Path)
		if err != nil || f.BootTarget() == "" {
			m.setStatus(u.Name+" is not enabled at boot", false)
			return m, nil
		}
		m.pending = &pendingAction{verb: "disable", unit: u}
		m.setStatus("disable at boot: remove [Install] from "+filepath.Base(u.Path)+"? [y/N]", false)
		return m, nil

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

	case "g":
		return m, tea.Batch(m.setBusy("system df"), storageCmd())

	case "w":
		return m.startEvents()

	case "n":
		if !podletAvailable() {
			m.setStatus("podlet not found — install it to generate quadlets (github.com/containers/podlet)", false)
			return m, nil
		}
		m.generating = true
		m.genIn.Focus()
		return m, textinput.Blink

	case "v":
		m.mode = modeValidate
		m.viewport.SetContent(m.validateView())
		m.viewport.GotoTop()
		m.resize()
		return m, nil

	case "t":
		m.treeModel = tree.New(buildTree(m.units, nil), 80, 20)
		m.mode = modeTree
		m.resize()
		return m, nil

	case "i":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if !strings.HasSuffix(u.Name, "@") {
			m.setStatus(u.Name+" is not a template (templates end with @)", false)
			return m, nil
		}
		m.instancing = true
		m.instanceIn.Focus()
		return m, textinput.Blink

	case "D":
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.pending = &pendingAction{verb: "delete", unit: u}
		m.setStatus("delete "+filepath.Base(u.Path)+"? the unit stops and the file is removed [y/N]", false)
		return m, nil

	case "y":
		return m.copySelection("name")

	case "Y":
		return m.copySelection("image")

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
		m.viewport.SetContent(withDropins(u, content))
		m.viewport.GotoTop()
		m.mode = modeDetail
		m.tab = tabSource
		m.resize()
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// runPending executes an armed y/N-confirmed action.
func (m Model) runPending(p *pendingAction) (tea.Model, tea.Cmd) {
	sys := m.sys
	switch p.verb {
	case "enable":
		return m, tea.Batch(m.setBusy("enable at boot "+p.unit.UnitName),
			actionCmdHint("enable at boot "+p.unit.UnitName, "starts on login from now on", func(ctx context.Context) (string, error) {
				changed, err := quadlet.EnsureBootTarget(p.unit.Path, "default.target")
				if err != nil {
					return "", err
				}
				if changed {
					if _, err := sys.DaemonReload(ctx); err != nil {
						return "", err
					}
				}
				return sys.UnitAction(ctx, "start", p.unit.UnitName)
			}))
	case "disable":
		return m, tea.Batch(m.setBusy("disable at boot "+p.unit.UnitName),
			actionCmdHint("disable at boot "+p.unit.UnitName, "still running now", func(ctx context.Context) (string, error) {
				changed, err := quadlet.RemoveBootTarget(p.unit.Path)
				if err != nil {
					return "", err
				}
				if changed {
					if _, err := sys.DaemonReload(ctx); err != nil {
						return "", err
					}
				}
				return "", nil
			}))
	case "install":
		return m.installBundle(p.unit)
	case "delete":
		return m.deleteUnit(p.unit)
	default: // "stop"
		hint := ""
		if p.unit.Kind == quadlet.KindContainer {
			hint = "container removed; state lives in volumes"
		}
		return m, tea.Batch(m.setBusy("stop "+p.unit.UnitName),
			actionCmdHint("stop "+p.unit.UnitName, hint, func(ctx context.Context) (string, error) {
				return sys.UnitAction(ctx, "stop", p.unit.UnitName)
			}))
	}
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
		if msg.err != nil {
			return m, nil // keep the previous list when a refresh fails
		}
		// A successful discovery of zero units is a valid loaded state,
		// not "keep loading" — the empty-state hint depends on this.
		msg.units = []quadlet.Unit{}
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
	if msg.issues != nil {
		m.issues = indexIssues(msg.issues)
	}
	if msg.version != "" {
		m.podmanVersion = msg.version
	}
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
	nameW, kindW, unitW, stateW, subW := len("QUADLET"), len("KIND"), len("SYSTEMD UNIT"), len("STATE"), len("SUB")
	for _, u := range m.filtered {
		state, sub := m.status[u.UnitName].Display()
		if m.health["systemd-"+u.Name] == "unhealthy" {
			state = "unhealthy"
			sub = "health"
		}
		name := u.Name + m.issues.marker(u.Name)
		rows = append(rows, table.Row{name, string(u.Kind), u.UnitName, state, sub, m.images[u.UnitName]})
		nameW = max(nameW, ansi.StringWidth(name))
		kindW = max(kindW, len(u.Kind))
		unitW = max(unitW, len(u.UnitName))
		stateW = max(stateW, len(state))
		subW = max(subW, len(sub))
	}
	m.table.SetColumns(adaptiveColumns(m.width, nameW, kindW, unitW, stateW, subW))
	m.table.SetRows(rows)
}

// adaptiveColumns sizes the table columns from the widest content per column
// (plus padding) and gives the leftover width to IMAGE, so short values no
// longer leave huge empty gaps. Caps keep one long value from eating the table.
func adaptiveColumns(total, nameW, kindW, unitW, stateW, subW int) []table.Column {
	if total <= 0 {
		total = 100
	}
	const pad = 2
	nameW = clampW(nameW+pad, 9, 34)
	kindW = clampW(kindW+pad, 6, 12)
	unitW = clampW(unitW+pad, 14, 44)
	stateW = clampW(stateW+pad, 7, 14)
	subW = clampW(subW+pad, 5, 13)
	imageW := total - nameW - kindW - unitW - stateW - subW - 8 // separators
	if imageW < 16 {
		imageW = 16
	}
	return []table.Column{
		{Title: "QUADLET", Width: nameW},
		{Title: "KIND", Width: kindW},
		{Title: "SYSTEMD UNIT", Width: unitW},
		{Title: "STATE", Width: stateW},
		{Title: "SUB", Width: subW},
		{Title: "IMAGE", Width: imageW},
	}
}

func clampW(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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

func (m Model) View() tea.View {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" quadman - rootless quadlet manager "))
	b.WriteString("\n")

	switch m.mode {
	case modeList:
		if m.generating {
			b.WriteString(filterStyle.Render("generate from: " + m.genIn.View()))
			b.WriteString("\n")
		}
		if m.instancing {
			b.WriteString(filterStyle.Render("instance name for " + m.selectedName() + " @: " + m.instanceIn.View()))
			b.WriteString("\n")
		}
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
				"⚠ %d quadlet file(s) changed since last daemon-reload - press R (e.g. %s)",
				len(m.stale), m.stale[0].Name)))
		}
	case modeDetail:
		u, ok := m.selected()
		name := ""
		if ok {
			name = u.UnitName
			if e := m.podmanInfo[u.UnitName]; e.App != "" {
				name += " · app: " + e.App
			}
			if e := m.podmanInfo[u.UnitName]; e.Pod != "" {
				name += " · pod: " + e.Pod
			}
		}
		b.WriteString(m.tabBar(name))
		b.WriteString("\n")
		if m.tab == tabJournal && (m.searching || m.searchStr != "") {
			info := ""
			if m.searchStr != "" {
				info = fmt.Sprintf("  (%d/%d matches)", m.matchPos, m.searchMatches)
				if m.searchMatches == 0 {
					info = "  (no matches)"
				}
			}
			b.WriteString(filterStyle.Render("/ " + m.searchIn.View() + info))
			b.WriteString("\n")
		}
		b.WriteString(m.viewport.View())
	case modeStorage:
		b.WriteString(headerStyle.Render(" STORAGE — podman system df "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeEvents:
		b.WriteString(headerStyle.Render(" EVENTS — podman events (live, f pause, q back) "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeGenerate:
		b.WriteString(headerStyle.Render(" GENERATE — preview (y write & reload, esc cancel) "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeValidate:
		b.WriteString(headerStyle.Render(" PROBLEMS "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	case modeTree:
		b.WriteString(headerStyle.Render(" TREE — quadlet dependencies "))
		b.WriteString("\n")
		b.WriteString(m.treeModel.View())
	case modeUpdates:
		b.WriteString(headerStyle.Render(" AUTO-UPDATE "))
		b.WriteString("\n")
		b.WriteString(m.viewport.View())
	}

	b.WriteString("\n")
	b.WriteString(m.helpBar())

	if m.pickingEditor {
		b.WriteString("\n")
		b.WriteString(warnStyle.Render(m.pickerLine()))
	} else if m.busy {
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
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) keys() keyMap {
	switch m.mode {
	case modeDetail:
		return detailKeys()
	case modeEvents:
		return detailKeys()
	case modeUpdates, modeValidate, modeTree, modeStorage, modeGenerate:
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
			"Linger keeps rootless containers running after logout - enable it once on every quadlet host (loginctl enable-linger).",
			"R re-runs systemd's generator after you edit quadlet files, then the list refreshes.",
			"e adds [Install] WantedBy=default.target to the quadlet file so the unit starts at boot; d removes it (newer systemd refuses 'systemctl enable' on generated units).",
			"x stops the unit; quadlet runs containers with --rm, so stopping removes the container (state lives in volumes).",
			"E edits in your editor; the first use asks once and saves the choice to ~/.config/quadman/config.json (delete that file to re-pick).",
			"Quadlet search order: " + strings.Join(quadlet.SearchDirs(), " → "),
		}
		return helpStyle.Render(clampLines(strings.Join(lines, "\n"), m.help.Width()))
	}

	// Compact legend: full words; one line when it fits, two when narrow.
	bar := strings.Join(m.legend(), "\n")
	ls := strings.Split(bar, "\n")
	ls[len(ls)-1] += "  ·  " + linger
	return helpStyle.Render(clampLines(strings.Join(ls, "\n"), m.help.Width()))
}

// legendChipReserve reserves room for the " · linger: on/off" chip when
// deciding whether the full legend fits on one line.
const legendChipReserve = 16

// legend returns the compact key hints for the current mode. In list mode it
// prefers a single line, wrapping to two lines (views, then unit actions)
// only when the terminal is too narrow for the full legend.
func (m Model) legend() []string {
	switch m.mode {
	case modeDetail:
		if m.tab == tabJournal {
			return []string{"[/] tabs · f pause/resume · / search · n/N match · E edit · esc/q back"}
		}
		return []string{"[/] tabs · E edit · ↑/↓ scroll · esc/q back"}
	case modeStorage:
		return []string{"r refresh · ↑/↓ scroll · esc/q back"}
	case modeEvents:
		return []string{"f pause/resume · ↑/↓ scroll · esc/q back"}
	case modeGenerate:
		return []string{"y write & reload · esc cancel · ↑/↓ scroll"}
	case modeUpdates:
		return []string{"U toggle timer · r refresh · esc/q back"}
	}
	w := m.help.Width()
	keys := legendKeys()
	if w > 0 && w < legendFullWidth {
		keys = legendKeysCompact()
	}
	full := strings.Join(keys, " · ")
	if w <= 0 || ansi.StringWidth(full)+legendChipReserve <= w {
		return []string{full}
	}
	return splitLegend(keys, w-legendChipReserve)
}

// legendFullWidth is the terminal width below which the legend falls back
// to the compact subset so nothing is truncated.
const legendFullWidth = 170

// legendKeys returns every list-mode key hint sorted alphabetically by key
// (symbols first, lowercase before uppercase within a letter).
func legendKeys() []string {
	return []string{
		"/ filter", "? all keys",
		"d disable", "D delete",
		"e enable", "E edit",
		"g storage", "h healthcheck",
		"i instantiate", "I install",
		"l logs", "L linger",
		"n generate",
		"q quit",
		"r restart", "R reload",
		"s start",
		"t tree",
		"u updates",
		"v problems",
		"w events",
		"x stop",
		"y/Y copy",
		"enter file",
	}
}

// legendKeysCompact is the essential subset for narrow terminals, also
// alphabetically sorted.
func legendKeysCompact() []string {
	return []string{
		"/ filter", "? all keys",
		"d disable",
		"e enable", "E edit",
		"l logs", "L linger",
		"q quit",
		"r restart", "R reload",
		"s start",
		"t tree",
		"u updates",
		"v problems",
		"x stop",
		"enter file",
	}
}

// splitLegend breaks the sorted key list into two lines, filling the first
// line up to the target width and putting the rest on the second.
func splitLegend(keys []string, target int) []string {
	var first []string
	w := 0
	i := 0
	for ; i < len(keys); i++ {
		kw := ansi.StringWidth(keys[i]) + 3 // separator
		if w > 0 && w+kw > target {
			break
		}
		w += kw
		first = append(first, keys[i])
	}
	if i == 0 || i >= len(keys) {
		return []string{strings.Join(keys, " · ")}
	}
	return []string{strings.Join(first, " · "), strings.Join(keys[i:], " · ")}
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

// selectedName returns the selected unit's name, or "" when none.
func (m Model) selectedName() string {
	if u, ok := m.selected(); ok {
		return u.Name
	}
	return ""
}

// cycleTab moves the detail view one tab in the given direction (+1/-1),
// loading each tab's content lazily.
func (m Model) cycleTab(dir int) (tea.Model, tea.Cmd) {
	m.tab = (m.tab + dir + 4) % 4
	m.clearSearch()
	switch m.tab {
	case tabSource:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		content, err := os.ReadFile(u.Path)
		if err != nil {
			m.setStatus(err.Error(), true)
			return m, nil
		}
		m.viewport.SetContent(withDropins(u, content))
		m.viewport.GotoTop()
		return m, nil
	case tabStatus:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		sys := m.sys
		return m, tea.Batch(m.setBusy("status "+u.UnitName), func() tea.Msg {
			text, _ := sys.StatusText(context.Background(), u.UnitName)
			return statusMsg{content: text}
		})
	case tabJournal:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.sess == nil || m.sess.unit != u.UnitName {
			return m.startLogs(u)
		}
		return m, nil
	case tabInspect:
		u, ok := m.selected()
		if !ok {
			return m, nil
		}
		if u.Kind != quadlet.KindContainer {
			m.viewport.SetContent("podman inspect only applies to container units\n")
			m.viewport.GotoTop()
			return m, nil
		}
		container := "systemd-" + u.Name
		return m, tea.Batch(m.setBusy("inspect "+container), func() tea.Msg {
			text, err := podman.Inspect(context.Background(), container)
			if err != nil {
				text = err.Error()
			}
			return inspectMsg{content: text}
		})
	}
	return m, nil
}

// tabBar renders the detail view's tab strip with the active tab highlighted
// and the unit name next to it.
func (m Model) tabBar(unit string) string {
	tabs := []string{"source", "status", "journal", "inspect"}
	var b strings.Builder
	for i, t := range tabs {
		if i == m.tab {
			b.WriteString(tabActiveStyle.Render(" " + t + " "))
		} else {
			b.WriteString(tabInactiveStyle.Render(" " + t + " "))
		}
		if i < len(tabs)-1 {
			b.WriteString(" ")
		}
	}
	if unit != "" {
		b.WriteString("  " + helpStyle.Render(unit))
	}
	if m.tab == tabJournal {
		state := "live"
		if m.sess == nil {
			state = "snapshot"
		} else if !m.following {
			state = "paused"
		}
		b.WriteString(helpStyle.Render("  (" + state + ")"))
	}
	return b.String()
}
