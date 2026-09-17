package ui

import (
	"fmt"
	"os"
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

	"github.com/kemal-labs/quadman/internal/config"
	"github.com/kemal-labs/quadman/internal/loginctl"
	"github.com/kemal-labs/quadman/internal/podlet"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/remote"
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
	modeRecent  // recent-actions log (A)
	modePalette // command palette (Ctrl+P)
)

// Detail tabs, cycled with [ and ].
const (
	tabSource = iota
	tabStatus
	tabJournal
	tabInspect
)

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
	// marks holds marked unit names for bulk actions (space toggles).
	marks       map[string]bool
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
	sess          *logSession
	logLines      []string
	logPriority   string // "", "err", "warning", "info"
	logFilter     string // live grep filter
	logFilterIn   textinput.Model
	filteringLogs bool
	following     bool
	pollCount     int

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

	// inspectCache memoizes quadlet.Inspect by file mtime so the 2.5s poll
	// only re-parses files that changed on disk.
	inspect *quadlet.InspectCache

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
	statusAt      time.Time
	showHelp      bool

	// pollInterval overrides the default refresh tick (0 = default).
	pollInterval time.Duration

	// recent-actions log of state-changing results this session.
	actions []actionRecord

	// readonly refuses every state-changing action with an explanation.
	readonly bool

	// custom commands from the YAML config (key -> command).
	custom []config.CustomCommand

	// ssh runs all CLI calls on a remote host (--ssh user@host). Zero value
	// means local execution.
	ssh remote.Runner

	// mouse enables click-to-select (opt-in via config or --mouse; off by
	// default so text selection keeps working).
	mouse bool

	// clientInfo identifies the connected client when served via Wish SSH.
	clientInfo string

	// noEditor disables local $EDITOR launching (e.g. in SSH server sessions).
	noEditor bool

	// system targets the system-wide (rootful) Quadlet session instead of user session.
	system bool

	// command palette (Ctrl+P)
	paletteIn       textinput.Model
	paletteCursor   int
	paletteActions  []paletteAction
	paletteFiltered []paletteAction
	prevMode        mode
}

// statusTTL is how long a success notification stays before it fades.
// Errors stay until something else replaces them.
const statusTTL = 5 * time.Second

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
	// No placeholders: the first placeholder character renders under the
	// bright block cursor while the rest stays dim, which reads as a stray
	// letter. The line prefixes already give the context.
	fi := textinput.New()
	fi.Prompt = ""
	si := textinput.New()
	si.Prompt = ""
	ii := textinput.New()
	ii.Prompt = ""
	gi := textinput.New()
	gi.Prompt = ""
	pi := textinput.New()
	pi.Prompt = ""
	lfi := textinput.New()
	lfi.Prompt = ""
	cfg, cfgErr := config.Load()
	m := Model{
		sys:         systemd.New(),
		lc:          loginctl.New(),
		table:       t,
		viewport:    vp,
		help:        help.New(),
		spinner:     spinner.New(spinner.WithSpinner(spinner.Dot)),
		filterIn:    fi,
		searchIn:    si,
		paletteIn:   pi,
		logFilterIn: lfi,
		cfg:         cfg,
		generator:   quadlet.GeneratorBinary(),
		instanceIn:  ii,
		genIn:       gi,
		inspect:     &quadlet.InspectCache{},
		status:      map[string]systemd.Status{},
		images:      map[string]string{},
		health:      map[string]string{},
		loading:     true,
		readonly:    cfg.Readonly(),
		custom:      cfg.Settings.CustomCommands,
	}
	m.pollInterval = cfg.RefreshInterval()
	m.mouse = cfg.Settings.Mouse
	applyExtraDirs(cfg.Settings.QuadletDirs)
	applyTheme(resolveTheme(cfg.Settings.Theme))
	if cfg.LogBuffer() != config.DefaultLogBuffer {
		logBufferCap = cfg.LogBuffer()
	}
	if cfg.LogTail() != config.DefaultLogTail {
		logTailLines = cfg.LogTail()
	}
	if cfg.Settings.System {
		m.system = true
		m.sys.User = false
	}
	if cfgErr != nil {
		m.setStatus("config: "+cfgErr.Error(), true)
	}
	return m
}

