package systemd

import (
	"context"
	"fmt"
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

// ListTimers returns all active or scheduled timers from systemctl list-timers.
func (s *Systemd) ListTimers(ctx context.Context) ([]Timer, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.bin(), s.args("list-timers", "--all", "--no-pager", "--full")...) // #nosec G204 -- argv slice, no shell, fixed verbs
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl list-timers: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTimers(string(out)), nil
}

// ParseTimers parses tabular output from `systemctl list-timers --all --no-pager --full`.
func ParseTimers(raw string) []Timer {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return nil
	}
	var header string
	var dataLines []string
	for _, l := range lines {
		l = strings.TrimRight(l, "\r\n")
		if strings.Contains(l, "UNIT") && strings.Contains(l, "ACTIVATES") {
			header = l
			continue
		}
		trimmed := strings.TrimSpace(l)
		if header != "" && trimmed != "" && !strings.HasSuffix(trimmed, "timers listed.") {
			dataLines = append(dataLines, l)
		}
	}
	if header == "" {
		return nil
	}
	unitIdx := strings.Index(header, "UNIT")
	activatesIdx := strings.Index(header, "ACTIVATES")
	nextIdx := strings.Index(header, "NEXT")
	leftIdx := strings.Index(header, "LEFT")
	lastIdx := strings.Index(header, "LAST")
	passedIdx := strings.Index(header, "PASSED")

	if unitIdx == -1 || activatesIdx == -1 || nextIdx == -1 || leftIdx == -1 || lastIdx == -1 || passedIdx == -1 {
		return nil
	}

	var timers []Timer
	for _, l := range dataLines {
		if len(l) <= unitIdx {
			continue
		}
		var next, left, last, passed, unit, activates string
		if len(l) > nextIdx && len(l) >= leftIdx {
			next = strings.TrimSpace(l[nextIdx:leftIdx])
		}
		if len(l) > leftIdx && len(l) >= lastIdx {
			left = strings.TrimSpace(l[leftIdx:lastIdx])
		}
		if len(l) > lastIdx && len(l) >= passedIdx {
			last = strings.TrimSpace(l[lastIdx:passedIdx])
		}
		if len(l) > passedIdx && len(l) >= unitIdx {
			passed = strings.TrimSpace(l[passedIdx:unitIdx])
		}
		if len(l) > unitIdx {
			tail := strings.TrimSpace(l[unitIdx:])
			tailFields := strings.Fields(tail)
			if len(tailFields) >= 2 {
				unit = tailFields[0]
				activates = tailFields[len(tailFields)-1]
			} else if len(tailFields) == 1 {
				unit = tailFields[0]
			}
		}
		if unit != "" {
			timers = append(timers, Timer{
				Unit:      unit,
				Activates: activates,
				Next:      next,
				Left:      left,
				Last:      last,
				Passed:    passed,
			})
		}
	}
	return timers
}
