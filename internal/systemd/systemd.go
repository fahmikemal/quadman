// Package systemd wraps the systemctl CLI for the systemd user session,
// where rootless Quadlet units live.
package systemd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// DefaultTimeout bounds each systemctl/journalctl call so a hung binary
// cannot stall a refresh forever.
const DefaultTimeout = 30 * time.Second

// JobTimeout bounds start/stop/restart. systemctl waits for the job to
// finish, and a first start that pulls a large image legitimately takes
// minutes (Quadlet users raise TimeoutStartSec= for exactly that), so the
// read timeout above would report healthy starts as failures.
const JobTimeout = 5 * time.Minute

// ErrJobPending reports that quadman stopped waiting for a systemd job
// that systemd itself keeps running. It is not a failure of the unit.
var ErrJobPending = errors.New("still running in systemd (quadman stopped waiting; the job continues in the background)")

// Systemd talks to systemd through the systemctl CLI.
type Systemd struct {
	// User targets the user session (the rootless Quadlet case). Default true.
	User bool
	// Bin overrides the systemctl binary path.
	Bin string
	// JournalBin overrides the journalctl binary path.
	JournalBin string
	// Timeout bounds each call; 0 means DefaultTimeout.
	Timeout time.Duration
	// Remote runs the CLI over SSH (or sudo compartment) when set. The
	// zero value runs everything locally.
	Remote remote.Runner
	// GenDir overrides the generator output directory (stale detection).
	// Empty means GeneratorDir() derives it from the session; compartments
	// set it to the target user's runtime generator dir.
	GenDir string
	// JobTimeout bounds start/stop/restart jobs; 0 means JobTimeout.
	JobTimeout time.Duration
}

// New returns a Systemd targeting the current user's session.
func New() *Systemd {
	return &Systemd{User: true, Bin: "systemctl", JournalBin: "journalctl"}
}

// NewSystem returns a Systemd targeting the system-wide (rootful) instance.
func NewSystem() *Systemd {
	return &Systemd{User: false, Bin: "systemctl", JournalBin: "journalctl"}
}

func (s *Systemd) bin() string {
	if s.Bin != "" {
		return s.Bin
	}
	return "systemctl"
}

func (s *Systemd) journalBin() string {
	if s.JournalBin != "" {
		return s.JournalBin
	}
	return "journalctl"
}

func (s *Systemd) timeoutCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	t := s.Timeout
	if t <= 0 {
		t = DefaultTimeout
	}
	return context.WithTimeout(ctx, t)
}

func (s *Systemd) args(extra ...string) []string {
	if s.User {
		return append([]string{"--user"}, extra...)
	}
	return extra
}

// run executes the systemctl CLI (locally or over SSH) like
// exec.CommandContext.
func (s *Systemd) run(ctx context.Context, name string, args ...string) *exec.Cmd {
	return s.Remote.Command(ctx, name, args...)
}

// Status is a snapshot of one unit's runtime state.
type Status struct {
	Id          string
	LoadState   string // loaded / not-found / masked / error
	ActiveState string // active / inactive / failed
	SubState    string // running / exited / dead / …
	Description string
	// ActiveSince is when the unit last entered the active state (zero
	// when it never did); the UI derives UPTIME from it.
	ActiveSince time.Time
	// Result is systemd's last run result: success, exit-code, signal,
	// timeout, start-limit-hit, …
	Result string
	// ExecMainStatus is the main process exit code (or signal number).
	ExecMainStatus int
	// NRestarts counts automatic restarts (Restart=) since the last
	// manual start; a climbing value means a crash loop.
	NRestarts int
	// NeedDaemonReload is systemd's own verdict that the unit's files —
	// including the Quadlet source it records as SourcePath= — changed
	// since the last daemon-reload. It needs no filesystem access, so it
	// is authoritative over SSH and in compartments alike.
	NeedDaemonReload bool
}

// NeedsReload reports whether a daemon-reload would change this unit:
// systemd says its sources changed, or it was never generated yet.
func (st Status) NeedsReload() bool {
	return st.NeedDaemonReload || st.LoadState == "not-found"
}

// Loaded reports whether systemd knows the unit (it was generated).
func (st Status) Loaded() bool { return st.LoadState != "" && st.LoadState != "not-found" }

// Display returns the state/sub-state pair to show. Unknown units surface
// their load state so "not-found" hints at a missing daemon-reload.
func (st Status) Display() (state, sub string) {
	if st.Id == "" {
		return "-", "-"
	}
	if !st.Loaded() && st.LoadState != "" {
		return st.LoadState, "-"
	}
	if st.ActiveState == "" {
		return "-", "-"
	}
	return st.ActiveState, st.SubState
}

