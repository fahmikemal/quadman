package ui

import (
	"time"

	"github.com/fahmikemal/quadman/internal/loginctl"
	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// A session is the target every CLI call addresses: the runner plus the
// systemctl, loginctl, podman, and podlet clients bound to it. Switching
// targets (SSH at startup, a compartment, back to the own session) never
// mutates the current clients: setSession builds new ones and bumps the
// session epoch. Commands already in flight keep the clients they
// captured, so they finish against the session they were started for, and
// results stamped with an older epoch are dropped instead of painting the
// previous target's units under the new header.

// setSession re-targets every CLI client at runner. system selects the
// system instance; genDir overrides the generator output dir ("" derives
// it from the session).
func (m *Model) setSession(runner remote.Runner, system bool, genDir string) {
	sys := systemd.New()
	if system {
		sys = systemd.NewSystem()
	}
	if m.sys != nil {
		sys.Bin, sys.JournalBin, sys.Timeout, sys.JobTimeout = m.sys.Bin, m.sys.JournalBin, m.sys.Timeout, m.sys.JobTimeout
	}
	sys.Remote = runner
	sys.GenDir = genDir

	lc := loginctl.New()
	if m.lc != nil {
		lc.Bin, lc.Timeout = m.lc.Bin, m.lc.Timeout
	}
	lc.Remote = runner

	m.ssh = runner
	m.sys = sys
	m.lc = lc
	m.pm = podman.New(runner)
	m.pl = podlet.New(runner)
	m.epoch++
	// The new target has not been enriched yet; whatever the previous
	// one reported about podman, health, and containers is not its truth.
	m.podmanTried = false
	m.podmanInfo = nil
	m.linger, m.lingerKnown = false, false // the new target's own, read next
	m.health = map[string]string{}
	m.ctrNames = map[string]string{}
	m.refreshInFlight = false
	m.lastRefresh = time.Time{}
	// So does every screen's data, and any view streaming from it: the
	// new target starts on its own unit list with nothing cached.
	m.storage, m.timers, m.secrets = storageData{}, timersData{}, secretsData{}
	m.stats = statsData{showIdle: m.stats.showIdle}
	m.updateEntries, m.timerEnabled, m.timerActive = nil, "", ""
	m.cancelLoad()
	m.scrLoaded, m.scrLoading, m.scrErr, m.scrErrFor = 0, modeList, "", modeList
	m.scrLoadedAt, m.statsInFlight = [modeCount]time.Time{}, false
	m.busy = false
	if m.mode != modeList {
		m.stopLogs()
		m.stopEvents()
		m.mode = modeList
	}
}

// applySSH points every CLI client at the remote target.
func (m *Model) applySSH(target string) {
	m.setSession(remote.Runner{Target: target, Multiplex: m.cfg.SSHMultiplex()}, m.system, "")
}
