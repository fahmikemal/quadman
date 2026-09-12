// Package podman wraps the optional podman CLI to enrich quadman with data
// it cannot get from systemctl alone (application/pod grouping, quadlet
// status). Everything degrades gracefully when podman is not installed.
package podman

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
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
	out, err := exec.CommandContext(ctx, "podman", "quadlet", "list", "--format", "json").Output()
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