// Options are the CLI-level overrides applied on top of config.yaml.
type Options struct {
	Readonly    bool
	SSH         string
	Mouse       bool
	Theme       string
	QuadletDirs []string
	ClientInfo  string
	NoEditor    bool
	System      bool
}

// Run starts the quadman TUI.
func Run() error {
	return RunWith(false)
}

// RunWith starts the quadman TUI, forcing readonly mode when readonly is
// true (from the --readonly flag).
func RunWith(readonly bool) error {
	return RunWithOptions(Options{Readonly: readonly})
}

// NewWithOptions returns a configured Model with CLI overrides applied.
func NewWithOptions(o Options) Model {
	m := New()
	if o.SSH != "" {
		m.applySSH(o.SSH)
	}
	if o.Readonly {
		m.readonly = true
	}
	if o.System {
		m.system = true
		m.sys.User = false
	}
	if o.Mouse {
		m.mouse = true
	}
	if o.Theme != "" {
		applyTheme(resolveTheme(o.Theme))
	}
	if len(o.QuadletDirs) > 0 {
		applyExtraDirs(o.QuadletDirs)
	}
	if o.ClientInfo != "" {
		m.clientInfo = o.ClientInfo
	}
	if o.NoEditor {
		m.noEditor = true
	}
	return m
}

func (m Model) searchDirs() []string {
	return quadlet.SearchDirsMode(m.system)
}

// RunWithOptions starts the quadman TUI with CLI overrides.
func RunWithOptions(o Options) error {
	m := NewWithOptions(o)
	_, err := tea.NewProgram(m).Run()
	return err
}

// applyExtraDirs appends user-configured Quadlet source directories to the
// discovery search path (config.yaml quadlet_dirs plus --quadlet-dir
// flags). Empty and duplicate entries are ignored, so calling it twice
// (once from main, once from New) is safe.
func applyExtraDirs(dirs []string) {
	seen := map[string]bool{}
	for _, d := range quadlet.ExtraDirs {
		seen[d] = true
	}
	for _, d := range dirs {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		quadlet.ExtraDirs = append(quadlet.ExtraDirs, d)
	}
}

// RunWithSSH starts the quadman TUI against a remote host: every CLI call
// (systemctl, journalctl, loginctl, podman, podlet) runs over SSH, and
// file-mutating actions are disabled with an explanation.
func RunWithSSH(target string, readonly bool) error {
	return RunWithOptions(Options{SSH: target, Readonly: readonly})
}

