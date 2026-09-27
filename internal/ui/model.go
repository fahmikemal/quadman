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
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/loginctl"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/systemd"
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
	modeTimers  // systemd timer entities (T)
	modeSecrets // podman secret store (K)
	modePalette // command palette (Ctrl+P)
	modeStats   // podman stats resource screen (o)
)

// Detail tabs, cycled with [ and ].
const (
	tabSource = iota
	tabStatus
	tabJournal
	tabInspect
)

// Model is the quadman Bubble Tea model. Screen state lives in the
// per-concern groups in model_state.go, embedded here so m.field keeps
// working through promotion; mode stays top-level as the discriminator.
type Model struct {
	sys      *systemd.Systemd
	lc       *loginctl.Loginctl
	mode     mode
	table    table.Model
	viewport viewport.Model
	help     help.Model
	spinner  spinner.Model

	// linger reports loginctl enable-linger for the target session.
	linger      bool
	lingerKnown bool

	listState
	logState
	detailState
	updateState
	eventState
	generateState
	execState
	envState
	treeState
	cacheState
	templateState
	confirmState
	sessionState
	statusBarState
	paletteState
	compartmentState
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
	ei := textinput.New()
	ei.Prompt = ""
	pi := textinput.New()
	pi.Prompt = ""
	lfi := textinput.New()
	lfi.Prompt = ""
	cfg, cfgErr := config.Load()
	m := Model{
		sys:      systemd.New(),
		lc:       loginctl.New(),
		table:    t,
		viewport: vp,
		help:     help.New(),
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		listState: listState{
			filterIn: fi,
			status:   map[string]systemd.Status{},
			images:   map[string]string{},
			health:   map[string]string{},
			loading:  true,
		},
		logState:       logState{searchIn: si, logFilterIn: lfi},
		paletteState:   paletteState{paletteIn: pi},
		templateState:  templateState{instanceIn: ii},
		generateState:  generateState{genIn: gi},
		execState:      execState{execIn: ei},
		cacheState:     cacheState{inspect: &quadlet.InspectCache{}},
		envState:       envState{generator: quadlet.GeneratorBinary()},
		sessionState:   sessionState{cfg: cfg},
		statusBarState: statusBarState{readonly: cfg.Readonly(), custom: cfg.Settings.CustomCommands},
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
	if bad := customKeyConflicts(m.custom); len(bad) > 0 {
		m.setStatus("custom command(s) shadowed by built-in keys (never fire): "+strings.Join(bad, ", "), true)
	}
	m.compList = cfg.Settings.Compartments
	return m
}

func fileMtime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}
