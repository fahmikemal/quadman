package ui

import (
	"time"

	"charm.land/bubbles/v2/textinput"

	"github.com/fahmikemal/quadman/internal/compartment"
	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// This file groups Model's state into one embedded struct per screen or
// concern. Model embeds them all, so existing m.field reads and writes
// keep compiling through promotion — only composite literals (New)
// must nest the groups. Add new state to the group that owns it.

// listState is the unit-list screen: discovery results plus filter.
type listState struct {
	units    []quadlet.Unit
	filtered []quadlet.Unit
	images   map[string]string
	status   map[string]systemd.Status
	// health maps container name → healthcheck state (podman ps).
	health map[string]string
	// ctrNames maps a unit to its running container's real name, from
	// the PODMAN_SYSTEMD_UNIT label (ContainerName= aware).
	ctrNames map[string]string
	// ctrOverride maps a unit to its ContainerName= value (from the file),
	// the fallback identity while the container is not running.
	ctrOverride map[string]string
	// marks holds marked unit names for bulk actions (space toggles).
	marks       map[string]bool
	podmanInfo  map[string]podman.Entry
	podmanTried bool
	stale       []quadlet.Unit
	loading     bool
	pollCount   int
	// refreshInFlight keeps poll ticks from stacking refreshes on a slow
	// session; lastRefresh drops results older than the last one applied.
	refreshInFlight bool
	lastRefresh     time.Time
	filtering       bool
	filterStr       string
	filterIn        textinput.Model

	// rowView is the render model of the table rows (same order as filtered).
	rowView []unitRow
	// listOff is the first visible row of the unit table.
	listOff int
	// sortBy is the S-cycled table order (problems first by default).
	sortBy listSort
	// dashOff collapses the tiles + attention block to one summary line
	// (z toggles; config dashboard: false starts collapsed).
	dashOff bool
}

// logState is the journal follow tail: live lines, filters, and search.
type logState struct {
	sess          *logSession
	logLines      []string
	logFmt        []string // logLines rendered for display (same indexes)
	logPriority   string   // "", "err", "warning", "info"
	logFilter     string   // live grep filter
	logFilterIn   textinput.Model
	filteringLogs bool
	following     bool
	searchIn      textinput.Model
	searching     bool
	searchStr     string
	searchMatches int
	matchPos      int
	// logCap bounds the follow buffer; logTail is the snapshot size of a
	// new follow stream (config log_buffer / log_tail).
	logCap  int
	logTail int
}

// detailState is the unified detail view tab (source/status/journal/inspect).
type detailState struct {
	tab int
}

// updateState is the auto-update screen (podman dry-run + timer).
type updateState struct {
	updateEntries  []podman.AutoUpdateEntry
	timerEnabled   string
	timerActive    string
	updateTimer    systemd.TimerInfo
	updatesApplied bool
}

// eventState is the live `podman events` stream.
type eventState struct {
	eventSess *logSession
}

// generateState is the podlet Quadlet generator prompt and preview.
type generateState struct {
	generating bool
	genIn      textinput.Model
	genContent string
	genInput   string
	genFiles   []genFile
	genTab     int
	genChecks  []genCheck
	genBoot    bool // add [Install] WantedBy=default.target on write
	genStart   bool // start the container units after writing
}

// execState is the interactive exec (X) container-name prompt.
type execState struct {
	execing bool
	execIn  textinput.Model
}

// envState is validation + environment info for the problems view.
type envState struct {
	issues        unitIssues
	generator     string
	podmanVersion string
}

// cacheState memoizes quadlet.Inspect by file mtime so the 2.5s poll
// only re-parses files that changed on disk.
type cacheState struct {
	inspect *quadlet.InspectCache
}

// templateState is template-unit instantiation (web@.container).
type templateState struct {
	instancing bool
	instanceIn textinput.Model
}

// confirmState holds the armed generic y/N pending action
// (stop / enable / disable).
type confirmState struct {
	pending *pendingAction
	// armedFor is the pending action the modal was last prepared for.
	armedFor *pendingAction
	confirm  confirmUI
	// confirming is set while a confirmed action builds its commands, so
	// they record the confirmation they passed (see actionCmdHint).
	confirming *confirmGuard
}

// sessionState is the first-use editor picker and persisted config.
type sessionState struct {
	cfg           config.Config
	pickingEditor bool
	editorChoices []editorChoice
	// editTarget overrides the file the next editor launch opens.
	editTarget editTarget
}

// statusBarState is the app chrome: busy spinner, status line, window
// size, refresh cadence, session action log, and global switches.
type statusBarState struct {
	busy     bool
	busyText string

	width, height int
	statusLine    string
	statusErr     bool
	statusAt      time.Time
	showHelp      bool

	// pollInterval overrides the default refresh tick (0 = default).
	pollInterval time.Duration
	// lastInput is the last key or click and blurred whether the terminal
	// reported losing focus: both slow the poll down (see idle). tickGen
	// numbers the poll chain so a wake can replace the slow one.
	lastInput time.Time
	blurred   bool
	tickGen   uint64

	// actions is the recent-actions log of state-changing results.
	actions []actionRecord

	// readonly refuses every state-changing action with an explanation.
	readonly bool

	// custom commands from the YAML config (key -> command).
	custom []config.CustomCommand

	// mouse enables click-to-select (opt-in; off by default so text
	// selection keeps working).
	mouse bool

	// clientInfo identifies the connected client when served via Wish SSH.
	clientInfo string

	// noEditor disables local $EDITOR launching (SSH server sessions).
	noEditor bool

	// system targets system-wide (rootful) Quadlets instead of user units.
	system bool

	// whoami is "user@host" of the local session, for the top bar.
	whoami string

	// trigger describes how the current action was started ("key r",
	// "palette"); busyHow freezes it when the action goes in flight, and
	// pendingHow when it waits for confirmation.
	trigger    string
	busyHow    string
	pendingHow string
}

// paletteState is the command palette overlay (Ctrl+P).
type paletteState struct {
	paletteIn       textinput.Model
	paletteCursor   int
	paletteActions  []paletteAction
	paletteFiltered []paletteAction
	prevMode        mode
}

// screenData holds what the secondary screens loaded; specs are derived
// from it on every render.
type screenData struct {
	storage  storageData
	timers   timersData
	secrets  secretsData
	stats    statsData
	events   eventsData
	treeView treeViewState
}

// compartmentState runs CLI calls outside the operator's own session:
// SSH target (--ssh) or sudo compartment (--as).
type compartmentState struct {
	ssh remote.Runner
	// pm and pl are the podman and podlet clients bound to ssh.
	pm podman.Client
	pl podlet.Client
	// epoch numbers the session target; setSession bumps it, and results
	// stamped with an older epoch are dropped.
	epoch uint64
	// comp is the active compartment (sudo target user); compOn reports it.
	// compList holds configured compartment names for the palette switcher.
	comp     compartment.Compartment
	compOn   bool
	compList []string
}
