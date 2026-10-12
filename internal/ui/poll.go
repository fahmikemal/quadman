package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/loginctl"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// defaultPollInterval is how often the unit list refreshes itself when the
// user has not configured another interval. The cursor stays pinned on the
// same unit (by name) across refreshes.
const defaultPollInterval = 2500 * time.Millisecond

// An unattended screen polls slowly: after idleAfter without a key or a
// click, or at once when the terminal loses focus, the cadence drops to
// idlePollInterval. The next key, click, or focus refreshes immediately
// and restores the configured cadence.
const (
	idleAfter        = time.Minute
	idlePollInterval = 15 * time.Second
)

// lingerPollEvery is how many polls pass between linger reads: linger only
// changes through loginctl, and L reports its own result.
const lingerPollEvery = 12

// tickMsg is one poll tick of chain gen. Waking from idle starts a new
// chain at the normal cadence; ticks of the slow one are dropped.
type tickMsg struct{ gen uint64 }

type refreshMsg struct {
	units    []quadlet.Unit
	images   []string
	statuses map[string]systemd.Status
	linger   bool
	lingerOK bool
	// lingerSkipped marks a refresh that did not read linger (see
	// lingerPollEvery): the model keeps what it knows.
	lingerSkipped bool
	podman        map[string]podman.Entry
	podmanTried   bool
	health        map[string]string
	ctrNames      map[string]string
	ctrOverride   map[string]string
	stale         []quadlet.Unit
	issues        []quadlet.Issue
	version       string
	err           error
	// epoch is the session the refresh ran against and issuedAt when it
	// started; applyRefresh drops results of an older session or older
	// than the last applied one. Zero values (hand-built messages) apply.
	epoch    uint64
	issuedAt time.Time
	// fromPoll marks the tick-driven refresh that holds the in-flight slot.
	fromPoll bool
}

// enrichLevel controls which podman-powered data a refresh gathers.
type enrichLevel int

const (
	enrichNone   enrichLevel = iota
	enrichHealth             // container health only (periodic poll)
	enrichFull               // quadlet list grouping + health (startup, daemon-reload)
)

// healthPollEvery is how many polls pass between health refreshes — podman
// ps is too heavy to run on every tick.
const healthPollEvery = 4

// refreshTimeout bounds one whole refresh, so a hung remote call can never
// hold the poll's in-flight slot forever.
const refreshTimeout = 90 * time.Second

func pollCmd(interval time.Duration, gen uint64) tea.Cmd {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return tickMsg{gen: gen} })
}

// pollEvery is the current poll cadence: the configured interval, slowed
// to idlePollInterval while nobody is looking.
func (m Model) pollEvery() time.Duration {
	d := m.pollInterval
	if d <= 0 {
		d = defaultPollInterval
	}
	if m.idle() && d < idlePollInterval {
		return idlePollInterval
	}
	return d
}

// idle reports an unattended screen: the terminal lost focus, or no key
// or click came for idleAfter.
func (m Model) idle() bool {
	return m.blurred || (!m.lastInput.IsZero() && time.Since(m.lastInput) > idleAfter)
}

// noteActivity tracks input and terminal focus. Coming back to an idle
// screen refreshes at once and restarts polling at the normal cadence.
func (m *Model) noteActivity(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case tea.BlurMsg:
		m.blurred = true
		return nil
	case tea.KeyPressMsg, tea.MouseClickMsg, tea.MouseWheelMsg, tea.PasteMsg, tea.FocusMsg:
	default:
		return nil
	}
	wasIdle := m.idle()
	m.blurred = false
	m.lastInput = time.Now()
	if !wasIdle {
		return nil
	}
	m.tickGen++
	cmds := []tea.Cmd{m.pollTick()}
	if !m.refreshInFlight && !m.loading {
		m.refreshInFlight = true
		req := m.refreshReq(enrichHealth)
		req.fromPoll = true
		cmds = append(cmds, req.cmd())
	}
	return tea.Batch(cmds...)
}

// refreshReq is everything one refresh needs, captured from the model when
// it is issued so the work runs against that session only.
type refreshReq struct {
	sys      *systemd.Systemd
	lc       *loginctl.Loginctl
	pm       podman.Client
	cache    *quadlet.InspectCache
	runner   remote.Runner
	level    enrichLevel
	linger   bool // read linger (see lingerPollEvery)
	system   bool
	dirs     []string
	epoch    uint64
	fromPoll bool
}

