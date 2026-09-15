// Package podman wraps the optional podman CLI to enrich quadman with data
// it cannot get from systemctl alone (application/pod grouping, quadlet
// status). Everything degrades gracefully when podman is not installed.
package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Timeout bounds the podman call; 0 means DefaultTimeout. podman can be slow
// to cold-start, but this only runs on first load and after actions.
const DefaultTimeout = 10 * time.Second

// Entry is one row of `podman quadlet list --format json`.
type Entry struct {
	Name     string `json:"Name"`
	UnitName string `json:"UnitName"`
	Path     string `json:"Path"`
	Status   string `json:"Status"`
	App      string `json:"App"`
	Pod      string `json:"Pod"`
}

// Available reports whether a podman binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

// QuadletList runs `podman quadlet list --format json` and returns its rows.
func QuadletList(ctx context.Context) ([]Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "podman", "quadlet", "list", "--format", "json").Output() // #nosec G204 -- constant argv, no shell
	if err != nil {
		return nil, fmt.Errorf("podman quadlet list: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("podman quadlet list: %w", err)
	}
	return entries, nil
}

// ByUnit indexes entries by generated systemd unit name.
func ByUnit(entries []Entry) map[string]Entry {
	m := make(map[string]Entry, len(entries))
	for _, e := range entries {
		m[e.UnitName] = e
	}
	return m
}

// psEntry is the subset of `podman ps --format json` quadman reads.
type psEntry struct {
	Names  []string `json:"Names"`
	Status string   `json:"Status"`
}

// healthOf extracts the health state podman embeds in the ps Status string:
// "Up 2 minutes (healthy)" / "(unhealthy)" / "(starting)" / no healthcheck.
func healthOf(status string) string {
	if !strings.HasSuffix(status, ")") {
		return ""
	}
	open := strings.LastIndex(status, "(")
	if open < 0 {
		return ""
	}
	inner := status[open+1 : len(status)-1]
	switch inner {
	case "healthy", "unhealthy", "starting":
		return inner
	}
	return ""
}

// PsHealth returns container name → health state ("healthy" / "unhealthy" /
// "starting") for running containers that define a healthcheck.
func PsHealth(ctx context.Context) (map[string]string, error) {
	out, err := runPodman(ctx, "ps", "--format", "json")
	if err != nil {
		return nil, err
	}
	var rows []psEntry
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("podman ps: %w", err)
	}
	m := map[string]string{}
	for _, r := range rows {
		if len(r.Names) == 0 {
			continue
		}
		if h := healthOf(r.Status); h != "" {
			m[r.Names[0]] = h
		}
	}
	return m, nil
}

// HealthcheckRun executes `podman healthcheck run` and reports whether the
// container's healthcheck passed. Exit code 0 = passed, 1 = failed,
// 125 = command error (no healthcheck, no such container, …).
func HealthcheckRun(ctx context.Context, container string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	err := exec.CommandContext(ctx, "podman", "healthcheck", "run", "--", container).Run() // #nosec G204 -- argv slice, no shell; container name is behind a "--" separator
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("podman healthcheck run %s: %w", container, err)
}

// AutoUpdateEntry is one row of `podman auto-update --dry-run --format json`.
type AutoUpdateEntry struct {
	Container string `json:"Container"`
	Image     string `json:"Image"`
	Policy    string `json:"Policy"`
	Unit      string `json:"Unit"`
	Updated   string `json:"Updated"` // true / false / failed / pending
}

// AutoUpdateDryRun reports which labeled containers an update would touch,
// without pulling or restarting anything.
func AutoUpdateDryRun(ctx context.Context) ([]AutoUpdateEntry, error) {
	out, err := runPodman(ctx, "auto-update", "--dry-run", "--format", "json")
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil // no labeled containers: podman prints nothing at all
	}
	var entries []AutoUpdateEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("podman auto-update: %w", err)
	}
	return entries, nil
}

// runPodman runs a read-only podman subcommand with the default timeout.
func runPodman(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "podman", args...).Output() // #nosec G204 -- argv slice, no shell; call sites pass read-only subcommands
	if err != nil {
		return nil, fmt.Errorf("podman %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
