package systemd

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTimerDetails(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	// The fake answers both show calls: timers first, then the services.
	bin := fakeBin(t, "systemctl", argsFile, `case "$*" in
*backup.timer*) cat <<'END'
Id=backup.timer
Unit=backup.service
ActiveState=active
SubState=waiting
TimersCalendar={ OnCalendar=*-*-* *:00:00 ; next_elapse=@1700003600 }
TimersMonotonic={ OnBootUSec=5min ; next_elapse=0 }
Persistent=yes
RandomizedDelayUSec=2min
AccuracyUSec=1min
NextElapseUSecRealtime=@1700003600
LastTriggerUSec=@1700000000
UnitFileState=enabled
FragmentPath=/home/u/.config/systemd/user/backup.timer
END
;;
*) cat <<'END'
Id=backup.service
Result=exit-code
ExecMainStatus=125
END
;;
esac
`)
	s := &Systemd{User: true, Bin: bin}
	ts, err := s.TimerDetails(context.Background(), []string{"backup.timer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("timers = %v", ts)
	}
	tm := ts[0]
	if tm.Triggers != "backup.service" || !tm.Persistent || tm.RandomDelay != "2min" || tm.FileState != "enabled" {
		t.Errorf("timer = %+v", tm)
	}
	if len(tm.Schedules) != 2 || tm.Schedules[0] != "OnCalendar=*-*-* *:00:00" || tm.Schedules[1] != "OnBootUSec=5min" {
		t.Errorf("schedules = %q", tm.Schedules)
	}
	if !tm.Next.Equal(time.Unix(1700003600, 0)) || !tm.Last.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("next/last = %v / %v", tm.Next, tm.Last)
	}
	if tm.LastResult != "exit-code" || tm.LastStatus != 125 || !tm.Failed() {
		t.Errorf("last run = %q/%d failed=%v", tm.LastResult, tm.LastStatus, tm.Failed())
	}
	args := readArgs(t, argsFile)
	if !contains(args, "--timestamp=unix") {
		t.Errorf("show must request unix timestamps: %v", args)
	}
}

func TestParseRunHistory(t *testing.T) {
	raw := `{"JOB_TYPE":"start","JOB_RESULT":"done","__REALTIME_TIMESTAMP":"1700000000000000"}
{"MESSAGE":"hello","__REALTIME_TIMESTAMP":"1700000001000000"}
{"JOB_TYPE":"stop","JOB_RESULT":"done","__REALTIME_TIMESTAMP":"1700000002000000"}
{"JOB_TYPE":"start","JOB_RESULT":"failed","__REALTIME_TIMESTAMP":"1700000003000000"}
{"JOB_TYPE":"start","JOB_RESULT":"done","__REALTIME_TIMESTAMP":"1700000004000000"}
`
	runs := parseRunHistory(raw, 2)
	if len(runs) != 2 || runs[0].OK || !runs[1].OK {
		t.Fatalf("runs = %+v, want the last two start jobs (failed, done)", runs)
	}
	if !runs[1].At.Equal(time.UnixMicro(1700000004000000)) {
		t.Errorf("at = %v", runs[1].At)
	}
}

func TestParseManagerEntry(t *testing.T) {
	line := `{"SYSLOG_IDENTIFIER":"systemd","USER_UNIT":"demo-web.service","JOB_TYPE":"start","JOB_RESULT":"done","PRIORITY":"6","MESSAGE":"Started demo-web.service.","__REALTIME_TIMESTAMP":"1700000000000000"}`
	e, ok := ParseManagerEntry(line)
	if !ok || e.Unit != "demo-web.service" || e.JobType != "start" || e.JobResult != "done" || e.Priority != 6 {
		t.Fatalf("entry = %+v ok=%v", e, ok)
	}
	if !e.At.Equal(time.UnixMicro(1700000000000000)) {
		t.Errorf("at = %v", e.At)
	}
	// The unit's own output is not a manager message.
	if _, ok := ParseManagerEntry(`{"SYSLOG_IDENTIFIER":"demo-web","MESSAGE":"hello"}`); ok {
		t.Error("unit output must be skipped")
	}
	// Binary MESSAGE arrives as a byte array.
	bin := `{"SYSLOG_IDENTIFIER":"systemd","UNIT":"x.service","MESSAGE":[104,105],"PRIORITY":"3"}`
	if e, ok := ParseManagerEntry(bin); !ok || e.Message != "hi" || e.Unit != "x.service" || e.Priority != 3 {
		t.Errorf("byte-array entry = %+v ok=%v", e, ok)
	}
}
