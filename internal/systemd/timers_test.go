package systemd

import (
	"context"
	"testing"
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
