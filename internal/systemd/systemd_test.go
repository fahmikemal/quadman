package systemd

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeBin writes an executable script that records its arguments to argsFile
// and then runs body.
func fakeBin(t *testing.T, name, argsFile, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"" + argsFile + "\"\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestShowParsesAllUnits(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, `cat <<'EOF'
Id=webapp.service
LoadState=loaded
ActiveState=active
SubState=running
Description=Web App

Id=cache-volume.service
LoadState=not-found
ActiveState=inactive
SubState=dead
Description=cache-volume.service
EOF
`)
	s := &Systemd{User: true, Bin: bin}

	statuses, err := s.Show(context.Background(), []string{"webapp.service", "cache-volume.service"})
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("got %d statuses, want 2: %v", len(statuses), statuses)
	}
	web := statuses["webapp.service"]
	if web.ActiveState != "active" || web.SubState != "running" || web.Description != "Web App" {
		t.Errorf("webapp = %+v", web)
	}
	cache := statuses["cache-volume.service"]
	if cache.Loaded() {
		t.Errorf("cache should not be loaded: %+v", cache)
	}

	args := readArgs(t, argsFile)
	for _, want := range []string{"--user", "show", "--", "webapp.service", "cache-volume.service"} {
		if !contains(args, want) {
			t.Errorf("systemctl args %v missing %q", args, want)
		}
	}
}

func TestShowEmpty(t *testing.T) {
	s := &Systemd{User: true, Bin: "/nonexistent/systemctl"}
	statuses, err := s.Show(context.Background(), nil)
	if err != nil {
		t.Fatalf("Show with no units must not run systemctl: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("statuses = %v, want empty", statuses)
	}
}

func TestShowCommandError(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, `echo "Failed to connect to bus" >&2
exit 1
`)
	s := &Systemd{User: true, Bin: bin}
	_, err := s.Show(context.Background(), []string{"webapp.service"})
	if err == nil {
		t.Fatal("Show should fail when systemctl exits non-zero")
	}
	if !strings.Contains(err.Error(), "systemctl show") || !strings.Contains(err.Error(), "Failed to connect to bus") {
		t.Errorf("error should wrap verb and stderr, got %v", err)
	}
}

func TestShowTimeout(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, "sleep 2\n")
	s := &Systemd{User: true, Bin: bin, Timeout: 100 * time.Millisecond}
	_, err := s.Show(context.Background(), []string{"webapp.service"})
	if err == nil {
		t.Fatal("Show should fail when systemctl exceeds the timeout")
	}
}

func TestUnitAction(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, "")
	s := &Systemd{User: true, Bin: bin}

	out, err := s.UnitAction(context.Background(), "start", "webapp.service")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Errorf("out = %q, want empty", out)
	}
	args := readArgs(t, argsFile)
	want := []string{"--user", "start", "--", "webapp.service"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestUnitActionError(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, `echo "Unit not found" >&2
exit 1
`)
	s := &Systemd{User: true, Bin: bin}
	_, err := s.UnitAction(context.Background(), "stop", "webapp.service")
	if err == nil {
		t.Fatal("UnitAction should fail when systemctl exits non-zero")
	}
	if !strings.Contains(err.Error(), "stop webapp.service") || !strings.Contains(err.Error(), "Unit not found") {
		t.Errorf("error should wrap verb, unit and stderr, got %v", err)
	}
}

func TestEnableDisable(t *testing.T) {
	cases := []struct {
		desc string
		run  func(s *Systemd, ctx context.Context, unit string) (string, error)
		want string
	}{
		{
			desc: "enable now",
			run:  func(s *Systemd, ctx context.Context, unit string) (string, error) { return s.Enable(ctx, unit, true) },
			want: "--user enable --now -- webapp.service",
		},
		{
			desc: "enable without now",
			run:  func(s *Systemd, ctx context.Context, unit string) (string, error) { return s.Enable(ctx, unit, false) },
			want: "--user enable -- webapp.service",
		},
		{
			desc: "disable",
			run:  func(s *Systemd, ctx context.Context, unit string) (string, error) { return s.Disable(ctx, unit, false) },
			want: "--user disable -- webapp.service",
		},
		{
			desc: "disable now",
			run:  func(s *Systemd, ctx context.Context, unit string) (string, error) { return s.Disable(ctx, unit, true) },
			want: "--user disable --now -- webapp.service",
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			argsFile := filepath.Join(t.TempDir(), "args")
			bin := fakeBin(t, "systemctl", argsFile, "")
			s := &Systemd{User: true, Bin: bin}

			if _, err := tc.run(s, context.Background(), "webapp.service"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(strings.Split(strings.TrimRight(string(data), "\n"), "\n"), " "); got != tc.want {
				t.Errorf("args = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserGeneratorDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	if UserGeneratorDir() != "" {
		t.Error("UserGeneratorDir must be empty without XDG_RUNTIME_DIR")
	}
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := UserGeneratorDir(); got != filepath.Join("/run/user/1000", "systemd", "generator") {
		t.Errorf("UserGeneratorDir = %q", got)
	}
}

func TestDaemonReload(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "systemctl", argsFile, "")
	s := &Systemd{User: true, Bin: bin}

	if _, err := s.DaemonReload(context.Background()); err != nil {
		t.Fatal(err)
	}
	args := readArgs(t, argsFile)
	want := []string{"--user", "daemon-reload"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestJournal(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "journalctl", argsFile, `echo "2026-09-12T10:00:00+07:00 host webapp[1]: started"
`)
	s := &Systemd{User: true, JournalBin: bin}

	out, err := s.Journal(context.Background(), "webapp.service", 300)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "webapp[1]: started") {
		t.Errorf("out = %q, want journal line", out)
	}
	args := readArgs(t, argsFile)
	want := []string{"--user", "--no-pager", "--output=short-iso", "--unit=webapp.service", "--lines=300"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestJournalSystemScope(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "journalctl", argsFile, "")
	s := &Systemd{User: false, JournalBin: bin}

	if _, err := s.Journal(context.Background(), "webapp.service", 300); err != nil {
		t.Fatal(err)
	}
	if args := readArgs(t, argsFile); contains(args, "--user") {
		t.Errorf("system scope must not pass --user: %v", args)
	}
}

func TestJournalDefaultLines(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "journalctl", argsFile, "")
	s := &Systemd{User: true, JournalBin: bin}

	if _, err := s.Journal(context.Background(), "webapp.service", 0); err != nil {
		t.Fatal(err)
	}
	if args := readArgs(t, argsFile); !contains(args, "--lines=200") {
		t.Errorf("lines<=0 should default to 200: %v", args)
	}
}

func TestJournalFollow(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	bin := fakeBin(t, "journalctl", argsFile, `echo "line one"
echo "line two"
sleep 5
`)
	s := &Systemd{User: true, JournalBin: bin}

	stop, stream, err := s.FollowJournal(context.Background(), "webapp.service", 200)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	sc := bufio.NewScanner(stream)
	var got []string
	for sc.Scan() {
		got = append(got, sc.Text())
		if len(got) == 2 {
			break
		}
	}
	if len(got) != 2 || got[0] != "line one" || got[1] != "line two" {
		t.Errorf("streamed = %v", got)
	}
	args := readArgs(t, argsFile)
	if !contains(args, "--follow") || !contains(args, "--unit=webapp.service") {
		t.Errorf("follow args = %v", args)
	}
}

func TestIsEnabledIsActive(t *testing.T) {
	cases := []struct {
		script string
		want   string
	}{
		{"echo enabled; exit 0\n", "enabled"},
		{"echo disabled; exit 1\n", "disabled"}, // is-enabled exits 1 when disabled
		{"echo static; exit 0\n", "static"},
	}
	for _, tc := range cases {
		argsFile := filepath.Join(t.TempDir(), "args")
		bin := fakeBin(t, "systemctl", argsFile, tc.script)
		s := &Systemd{User: true, Bin: bin}
		got, err := s.IsEnabled(context.Background(), "podman-auto-update.timer")
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("IsEnabled = %q, want %q (script %q)", got, tc.want, tc.script)
		}
		args := readArgs(t, argsFile)
		if !contains(args, "--") || !contains(args, "podman-auto-update.timer") {
			t.Errorf("args = %v, want -- separator + unit", args)
		}
	}
}

func TestDisplay(t *testing.T) {
	cases := []struct {
		st        Status
		wantState string
		wantSub   string
	}{
		{Status{}, "-", "-"},
		{Status{Id: "a.service", LoadState: "loaded", ActiveState: "active", SubState: "running"}, "active", "running"},
		{Status{Id: "a.service", LoadState: "not-found"}, "not-found", "-"},
		{Status{Id: "a.service", LoadState: "masked", ActiveState: "inactive", SubState: "dead"}, "inactive", "dead"},
		{Status{Id: "a.service", LoadState: "loaded"}, "-", "-"},
	}
	for _, tc := range cases {
		if state, sub := tc.st.Display(); state != tc.wantState || sub != tc.wantSub {
			t.Errorf("Display(%+v) = %q,%q want %q,%q", tc.st, state, sub, tc.wantState, tc.wantSub)
		}
	}
}

func TestLoaded(t *testing.T) {
	if (Status{LoadState: "not-found"}).Loaded() {
		t.Error("not-found must not count as loaded")
	}
	if !(Status{LoadState: "loaded"}).Loaded() {
		t.Error("loaded must count as loaded")
	}
	if (Status{}).Loaded() {
		t.Error("empty state must not count as loaded")
	}
}
