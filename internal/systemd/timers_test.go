package systemd

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/fahmikemal/quadman/internal/remote"
)

func TestParseTimers(t *testing.T) {
	raw := `NEXT                                  LEFT LAST                              PASSED UNIT                                      ACTIVATES
Sat 2026-09-19 10:22:10 WIB            20h Fri 2026-09-18 10:22:10 WIB 3h 37min ago launchpadlib-cache-clean.timer            launchpadlib-cache-clean.service
-                                        - Fri 2026-09-18 12:00:03 WIB 1h 59min ago snap.firmware-updater.timer               snap.firmware-updater.service

2 timers listed.
`
	timers := ParseTimers(raw)
	if len(timers) != 2 {
		t.Fatalf("len(timers) = %d, want 2", len(timers))
	}

	t0 := timers[0]
	if t0.Unit != "launchpadlib-cache-clean.timer" {
		t.Errorf("Unit = %q, want launchpadlib-cache-clean.timer", t0.Unit)
	}
	if t0.Activates != "launchpadlib-cache-clean.service" {
		t.Errorf("Activates = %q, want launchpadlib-cache-clean.service", t0.Activates)
	}
	if t0.Left != "20h" {
		t.Errorf("Left = %q, want 20h", t0.Left)
	}

	t1 := timers[1]
	if t1.Unit != "snap.firmware-updater.timer" {
		t.Errorf("Unit = %q, want snap.firmware-updater.timer", t1.Unit)
	}
	if t1.Next != "-" {
		t.Errorf("Next = %q, want -", t1.Next)
	}
}

// Real systemd 259 output: LEFT and PASSED are right-aligned, so values
// such as "6 days" start left of their header and spill across offsets.
func TestParseTimersRightAligned(t *testing.T) {
	raw := `NEXT                                  LEFT LAST                           PASSED UNIT                                          ACTIVATES
Mon 2026-10-12 00:00:00 WIB          29min -                                   - snap.firmware-updater.firmware-notifier.timer snap.firmware-updater.firmware-notifier.service
Mon 2026-10-12 22:13:58 WIB            22h Sun 2026-10-11 22:13:58 WIB 1h 16min ago launchpadlib-cache-clean.timer                launchpadlib-cache-clean.service
Sun 2026-10-18 22:17:20 WIB         6 days Sun 2026-10-11 22:17:20 WIB 1h 12min ago ubuntu-insights-upload.timer                  ubuntu-insights-upload.service
Wed 2026-11-11 08:43:33 WIB 4 weeks 2 days Sun 2026-10-11 22:13:33 WIB 1h 16min ago ubuntu-insights-collect.timer                 ubuntu-insights-collect.service

4 timers listed.
`
	ts := ParseTimers(raw)
	if len(ts) != 4 {
		t.Fatalf("timers = %d, want 4: %+v", len(ts), ts)
	}
	want := []Timer{
		{Next: "Mon 2026-10-12 00:00:00 WIB", Left: "29min", Last: "-", Passed: "-", Unit: "snap.firmware-updater.firmware-notifier.timer", Activates: "snap.firmware-updater.firmware-notifier.service"},
		{Next: "Mon 2026-10-12 22:13:58 WIB", Left: "22h", Last: "Sun 2026-10-11 22:13:58 WIB", Passed: "1h 16min ago", Unit: "launchpadlib-cache-clean.timer", Activates: "launchpadlib-cache-clean.service"},
		{Next: "Sun 2026-10-18 22:17:20 WIB", Left: "6 days", Last: "Sun 2026-10-11 22:17:20 WIB", Passed: "1h 12min ago", Unit: "ubuntu-insights-upload.timer", Activates: "ubuntu-insights-upload.service"},
		{Next: "Wed 2026-11-11 08:43:33 WIB", Left: "4 weeks 2 days", Last: "Sun 2026-10-11 22:13:33 WIB", Passed: "1h 16min ago", Unit: "ubuntu-insights-collect.timer", Activates: "ubuntu-insights-collect.service"},
	}
	for i := range want {
		if ts[i] != want[i] {
			t.Errorf("row %d = %+v\nwant     %+v", i, ts[i], want[i])
		}
	}
}

func TestListTimersWithFakeBin(t *testing.T) {
	argsFile := t.TempDir() + "/args"
	bin := fakeBin(t, "systemctl", argsFile, `cat <<'EOF'
NEXT                         LEFT LAST                         PASSED UNIT                  ACTIVATES
Sat 2026-09-19 10:00:00 UTC   12h Fri 2026-09-18 10:00:00 UTC  12h ago podman-auto-update.timer podman-auto-update.service

1 timers listed.
EOF
`)
	sys := New()
	sys.Bin = bin
	timers, err := sys.ListTimers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(timers))
	}
	if timers[0].Unit != "podman-auto-update.timer" {
		t.Errorf("Unit = %q", timers[0].Unit)
	}
}

func TestListTimersCmdLocale(t *testing.T) {
	local := New().listTimersCmd(context.Background())
	if !slices.Contains(local.Env, "LC_ALL=C") {
		t.Error("local list-timers must force LC_ALL=C for stable headers")
	}
	ssh := New()
	ssh.Remote = remote.Runner{Target: "user@host"}
	argv := ssh.listTimersCmd(context.Background()).Args
	if got := argv[len(argv)-1]; got != "LC_ALL=C systemctl --user list-timers --all --no-pager --full" {
		t.Errorf("remote command = %q", got)
	}
	comp := New()
	comp.Remote = remote.Runner{As: "svc"}
	got := strings.Join(comp.listTimersCmd(context.Background()).Args, " ")
	if got != "sudo -n -u svc LC_ALL=C -- systemctl --user list-timers --all --no-pager --full" {
		t.Errorf("compartment argv = %q (sudo carries the locale, no /usr/bin/env)", got)
	}
}
