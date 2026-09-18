// Package systemd wraps the systemctl CLI for the systemd user session,
// where rootless Quadlet units live.
package systemd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// DefaultTimeout bounds each systemctl/journalctl call so a hung binary
// cannot stall a refresh forever.
const DefaultTimeout = 30 * time.Second

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
	// Remote runs the CLI over SSH when set (--ssh user@host). The zero
	// value runs everything locally.
	Remote remote.Runner
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
		"--property=Id,LoadState,ActiveState,SubState,Description",
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
		}
	}
	flush()
	return out, nil
}

// UnitAction runs a lifecycle verb (start, stop, restart) on a unit and
// returns any output from systemctl.
func (s *Systemd) UnitAction(ctx context.Context, verb, unit string) (string, error) {
	return s.unitCmd(ctx, verb, unit, verb)
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

// unitCmd runs systemctl with the given sub-arguments; verb and unit are used
// for error messages only.
func (s *Systemd) unitCmd(ctx context.Context, verb, unit string, args ...string) (string, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
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

// GeneratorDir returns the generator directory for this Systemd instance.
// For system instances (!User), it returns SystemGeneratorDir() (/run/systemd/generator).
// For user instances (User), it returns UserGeneratorDir().
func (s *Systemd) GeneratorDir() string {
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