// applySSH points every CLI client at the remote target.
func (m *Model) applySSH(target string) {
	m.ssh = remote.Runner{Target: target}
	m.sys.Remote = m.ssh
	m.lc.Remote = m.ssh
	podman.DefaultRunner = m.ssh
	podlet.DefaultRunner = m.ssh
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(enrichFull), m.pollTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil

	case tickMsg:
		// Fade stale success notifications; errors stay put.
		if m.statusLine != "" && !m.statusErr && time.Since(m.statusAt) > statusTTL {
			m.clearStatus()
		}
		// Poll: refresh the list. Full podman enrichment runs once (or after
		// daemon-reload); health refreshes on a slower cadence.
		m.pollCount++
		level := enrichNone
		if !m.podmanTried {
			level = enrichFull
		} else if m.pollCount%healthPollEvery == 0 {
			level = enrichHealth
		}
		return m, tea.Batch(m.refresh(level), m.pollTick())

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
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(msg.desc+": "+msg.err.Error(), true)
			return m, nil
		}
		status := strings.TrimSpace(msg.desc + " ok " + msg.out)
		if msg.hint != "" {
			status += " - " + msg.hint
		}
		m.setStatus(status, false)
		return m, m.refresh(enrichHealth)

	case bulkMsg:
		m.busy = false
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("%s: %d/%d ok: %s", msg.desc, msg.done, msg.total, msg.err.Error()), true)
			return m, m.refresh(enrichHealth)
		}
		m.clearMarks()
		m.setStatus(fmt.Sprintf("%s: %d/%d ok", msg.desc, msg.done, msg.total), false)
		return m, m.refresh(enrichHealth)

	case logsMsg:
		m.busy = false
		if msg.err != nil {
			m.viewport.SetContent(
				dimErrStyle.Render("journalctl failed for " + msg.unit + "\n\n" + msg.err.Error() +
					"\n\nHint: the user journal may be missing. Is systemd user session running?"))
		} else {
			if msg.out != "" {
				m.logLines = strings.Split(msg.out, "\n")
				m.viewport.SetContent(strings.Join(m.visibleLogLines(), "\n"))
			} else {
				m.viewport.SetContent(msg.out)
			}
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
		m.viewport.SetContent(strings.Join(m.visibleLogLines(), "\n"))
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
			m.recordAction("toggle linger", msg.err)
			m.setStatus("linger: "+msg.err.Error(), true)
			return m, nil
		}
		m.linger = msg.on
		m.lingerKnown = true
		m.recordAction("linger "+onOff(msg.on), nil)
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
		m.recordAction("toggle "+autoUpdateTimer, msg.err)
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

	case customMsg:
		m.busy = false
		m.recordAction(msg.desc, msg.err)
		if msg.err != nil {
			m.setStatus(msg.desc+": "+msg.err.Error(), true)
			return m, nil
		}
		status := strings.TrimSpace(msg.desc + " ok " + msg.out)
		m.setStatus(status, false)
		return m, m.refresh(enrichNone)

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
				if content, rerr := m.readUnitFile(u); rerr == nil {
					m.viewport.SetContent(m.fileContent(u, content))
				}
			}
		}
		return m, m.refresh(enrichNone)

	case healthMsg:
		m.busy = false
		if msg.err != nil {
			m.recordAction("healthcheck "+msg.container, msg.err)
			m.setStatus("healthcheck: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.ok {
			m.recordAction("healthcheck "+msg.container, nil)
			m.setStatus("healthcheck "+msg.container+": healthy", false)
		} else {
			m.recordAction("healthcheck "+msg.container, errUnhealthy)
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

	// Focused text inputs own their keys while open.
	if mm, cmd, ok := m.searchKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.instanceKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.generateKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.filterKeys(msg); ok {
		return mm, cmd
	}

	// Command Palette modal handler or launcher.
	if mm, cmd, ok := m.modePaletteKeys(msg); ok {
		return mm, cmd
	}
	if msg.String() == "ctrl+p" {
		m.openPalette()
		return m, nil
	}

	// One small handler per mode.
	if mm, cmd, ok := m.modeUpdatesKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeStorageKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeEventsKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeGenerateKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeValidateKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeTreeKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeDetailKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.modeRecentKeys(msg); ok {
		return mm, cmd
	}

	// .quadlets bundles get their own small action set: preview + install.
	if mm, cmd, ok := m.bundleKeys(msg); ok {
		return mm, cmd
	}

	// List-mode actions, one handler per key group.
	if mm, cmd, ok := m.quitKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.escKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.helpKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.filterOpenKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.startRestartKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.stopKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.enableKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.disableKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.editKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.updatesOpenKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.healthKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.storageKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.eventsKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.generateOpenKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.problemsKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.openRecentKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.treeKeysOpen(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.instantiateOpenKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.deleteKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.copyNameKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.copyImageKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.reloadKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.lingerKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.openLogsKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.openFileKeys(msg); ok {
		return mm, cmd
	}
	if mm, cmd, ok := m.markKeys(msg); ok {
		return mm, cmd
	}

	// User-defined custom commands from config.yaml (single-char keys).
	if mm, cmd, ok := m.runCustom(msg.String()); ok {
		return mm, cmd
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func fileMtime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}