// refresh builds the list-refresh command for the model's own session and
// inspect cache, so call sites never repeat the wiring.
func (m Model) refresh(level enrichLevel) tea.Cmd {
	return m.refreshReq(level).cmd()
}

func (m Model) refreshReq(level enrichLevel) refreshReq {
	cache := m.inspect
	if cache == nil {
		cache = &quadlet.InspectCache{}
	}
	return refreshReq{
		sys: m.sys, lc: m.lc, pm: m.pm, cache: cache, runner: m.ssh,
		level: level, system: m.system, dirs: m.searchDirs(), epoch: m.epoch,
		linger: level == enrichFull || !m.lingerKnown,
	}
}

// pollTick schedules the next poll of the current chain at the current
// cadence (pollEvery).
func (m Model) pollTick() tea.Cmd {
	return pollCmd(m.pollEvery(), m.tickGen)
}

func (r refreshReq) cmd() tea.Cmd {
	if r.cache == nil {
		r.cache = &quadlet.InspectCache{}
	}
	return func() tea.Msg {
		msg := r.run()
		msg.epoch, msg.fromPoll = r.epoch, r.fromPoll
		return msg
	}
}

func (r refreshReq) run() refreshMsg {
	issuedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	var units []quadlet.Unit
	var images []string
	ctrOverride := map[string]string{}
	var files []*quadlet.File // parsed container files, for secret checks
	if r.runner.Isolated() {
		var err error
		units, err = quadlet.DiscoverRemoteMode(ctx, r.runner, r.system)
		if err != nil {
			return refreshMsg{err: err, issuedAt: issuedAt}
		}
		// Isolated files have no cheap local mtime: read and parse
		// each unit once per refresh through the session runner.
		images = make([]string, len(units))
		for i := range units {
			info, f := quadlet.InspectRemoteFile(ctx, r.runner, units[i])
			if units[i].Kind != quadlet.KindQuadlets {
				units[i].UnitName = info.UnitName
			}
			images[i] = info.Image
			if info.ContainerName != "" {
				ctrOverride[units[i].UnitName] = info.ContainerName
			}
			if f != nil && units[i].Kind == quadlet.KindContainer {
				files = append(files, f)
			}
		}
	} else {
		var err error
		units, err = quadlet.DiscoverDirs(r.dirs)
		if err != nil {
			return refreshMsg{err: err, issuedAt: issuedAt}
		}
		// Info is memoized by file mtime: only changed files are re-parsed.
		infos, imgs := r.cache.InspectAll(units)
		images = imgs
		for i, info := range infos {
			if info.ContainerName != "" {
				ctrOverride[units[i].UnitName] = info.ContainerName
			}
		}
	}
	statuses, err := r.sys.Show(ctx, unitNames(units))
	if err != nil {
		return refreshMsg{units: units, images: images, err: err, issuedAt: issuedAt}
	}
	var linger, lingerOK bool
	if !r.system && r.linger {
		var lerr error
		linger, lerr = r.lc.Enabled(ctx, "")
		lingerOK = lerr == nil
	}

	// Optional enrichment via podman (app/pod grouping, health, secrets).
	var pinfo map[string]podman.Entry
	var health, ctrNames map[string]string
	var issues []quadlet.Issue
	var version string
	reachable := r.level > enrichNone && r.pm.Reachable(ctx)
	if reachable {
		if h, herr := r.pm.PsHealth(ctx); herr == nil {
			health, ctrNames = h.ByName, h.NameByUnit
		}
	}
	if r.level == enrichFull {
		if reachable {
			if entries, perr := r.pm.QuadletList(ctx); perr == nil {
				pinfo = podman.ByUnit(entries)
			}
		}
		issues, _ = quadlet.ValidateSession(ctx, r.runner, r.dirs, r.system)
		if reachable {
			if secList, serr := r.pm.SecretList(ctx); serr == nil {
				if !r.runner.Isolated() {
					for _, u := range units {
						if u.Kind == quadlet.KindContainer && u.Path != "" {
							if pf, perr := quadlet.Parse(u.Path); perr == nil {
								files = append(files, pf)
							}
						}
					}
				}
				secNames := make([]string, len(secList))
				for i, sec := range secList {
					secNames[i] = sec.Name
				}
				issues = append(issues, quadlet.ValidateSecrets(files, secNames)...)
			}
			if ver, verr := r.pm.Version(ctx); verr == nil {
				version = ver
			}
		}
		if issues == nil {
			issues = []quadlet.Issue{} // validated: no problems (vs. not validated)
		}
	}

	return refreshMsg{
		units:         units,
		images:        images,
		statuses:      statuses,
		linger:        linger,
		lingerOK:      lingerOK,
		lingerSkipped: !r.system && !r.linger,
		podman:        pinfo,
		podmanTried:   r.level == enrichFull,
		health:        health,
		ctrNames:      ctrNames,
		ctrOverride:   ctrOverride,
		stale:         staleUnits(units, statuses),
		issues:        issues,
		version:       version,
		issuedAt:      issuedAt,
	}
}

