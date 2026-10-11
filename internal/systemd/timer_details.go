package systemd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ShowProps runs `systemctl show` for units and returns every requested
// property per unit Id. Values are kept as lists because some properties
// (TimersCalendar, TimersMonotonic) repeat once per trigger. Timestamps
// come back in the "@<unix secs>" form.
func (s *Systemd) ShowProps(ctx context.Context, units, props []string) (map[string]map[string][]string, error) {
	out := map[string]map[string][]string{}
	if len(units) == 0 {
		return out, nil
	}
	args := s.args(append([]string{
		"show",
		"--property=Id," + strings.Join(props, ","),
		"--timestamp=unix",
		"--no-pager",
		"--",
	}, units...)...)
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.bin(), args...) // #nosec G204 -- argv slice, no shell; unit names are behind a "--" separator
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("systemctl show: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var cur map[string][]string
	for _, line := range strings.Split(stdout.String(), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if k == "Id" {
			cur = map[string][]string{}
			out[v] = cur
			continue
		}
		if cur != nil {
			cur[k] = append(cur[k], v)
		}
	}
	return out, nil
}

// TimerInfo is the schedule and outcome of one timer and the unit it
// triggers.
type TimerInfo struct {
	Unit        string    // the timer, e.g. backup.timer
	Triggers    string    // the unit it activates
	ActiveState string    // active / inactive / failed
	SubState    string    // waiting / running / elapsed / dead
	Schedules   []string  // OnCalendar= expressions and monotonic triggers, as declared
	Persistent  bool      // catches up runs missed while powered off
	RandomDelay string    // RandomizedDelaySec, "0" when unset
	Accuracy    string    // AccuracySec
	Next        time.Time // zero for monotonic-only or inactive timers
	Last        time.Time // zero when it never fired
	FileState   string    // enabled / disabled / static / …
	Path        string    // the timer's unit file
	// LastResult is the triggered unit's Result (success, exit-code, …)
	// and LastStatus its main exit code.
	LastResult string
	LastStatus int
}

// Failed reports a timer whose own state or last triggered run failed.
func (t TimerInfo) Failed() bool {
	return t.ActiveState == "failed" || (t.LastResult != "" && t.LastResult != "success")
}

// TimerDetails reads TimerInfo for every named timer (two systemctl show
// calls in total: the timers, then the units they trigger).
func (s *Systemd) TimerDetails(ctx context.Context, timers []string) ([]TimerInfo, error) {
	props, err := s.ShowProps(ctx, timers, []string{
		"Unit", "ActiveState", "SubState", "TimersCalendar", "TimersMonotonic", "Persistent",
		"RandomizedDelayUSec", "AccuracyUSec", "NextElapseUSecRealtime", "LastTriggerUSec",
		"UnitFileState", "FragmentPath",
	})
	if err != nil {
		return nil, err
	}
	var out []TimerInfo
	var services []string
	for _, name := range timers {
		p := props[name]
		if p == nil {
			continue
		}
		get := func(k string) string {
			if v := p[k]; len(v) > 0 {
				return v[len(v)-1]
			}
			return ""
		}
		t := TimerInfo{
			Unit:        name,
			Triggers:    get("Unit"),
			ActiveState: get("ActiveState"),
			SubState:    get("SubState"),
			Persistent:  get("Persistent") == "yes",
			RandomDelay: get("RandomizedDelayUSec"),
			Accuracy:    get("AccuracyUSec"),
			Next:        parseUnixTimestamp(get("NextElapseUSecRealtime")),
			Last:        parseUnixTimestamp(get("LastTriggerUSec")),
			FileState:   get("UnitFileState"),
			Path:        get("FragmentPath"),
		}
		for _, c := range p["TimersCalendar"] {
			if expr := triggerExpr(c); expr != "" {
				t.Schedules = append(t.Schedules, expr)
			}
		}
		for _, c := range p["TimersMonotonic"] {
			if expr := triggerExpr(c); expr != "" {
				t.Schedules = append(t.Schedules, expr)
			}
		}
		if t.Triggers != "" {
			services = append(services, t.Triggers)
		}
		out = append(out, t)
	}
	if len(services) > 0 {
		sp, err := s.ShowProps(ctx, services, []string{"Result", "ExecMainStatus"})
		if err == nil {
			for i := range out {
				if p := sp[out[i].Triggers]; p != nil {
					if v := p["Result"]; len(v) > 0 {
						out[i].LastResult = v[0]
					}
					if v := p["ExecMainStatus"]; len(v) > 0 {
						out[i].LastStatus, _ = strconv.Atoi(v[0])
					}
				}
			}
		}
	}
	return out, nil
}

// triggerExpr turns "{ OnCalendar=*-*-* 03:00:00 ; next_elapse=@… }" or
// "{ OnUnitActiveUSec=1d ; next_elapse=… }" into "OnCalendar=*-*-* 03:00:00".
func triggerExpr(raw string) string {
	raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "{"), "}"))
	first, _, _ := strings.Cut(raw, ";")
	return strings.TrimSpace(first)
}

// RunRecord is one completed start job of a unit, read from the journal.
type RunRecord struct {
	At time.Time
	OK bool
}

// RunHistory returns up to n most recent start-job results of unit, oldest
// first, from systemd's own job messages (JOB_TYPE=start, JOB_RESULT).
func (s *Systemd) RunHistory(ctx context.Context, unit string, n int) ([]RunRecord, error) {
	args := []string{
		"--no-pager", "--output=json",
		"--output-fields=JOB_TYPE,JOB_RESULT",
		"--unit=" + unit,
		"--lines=2000",
	}
	if s.User {
		args = append([]string{"--user"}, args...)
	}
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.journalBin(), args...) // #nosec G204 -- argv slice, no shell; unit name is a "--unit=" value
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w", err)
	}
	return parseRunHistory(string(out), n), nil
}

func parseRunHistory(raw string, n int) []RunRecord {
	var runs []RunRecord
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e struct {
			JobType   string `json:"JOB_TYPE"`
			JobResult string `json:"JOB_RESULT"`
			RT        string `json:"__REALTIME_TIMESTAMP"`
		}
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.JobType != "start" || e.JobResult == "" {
			continue
		}
		usec, _ := strconv.ParseInt(e.RT, 10, 64)
		runs = append(runs, RunRecord{At: time.UnixMicro(usec), OK: e.JobResult == "done"})
	}
	if n > 0 && len(runs) > n {
		runs = runs[len(runs)-n:]
	}
	return runs
}
