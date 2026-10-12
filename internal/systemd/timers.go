package systemd

import (
	"context"
	"fmt"
	"strings"
	"time"
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

// timerStampLayout renders timer times like systemctl does in the C locale.
const timerStampLayout = "Mon 2006-01-02 15:04:05 MST"

// ListTimers returns every loaded timer with its schedule. It only reads
// systemctl's machine-oriented output: unit names from the first column
// of `list-units --plain --no-legend`, times from `show --timestamp=unix`.
// No localized, column-aligned table is parsed, so locale, column widths,
// and timezone names cannot break it, locally or through any runner.
func (s *Systemd) ListTimers(ctx context.Context) ([]Timer, error) {
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.bin(), s.args("list-units", "--type=timer", "--all", "--plain", "--no-legend", "--no-pager")...) // #nosec G204 -- argv slice, fixed verbs
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl list-units --type=timer: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	names := timerUnitNames(string(out))
	if len(names) == 0 {
		return nil, nil
	}
	props, err := s.ShowProps(ctx, names, []string{"Unit", "NextElapseUSecRealtime", "LastTriggerUSec"})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	timers := make([]Timer, 0, len(names))
	for _, name := range names {
		p := props[name]
		get := func(k string) string {
			if v := p[k]; len(v) > 0 {
				return v[len(v)-1]
			}
			return ""
		}
		next := parseUnixTimestamp(get("NextElapseUSecRealtime"))
		last := parseUnixTimestamp(get("LastTriggerUSec"))
		t := Timer{Unit: name, Activates: orDash(get("Unit")), Next: "-", Left: "-", Last: "-", Passed: "-"}
		if !next.IsZero() {
			t.Next, t.Left = next.Format(timerStampLayout), Span(next.Sub(now))
		}
		if !last.IsZero() {
			t.Last, t.Passed = last.Format(timerStampLayout), Span(now.Sub(last))+" ago"
		}
		timers = append(timers, t)
	}
	return timers, nil
}

// timerUnitNames reads the unit names (first column) of `list-units
// --plain --no-legend`, which never localizes or aligns that column.
func timerUnitNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && strings.HasSuffix(f[0], ".timer") {
			names = append(names, f[0])
		}
	}
	return names
}

// Span renders a duration the way systemctl's LEFT/PASSED columns do, to
// two units: "45s", "29min", "1h 16min", "6 days", "4 weeks 2 days".
func Span(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	units := []struct {
		name string
		size time.Duration
	}{
		{"weeks", 7 * 24 * time.Hour}, {"days", 24 * time.Hour}, {"h", time.Hour}, {"min", time.Minute}, {"s", time.Second},
	}
	var parts []string
	for _, u := range units {
		if n := d / u.size; n > 0 {
			name := u.name
			if n == 1 && (name == "weeks" || name == "days") {
				name = strings.TrimSuffix(name, "s")
			}
			sep := ""
			if name != "h" && name != "min" && name != "s" {
				sep = " "
			}
			parts = append(parts, fmt.Sprintf("%d%s%s", n, sep, name))
			d -= n * u.size
			if len(parts) == 2 {
				break
			}
		} else if len(parts) > 0 {
			break // "1h 0min" reads worse than "1h"
		}
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return strings.TrimSpace(s)
}