// Show fetches state for many units in a single systemctl call. Units that
// systemd does not know come back with LoadState=not-found rather than an
// error.
func (s *Systemd) Show(ctx context.Context, units []string) (map[string]Status, error) {
	out := map[string]Status{}
	if len(units) == 0 {
		return out, nil
	}
	args := s.args(append([]string{
		"show",
		"--property=Id,LoadState,ActiveState,SubState,Description,ActiveEnterTimestamp,Result,ExecMainStatus,NRestarts,NeedDaemonReload",
		"--timestamp=unix", // "@1696000000": no locale or timezone parsing
		"--no-pager",
		"--", // never let a unit name be read as an option
	}, units...)...)
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.bin(), args...) // #nosec G204 -- argv slice, no shell; unit names are behind a "--" separator; driving systemctl is this tool's purpose
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl show: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var cur *Status
	flush := func() {
		if cur != nil && cur.Id != "" {
			out[cur.Id] = *cur
		}
		cur = nil
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "Id":
			flush()
			cur = &Status{Id: v}
		case "LoadState":
			if cur != nil {
				cur.LoadState = v
			}
		case "ActiveState":
			if cur != nil {
				cur.ActiveState = v
			}
		case "SubState":
			if cur != nil {
				cur.SubState = v
			}
		case "Description":
			if cur != nil {
				cur.Description = v
			}
		case "ActiveEnterTimestamp":
			if cur != nil {
				cur.ActiveSince = parseUnixTimestamp(v)
			}
		case "Result":
			if cur != nil {
				cur.Result = v
			}
		case "ExecMainStatus":
			if cur != nil {
				cur.ExecMainStatus, _ = strconv.Atoi(v)
			}
		case "NRestarts":
			if cur != nil {
				cur.NRestarts, _ = strconv.Atoi(v)
			}
		case "NeedDaemonReload":
			if cur != nil {
				cur.NeedDaemonReload = v == "yes"
			}
		}
	}
	flush()
	return out, nil
}

// UnitAction runs a lifecycle verb (start, stop, restart) on a unit and
// returns any output from systemctl. It waits for the job up to the job
// timeout; past it the error wraps ErrJobPending, because systemd keeps
// running the job and the unit's state will still change.
func (s *Systemd) UnitAction(ctx context.Context, verb, unit string) (string, error) {
	t := s.JobTimeout
	if t <= 0 {
		t = JobTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	out, err := s.unitCmdCtx(ctx, verb, unit, verb)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("systemctl %s %s: %w (waited %s)", verb, unit, ErrJobPending, t)
	}
	return out, err
}

// Enable makes a unit start at boot; with now it is also started immediately.
func (s *Systemd) Enable(ctx context.Context, unit string, now bool) (string, error) {
	verb := "enable"
	args := []string{verb}
	if now {
		args = append(args, "--now")
	}
	return s.unitCmd(ctx, verb, unit, args...)
}

// Disable stops a unit from starting at boot; with now it is also stopped.
func (s *Systemd) Disable(ctx context.Context, unit string, now bool) (string, error) {
	verb := "disable"
	args := []string{verb}
	if now {
		args = append(args, "--now")
	}
	return s.unitCmd(ctx, verb, unit, args...)
}

// unitCmd runs systemctl with the given sub-arguments under the default
// timeout; verb and unit are used for error messages only.
func (s *Systemd) unitCmd(ctx context.Context, verb, unit string, args ...string) (string, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	return s.unitCmdCtx(ctx, verb, unit, args...)
}

// unitCmdCtx is unitCmd under the caller's deadline.
func (s *Systemd) unitCmdCtx(ctx context.Context, verb, unit string, args ...string) (string, error) {
	cmd := s.run(ctx, s.bin(), s.args(append(args, "--", unit)...)...) // #nosec G204 -- argv slice, no shell; unit name is behind a "--" separator
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return msg, fmt.Errorf("systemctl %s %s: %w: %s", verb, unit, err, strings.TrimSpace(stderr.String()))
	}
	return msg, nil
}

// UserGeneratorDir returns the directory where the user systemd instance
// writes generator output, which is where Quadlet units land after a
// daemon-reload ($XDG_RUNTIME_DIR/systemd/generator). It returns "" when the
// runtime dir is unknown.
func UserGeneratorDir() string {
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		return ""
	}
	return filepath.Join(rt, "systemd", "generator")
}

// SystemGeneratorDir returns the directory where the system-wide systemd instance
// writes generator output (/run/systemd/generator).
func SystemGeneratorDir() string {
	return "/run/systemd/generator"
}

// GeneratorDir returns the generator directory for this Systemd instance:
// the GenDir override when set (compartments), SystemGeneratorDir() for
// system instances, or UserGeneratorDir() for user sessions.
func (s *Systemd) GeneratorDir() string {
	if s.GenDir != "" {
		return s.GenDir
	}
	if !s.User {
		return SystemGeneratorDir()
	}
	return UserGeneratorDir()
}

// DaemonReload makes systemd regenerate Quadlet units from their source
// files (systemctl --user daemon-reload).
func (s *Systemd) DaemonReload(ctx context.Context) (string, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.bin(), s.args("daemon-reload")...) // #nosec G204 -- argv slice, no shell, constant verb
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return msg, fmt.Errorf("systemctl daemon-reload: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return msg, nil
}

// parseUnixTimestamp reads systemctl's --timestamp=unix form ("@<secs>").
// Empty, "n/a" or malformed values yield the zero time.
func parseUnixTimestamp(v string) time.Time {
	secs, err := strconv.ParseInt(strings.TrimPrefix(v, "@"), 10, 64)
	if err != nil || secs <= 0 || !strings.HasPrefix(v, "@") {
		return time.Time{}
	}
	return time.Unix(secs, 0)
}
