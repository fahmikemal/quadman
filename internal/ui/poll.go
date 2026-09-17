package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/loginctl"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/remote"
	"github.com/kemal-labs/quadman/internal/systemd"
)

// defaultPollInterval is how often the unit list refreshes itself when the
// user has not configured another interval. The cursor stays pinned on the
// same unit (by name) across refreshes.
const defaultPollInterval = 2500 * time.Millisecond

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

func pollCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return tickMsg{} })
}

// refresh builds the list-refresh command for the model's own clients and
// inspect cache, so call sites never repeat the wiring.
func (m Model) refresh(level enrichLevel) tea.Cmd {
	cache := m.inspect
	if cache == nil {
		cache = &quadlet.InspectCache{}
	}
	return refreshCmd(m.sys, m.lc, cache, m.ssh, level, m.system)
}

// pollTick schedules the next poll using the model's configured interval
// (or the default when unset).
func (m Model) pollTick() tea.Cmd {
	return pollCmd(m.pollInterval)
}

func refreshCmd(sys *systemd.Systemd, lc *loginctl.Loginctl, cache *quadlet.InspectCache, runner remote.Runner, level enrichLevel, system bool) tea.Cmd {
	if cache == nil {
		cache = &quadlet.InspectCache{}
	}
	return func() tea.Msg {
		ctx := context.Background()
		var units []quadlet.Unit
		var images []string
		if runner.IsRemote() {
			var err error
			units, err = quadlet.DiscoverRemoteMode(ctx, runner, system)
			if err != nil {
				return refreshMsg{err: err}
			}
			// Remote files have no cheap mtime: read and parse each unit
			// once per refresh through the SSH runner.
			images = make([]string, len(units))
			for i := range units {
				info := inspectRemote(ctx, runner, units[i])
				units[i].UnitName = info.UnitName
				images[i] = info.Image
			}
		} else {
			var err error
			units, err = quadlet.DiscoverMode(system)
			if err != nil {
				return refreshMsg{err: err}
			}
			// Info is memoized by file mtime: only changed files are re-parsed.
			_, images = cache.InspectAll(units)
		}
		statuses, err := sys.Show(ctx, unitNames(units))
		if err != nil {
			return refreshMsg{units: units, images: images, err: err}
		}
		var linger bool
		var lingerOK bool
		if !system {
			var lerr error
			linger, lerr = lc.Enabled(ctx, "")
			lingerOK = lerr == nil
		}

		// Optional enrichment via podman (app/pod grouping + container health).
		var pinfo map[string]podman.Entry
		var health map[string]string
		var issues []quadlet.Issue
		var version string
		if level >= enrichHealth && podmanAvailable(ctx, runner) {
			if hm, herr := podman.PsHealth(ctx); herr == nil {
				health = hm
			}
		}
		if level == enrichFull {
			if podmanAvailable(ctx, runner) {
				if entries, perr := podman.QuadletList(ctx); perr == nil {
					pinfo = podman.ByUnit(entries)
				}
			}
			if !runner.IsRemote() {
				issues, _ = quadlet.ValidateMode(ctx, quadlet.SearchDirsMode(system), system)
			}
			if ver, verr := podman.Version(ctx); verr == nil {
				version = ver
			}
		}

		var stale []quadlet.Unit
		if !runner.IsRemote() {
			stale = quadlet.StaleUnits(units, sys.GeneratorDir())
		}
		return refreshMsg{
			units:       units,
			images:      images,
			statuses:    statuses,
			linger:      linger,
			lingerOK:    lingerOK,
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

// podmanAvailable reports whether podman answers, locally via PATH or
// remotely through the runner.
func podmanAvailable(ctx context.Context, runner remote.Runner) bool {
	if !runner.IsRemote() {
		return podman.Available()
	}
	ctx, cancel := context.WithTimeout(ctx, remote.DialTimeout)
	defer cancel()
	_, err := runner.Output(ctx, "podman", "--version")
	return err == nil
}

// inspectRemote resolves one remote unit's Info by catting its source file
// through the SSH runner. Failures keep the default generated name,
// matching the uncached local fallback.
func inspectRemote(ctx context.Context, runner remote.Runner, u quadlet.Unit) quadlet.Info {
	info := quadlet.Info{UnitName: quadlet.UnitFileName(u.Name, u.Kind)}
	if u.Path == "" {
		return info
	}
	data, err := runner.Cat(ctx, u.Path)
	if err != nil {
		return info
	}
	f, err := quadlet.ParseBytes(u.Path, data)
	if err != nil {
		return info
	}
	sec := f.Section(string(u.Kind))
	if sn := sec.Get("ServiceName"); sn != "" {
		info.UnitName = sn + ".service"
	}
	if u.Kind == quadlet.KindBuild {
		info.Image = sec.Get("ImageTag")
	} else {
		info.Image = sec.Get("Image")
	}
	return info
}

// applyRefresh merges a finished refresh into the model, keeping the cursor
// pinned on the same unit by name.
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

// setBusy marks an action in flight; the spinner runs until its result msg lands.
func (m *Model) setBusy(text string) tea.Cmd {
	m.busy = true
	m.busyText = text
	return m.spinner.Tick
}
