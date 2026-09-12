package systemd

import (
	"context"
	"fmt"
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
