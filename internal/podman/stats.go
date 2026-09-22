// Package podman resource reporting: live stats, container inventory for
// exec targeting, and guarded prune. Stats and inventory are read-only and
// reuse the default timeout; prune is destructive and gets its own long
// timeout, and the UI gates it behind readonly/remote checks plus an
// inactive-Quadlet warning before this package is ever called.
package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PruneTimeout bounds `podman system prune`: it can pull nothing but may
// walk gigabytes of storage, so 10s is far too short.
const PruneTimeout = 5 * time.Minute

// StatEntry is one row of `podman stats --no-stream --all --format json`.
// Field names are podman's snake_case JSON keys, verified against
// podman 6.1 (`cpu_percent`, `mem_usage`, `mem_percent`, `net_io`,
// `block_io`, `pids`). Stopped containers report zeros, never absent.
type StatEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	CPU     string `json:"cpu_percent"`
	Mem     string `json:"mem_usage"`
	MemPerc string `json:"mem_percent"`
	Net     string `json:"net_io"`
	Block   string `json:"block_io"`
	PIDs    string `json:"pids"`
}

// Stats returns resource usage for every container, running or stopped.
func Stats(ctx context.Context) ([]StatEntry, error) {
	out, err := runPodman(ctx, "stats", "--no-stream", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var entries []StatEntry
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return nil, fmt.Errorf("podman stats: %w", err)
	}
	return entries, nil
}

// ContainerNames returns every known container name, oldest first as
// `podman ps -a` reports them. Used to confirm an exec target exists
// before handing the terminal over.
func ContainerNames(ctx context.Context) ([]string, error) {
	out, err := runPodman(ctx, "ps", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" || trimmed == "null" {
		return nil, nil
	}
	var rows []psEntry
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		return nil, fmt.Errorf("podman ps: %w", err)
	}
	var names []string
	for _, r := range rows {
		names = append(names, r.Names...)
	}
	return names, nil
}

// ResolveExecTarget reports whether want names a known container. Matching
// is exact-only on purpose: exec hands the user a root shell inside the
// target, so prefix/substring guessing could land in the wrong container
// (prod vs staging with a shared prefix). The UI prefills the unit's base
// name but always lets the user correct it.
func ResolveExecTarget(names []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// SystemPrune runs `podman system prune -f` and returns its report.
// Destructive: the UI must refuse readonly/remote sessions and show the
// inactive-Quadlet warning before calling.
func SystemPrune(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, PruneTimeout)
	defer cancel()
	out, err := runCmd(ctx, "podman", "system", "prune", "-f").CombinedOutput() // #nosec G204 -- argv slice, no shell; fixed subcommand
	if err != nil {
		return "", fmt.Errorf("podman system prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
