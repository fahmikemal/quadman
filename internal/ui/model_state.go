package ui

import (
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/tree"

	"github.com/fahmikemal/quadman/internal/compartment"
	"github.com/fahmikemal/quadman/internal/config"
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
	health   map[string]string
	// marks holds marked unit names for bulk actions (space toggles).
	marks       map[string]bool
	podmanInfo  map[string]podman.Entry
	podmanTried bool
	stale       []quadlet.Unit
	loading     bool
	pollCount   int
	filtering   bool
	filterStr   string
	filterIn    textinput.Model
}

// logState is the journal follow tail: live lines, filters, and search.
type logState struct {
	sess          *logSession
	logLines      []string
	logPriority   string // "", "err", "warning", "info"
	logFilter     string // live grep filter
	logFilterIn   textinput.Model
	filteringLogs bool
	following     bool
	searchIn      textinput.Model
	searching     bool
	searchStr     string
	searchMatches int
	matchPos      int
}

// detailState is the unified detail view tab (source/status/journal/inspect).
type detailState struct {
	tab int
}

// updateState is the auto-update screen (podman dry-run + timer).
type updateState struct {
	updateEntries []podman.AutoUpdateEntry
	timerEnabled  string
	timerActive   string
}

// eventState is the live `podman events` stream.
type eventState struct {
	eventSess  *logSession
	eventLines []string
}

// generateState is the podlet Quadlet generator prompt and preview.
type generateState struct {
	generating bool
	genIn      textinput.Model
	genContent string
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

// treeState is the dependency tree view.
type treeState struct {
	treeModel tree.Model
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
}

// sessionState is the first-use editor picker and persisted config.
type sessionState struct {
	cfg           config.Config
	pickingEditor bool
	editorChoices []editorChoice
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
}

// paletteState is the command palette overlay (Ctrl+P).
type paletteState struct {
	paletteIn       textinput.Model
	paletteCursor   int
	paletteActions  []paletteAction
	paletteFiltered []paletteAction
	prevMode        mode
}

// compartmentState runs CLI calls outside the operator's own session:
// SSH target (--ssh) or sudo compartment (--as).
type compartmentState struct {
	ssh remote.Runner
	// comp is the active compartment (sudo target user); compOn reports it.
	// compList holds configured compartment names for the palette switcher.
	comp     compartment.Compartment
	compOn   bool
	compList []string
}
