package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ManagerEntry is one message the service manager itself logged about a
// unit (Starting/Started/Failed/Scheduled restart …), as opposed to the
// unit's own output.
type ManagerEntry struct {
	At        time.Time
	Unit      string
	Message   string
	JobType   string // start / stop / restart / …
	JobResult string // done / failed / timeout / … ("" while in progress)
	Priority  int    // syslog priority, 0 = emerg … 7 = debug
}

// managerFields keeps the journal JSON small.
const managerFields = "MESSAGE,USER_UNIT,UNIT,JOB_TYPE,JOB_RESULT,SYSLOG_IDENTIFIER,PRIORITY"

func (s *Systemd) managerArgs(units []string, extra ...string) []string {
	args := []string{"--no-pager", "--output=json", "--output-fields=" + managerFields}
	args = append(args, extra...)
	for _, u := range units {
		args = append(args, "--unit="+u) // value form: never read as an option
	}
	if s.User {
		args = append([]string{"--user"}, args...)
	}
	return args
}

// ManagerJournal returns the manager's messages about units since a
// journalctl --since value (e.g. "-1h"), oldest first.
func (s *Systemd) ManagerJournal(ctx context.Context, units []string, since string) ([]ManagerEntry, error) {
	if len(units) == 0 {
		return nil, nil
	}
	ctx, cancel := s.timeoutCtx(ctx)
	defer cancel()
	cmd := s.run(ctx, s.journalBin(), s.managerArgs(units, "--since="+since)...) // #nosec G204 -- argv slice, no shell; units are "--unit=" values
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var entries []ManagerEntry
	for _, line := range strings.Split(string(out), "\n") {
		if e, ok := ParseManagerEntry(line); ok {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// FollowManagerJournal streams new manager messages about units (no
// history: the caller loads that with ManagerJournal).
func (s *Systemd) FollowManagerJournal(ctx context.Context, units []string) (stop func(), stream io.Reader, err error) {
	cmd := s.run(ctx, s.journalBin(), s.managerArgs(units, "--lines=0", "--follow")...) // #nosec G204 -- argv slice, no shell; units are "--unit=" values
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("journalctl -f: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("journalctl -f: %w", err)
	}
	return func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}, stdout, nil
}

// ParseManagerEntry decodes one journal JSON line and keeps it only when
// systemd itself logged it about a unit.
func ParseManagerEntry(line string) (ManagerEntry, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return ManagerEntry{}, false
	}
	str := func(k string) string { return journalString(raw[k]) }
	if str("SYSLOG_IDENTIFIER") != "systemd" {
		return ManagerEntry{}, false
	}
	e := ManagerEntry{
		Unit:      str("USER_UNIT"),
		Message:   str("MESSAGE"),
		JobType:   str("JOB_TYPE"),
		JobResult: str("JOB_RESULT"),
	}
	if e.Unit == "" {
		e.Unit = str("UNIT")
	}
	if e.Unit == "" {
		return ManagerEntry{}, false
	}
	e.Priority, _ = strconv.Atoi(str("PRIORITY"))
	if usec, err := strconv.ParseInt(str("__REALTIME_TIMESTAMP"), 10, 64); err == nil {
		e.At = time.UnixMicro(usec)
	}
	return e, true
}

// journalString reads a journal JSON field: a string, null, or (for
// non-UTF-8 or binary payloads) an array of byte values.
func journalString(v json.RawMessage) string {
	if len(v) == 0 || string(v) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var b []byte
	var ints []int
	if json.Unmarshal(v, &ints) == nil {
		b = make([]byte, 0, len(ints))
		for _, i := range ints {
			if i < 0 || i > 0xff {
				return "" // not a byte array: refuse rather than truncate
			}
			b = append(b, byte(i))
		}
		return string(b)
	}
	return ""
}
