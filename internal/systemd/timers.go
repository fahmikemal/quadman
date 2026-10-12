package systemd

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Timer represents one systemd timer entity and its schedule.
type Timer struct {
	Unit      string `json:"unit"`
	Activates string `json:"activates"`
	Next      string `json:"next"`
	Left      string `json:"left"`
	Last      string `json:"last"`
	Passed    string `json:"passed"`
}

// listTimersCmd builds the list-timers invocation with a fixed C locale.
// Column headers are localized by systemd, so parsing them requires a
// known locale. The runner carries LC_ALL=C in every mode (local
// environment, remote shell assignment, sudo VAR=value), so sudoers never
// needs to allow /usr/bin/env.
func (s *Systemd) listTimersCmd(ctx context.Context) *exec.Cmd {
	args := s.args("list-timers", "--all", "--no-pager", "--full")
	return s.Remote.CommandEnv(ctx, []string{"LC_ALL=C"}, s.bin(), args...) // #nosec G204 -- argv slice, fixed verbs
}

// ListTimers returns all active or scheduled timers from systemctl list-timers.
func (s *Systemd) ListTimers(ctx context.Context) ([]Timer, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.listTimersCmd(ctx)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl list-timers: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTimers(string(out)), nil
}

// timerLine matches one data row of `list-timers --all --full` in the C
// locale: NEXT and LAST are a timestamp ("Mon 2026-10-12 22:13:58 WIB")
// or "-"/"n/a"; LEFT and PASSED are free-form spans ("20h", "4 weeks 2
// days", "1h 16min ago"); UNIT and ACTIVATES are the last two fields.
// Values are right-aligned under their headers, so a row cannot be cut at
// header offsets ("6 days" starts left of the LEFT header).
var timerLine = regexp.MustCompile(`^\s*(` + timerStamp + `|-|n/a)\s+(.*?)\s+(` + timerStamp + `|-|n/a)\s+(.*?)\s+(\S+)\s+(\S+)\s*$`)

// timerStamp is "Day YYYY-MM-DD HH:MM:SS TZ".
const timerStamp = `\S+ \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \S+`

// ParseTimers parses tabular output from `systemctl list-timers --all --no-pager --full`.
func ParseTimers(raw string) []Timer {
	var timers []Timer
	seenHeader := false
	for _, l := range strings.Split(raw, "\n") {
		l = strings.TrimRight(l, "\r")
		if strings.Contains(l, "UNIT") && strings.Contains(l, "ACTIVATES") {
			seenHeader = true
			continue
		}
		t := strings.TrimSpace(l)
		if !seenHeader || t == "" || strings.HasSuffix(t, "timers listed.") || strings.HasSuffix(t, "timer listed.") {
			continue
		}
		mt := timerLine.FindStringSubmatch(l)
		if mt == nil {
			continue
		}
		timers = append(timers, Timer{
			Next:      mt[1],
			Left:      orDash(mt[2]),
			Last:      mt[3],
			Passed:    orDash(mt[4]),
			Unit:      mt[5],
			Activates: mt[6],
		})
	}
	return timers
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return strings.TrimSpace(s)
}
