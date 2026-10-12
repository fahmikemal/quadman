package systemd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// TestListTimersWithFakeBin: names come from list-units, times from
// show --timestamp=unix; nothing depends on locale or column layout.
func TestListTimersWithFakeBin(t *testing.T) {
	argsFile := t.TempDir() + "/args"
	next := time.Now().Add(12*time.Hour + 30*time.Second).Unix()
	last := time.Now().Add(-(time.Hour + 16*time.Minute + 30*time.Second)).Unix()
	body := fmt.Sprintf(`case "$*" in
  *list-units*) printf 'podman-auto-update.timer loaded active waiting Daily podman auto-update\nidle.timer loaded inactive dead Idle\n' ;;
  *show*) printf 'Id=podman-auto-update.timer\nUnit=podman-auto-update.service\nNextElapseUSecRealtime=@%%d\nLastTriggerUSec=@%%d\n\nId=idle.timer\nUnit=idle.service\nNextElapseUSecRealtime=\nLastTriggerUSec=n/a\n' %d %d ;;
esac
`, next, last)
	sys := New()
	sys.Bin = fakeBin(t, "systemctl", argsFile, body)
	timers, err := sys.ListTimers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 2 {
		t.Fatalf("expected 2 timers, got %+v", timers)
	}
	auto := timers[0]
	if auto.Unit != "podman-auto-update.timer" || auto.Activates != "podman-auto-update.service" {
		t.Errorf("timer = %+v", auto)
	}
	if auto.Left != "12h" || auto.Passed != "1h 16min ago" || auto.Next == "-" || auto.Last == "-" {
		t.Errorf("schedule = %+v", auto)
	}
	if idle := timers[1]; idle.Next != "-" || idle.Left != "-" || idle.Last != "-" || idle.Passed != "-" {
		t.Errorf("a timer that never ran and is not scheduled must read -: %+v", idle)
	}
}

func TestListTimersRemoteNeedsNoEnv(t *testing.T) {
	ssh := New()
	ssh.Remote = remote.Runner{Target: "user@host"}
	argv := ssh.run(context.Background(), ssh.bin(), ssh.args("list-units", "--type=timer")...).Args
	if got := argv[len(argv)-1]; got != "systemctl --user list-units --type=timer" {
		t.Errorf("remote command = %q", got)
	}
}

func TestSpan(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                              "0s",
		45 * time.Second:               "45s",
		29 * time.Minute:               "29min",
		time.Hour + 16*time.Minute:     "1h 16min",
		time.Hour:                      "1h",
		6 * 24 * time.Hour:             "6 days",
		24*time.Hour + 3*time.Hour:     "1 day 3h",
		30 * 24 * time.Hour:            "4 weeks 2 days",
		-(2*time.Hour + 5*time.Minute): "2h 5min",
	} {
		if got := Span(d); got != want {
			t.Errorf("Span(%v) = %q, want %q", d, got, want)
		}
	}
}
