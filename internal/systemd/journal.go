package systemd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Journal returns the last lines of a unit's journal as a snapshot. It
// honors the same User scope as the systemctl calls.
func (s *Systemd) Journal(ctx context.Context, unit string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	args := []string{
		"--no-pager",
		"--output=short-iso",
		"--unit=" + unit, // value form: a unit name can never be read as an option
		"--lines=" + strconv.Itoa(lines),
	}
	if s.User {
		args = append([]string{"--user"}, args...)
	}
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.journalBin(), args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	msg := strings.TrimSpace(string(out))
	if err != nil {
		return "", fmt.Errorf("journalctl: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return msg, nil
}

// FollowJournal starts `journalctl -f` for a unit and returns the running
// process together with its stdout stream. The caller must call stop to kill
// the process and reap it; ctx cancelation stops it too.
func (s *Systemd) FollowJournal(ctx context.Context, unit string, lines int) (stop func(), stream io.Reader, err error) {
	if lines <= 0 {
		lines = 200
	}
	args := []string{
		"--no-pager",
		"--output=short-iso",
		"--unit=" + unit,
		"--lines=" + strconv.Itoa(lines),
		"--follow",
	}
	if s.User {
		args = append([]string{"--user"}, args...)
	}
	cmd := exec.CommandContext(ctx, s.journalBin(), args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("journalctl -f: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("journalctl -f: %w", err)
	}
	stop = func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return stop, stdout, nil
}

// IsEnabled reports the boot-time enablement of a unit via
// `systemctl is-enabled` (enabled / disabled / static / generated / …).
func (s *Systemd) IsEnabled(ctx context.Context, unit string) (string, error) {
	return s.stateQuery(ctx, "is-enabled", unit)
}

// IsActive reports the runtime state of a unit via `systemctl is-active`
// (active / inactive / failed / …).
func (s *Systemd) IsActive(ctx context.Context, unit string) (string, error) {
	return s.stateQuery(ctx, "is-active", unit)
}

// stateQuery runs `systemctl <verb> -- <unit>` and returns its one-word
// answer. Both verbs exit non-zero for "not the requested state", which is
// not an error here — the answer itself is the payload.
func (s *Systemd) stateQuery(ctx context.Context, verb, unit string) (string, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	out, err := exec.CommandContext(ctx, s.bin(), s.args(verb, "--", unit)...).Output()
	if err != nil {
		// is-enabled/is-active exit non-zero for "not in that state"; the
		// one-word answer printed to stdout is still the payload we want.
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(out) > 0 {
			return strings.TrimSpace(string(out)), nil
		}
		return "", fmt.Errorf("systemctl %s %s: %w", verb, unit, err)
	}
	return strings.TrimSpace(string(out)), nil
}