// staleUnits lists the units a daemon-reload would change, from systemd's
// own NeedDaemonReload verdict (works in every session type, no file
// access needed). Extra-dir units are skipped: no reload can load them.
func staleUnits(units []quadlet.Unit, statuses map[string]systemd.Status) []quadlet.Unit {
	var stale []quadlet.Unit
	for _, u := range units {
		if u.UnitName == "" || isExternalUnit(u.Path) {
			continue
		}
		if st, ok := statuses[u.UnitName]; ok && st.NeedsReload() {
			stale = append(stale, u)
		}
	}
	return stale
}

// actionCmd runs one state-changing action and reports it as an
// actionMsg. The result carries the session epoch and the confirmation
// the action passed, so the actions screen can repeat it only in the same
// session and only behind the same confirmation.
func (m Model) actionCmd(desc string, run func(context.Context) (string, error)) tea.Cmd {
	return m.actionCmdHint(desc, "", run)
}

// actionCmdHint is actionCmd with a hint appended to the success status.
func (m Model) actionCmdHint(desc, hint string, run func(context.Context) (string, error)) tea.Cmd {
	epoch, guard := m.epoch, m.confirming
	return func() tea.Msg {
		start := time.Now()
		out, err := run(context.Background())
		return actionMsg{desc: desc, hint: hint, out: out, err: err, took: time.Since(start), run: run, epoch: epoch, guard: guard}
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

// applyRefresh merges a finished refresh into the model, keeping the cursor
// pinned on the same unit by name.
func (m Model) applyRefresh(msg refreshMsg) (tea.Model, tea.Cmd) {
	if msg.epoch != 0 && msg.epoch != m.epoch {
		return m, nil // a previous session's result: never paint it here
	}
	if msg.fromPoll {
		m.refreshInFlight = false
	}
	if !msg.issuedAt.IsZero() {
		if msg.issuedAt.Before(m.lastRefresh) {
			return m, nil // overtaken by a newer refresh already applied
		}
		m.lastRefresh = msg.issuedAt
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
	if !msg.lingerSkipped {
		m.linger = msg.linger
		m.lingerKnown = msg.lingerOK
	}
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
	if msg.ctrNames != nil {
		m.ctrNames = msg.ctrNames
	}
	if msg.ctrOverride != nil {
		m.ctrOverride = msg.ctrOverride
	}
	m.images = make(map[string]string, len(msg.units))
	for i, u := range msg.units {
		m.images[u.UnitName] = msg.images[i]
	}

	m.filtered = m.sortUnits(filterUnits(m.units, m.filterStr))
	m.buildRows()
	m.pinCursor(pin)
	m.loading = false
	switch m.mode {
	case modeTree:
		// Local files are cheap to re-parse; isolated sessions would pay a
		// remote cat per unit per tick, so they refresh on reopen only.
		if !m.ssh.Isolated() {
			m.treeView = m.computeTree()
		}
	case modeValidate:
		if msg.issues != nil {
			pin, _ := m.selectedRow()
			m.probList = m.computeProblems()
			m.pinScreenRow(pin.id)
		}
	}
	m.resize()
	return m, nil
}

// setBusy marks an action in flight; the spinner runs until its result msg lands.
func (m *Model) setBusy(text string) tea.Cmd {
	m.busy = true
	m.busyText = text
	m.busyHow = m.trigger
	return m.spinner.Tick
}
