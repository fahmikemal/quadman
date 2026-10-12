// Package podman wraps the optional podman CLI to enrich quadman with data
// it cannot get from systemctl alone (application/pod grouping, quadlet
// status, storage, secrets). Everything degrades gracefully when podman is
// not installed.
//
// Every call goes through a Client, which binds the CLI to one session
// runner (local, SSH, or sudo compartment). There is deliberately no
// package-level runner: concurrent sessions (quadman serve) and a session
// switch with commands still in flight must never redirect each other's
// podman calls.
package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/vercmp"
)

// DefaultTimeout bounds one read-only podman call. podman can be slow to
// cold-start, but these only run on refreshes and screen loads.
const DefaultTimeout = 10 * time.Second

// Client runs the podman CLI through one session runner. The zero value
// runs podman locally as the current user.
type Client struct {
	Runner remote.Runner
}

// New returns a Client bound to runner.
func New(runner remote.Runner) Client { return Client{Runner: runner} }

// cmd builds a podman invocation through the session runner.
func (c Client) cmd(ctx context.Context, args ...string) *exec.Cmd {
	return c.Runner.Command(ctx, "podman", args...)
}

// Reachable reports whether podman answers in this session: a PATH lookup
// locally, a `podman --version` probe through the runner otherwise.
func (c Client) Reachable(ctx context.Context) bool {
	if !c.Runner.Isolated() {
		return Available()
	}
	ctx, cancel := context.WithTimeout(ctx, remote.DialTimeout)
	defer cancel()
	_, err := c.Runner.Output(ctx, "podman", "--version")
	return err == nil
}

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
func (c Client) QuadletList(ctx context.Context) ([]Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := c.cmd(ctx, "quadlet", "list", "--format", "json").Output() // #nosec G204 -- constant argv, no shell
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
	Names  []string          `json:"Names"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

// healthOf extracts the health state podman embeds in the ps Status string:
// "Up 2 minutes (healthy)" / "(unhealthy)" / "(starting)" / no healthcheck.
// The parenthesized token is matched anywhere in the string (not only as a
// suffix), so trailing annotations podman may add never hide the state.
func healthOf(status string) string {
	for _, h := range []string{"healthy", "unhealthy", "starting"} {
		if strings.Contains(status, "("+h+")") {
			return h
		}
	}
	return ""
}

// SystemdUnitLabel is the label Quadlet stamps on every container it
// generates, naming the systemd unit that owns it.
const SystemdUnitLabel = "PODMAN_SYSTEMD_UNIT"

// Health is what `podman ps` says about the running containers.
type Health struct {
	// ByName maps container name → "healthy" / "unhealthy" / "starting"
	// for containers that define a healthcheck.
	ByName map[string]string
	// NameByUnit maps the PODMAN_SYSTEMD_UNIT label → container name for
	// every running Quadlet container, healthcheck or not. It is the
	// authoritative container identity: ContainerName= and ServiceName=
	// overrides resolve without guessing the systemd-%N default.
	NameByUnit map[string]string
}

// PsHealth reads the running containers' health and Quadlet ownership.
func (c Client) PsHealth(ctx context.Context) (Health, error) {
	h := Health{ByName: map[string]string{}, NameByUnit: map[string]string{}}
	out, err := c.run(ctx, "ps", "--format", "json")
	if err != nil {
		return h, err
	}
	if t := bytes.TrimSpace(out); len(t) == 0 || string(t) == "null" {
		return h, nil
	}
	var rows []psEntry
	if err := json.Unmarshal(out, &rows); err != nil {
		return h, fmt.Errorf("podman ps: %w", err)
	}
	for _, r := range rows {
		if len(r.Names) == 0 {
			continue
		}
		name := r.Names[0]
		if state := healthOf(r.Status); state != "" {
			h.ByName[name] = state
		}
		if unit := r.Labels[SystemdUnitLabel]; unit != "" {
			h.NameByUnit[unit] = name
		}
	}
	return h, nil
}

// HealthcheckRun executes `podman healthcheck run` and reports whether the
// container's healthcheck passed. Exit code 0 = passed, 1 = failed,
// 125 = command error (no healthcheck, no such container, …).
func (c Client) HealthcheckRun(ctx context.Context, container string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	err := c.cmd(ctx, "healthcheck", "run", "--", container).Run() // #nosec G204 -- argv slice, no shell; container name is behind a "--" separator
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
func (c Client) AutoUpdateDryRun(ctx context.Context) ([]AutoUpdateEntry, error) {
	out, err := c.run(ctx, "auto-update", "--dry-run", "--format", "json")
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

// run runs a read-only podman subcommand with the default timeout.
func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := c.cmd(ctx, args...).Output() // #nosec G204 -- argv slice, no shell; call sites pass fixed subcommands
	if err != nil {
		// podman explains failures ("image is in use by a container") on
		// stderr, which Output keeps on the ExitError.
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(bytes.TrimSpace(ee.Stderr)) > 0 {
			return nil, fmt.Errorf("podman %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("podman %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// QuadletRm removes an installed quadlet by name (e.g. "web.container")
// via `podman quadlet rm --force`, which stops running units first and
// keeps podman's application bookkeeping consistent. Callers must only use
// it for a quadlet that `podman quadlet list` reports at the path they
// mean to delete: the command addresses quadlets by name, not by path.
func (c Client) QuadletRm(ctx context.Context, name string) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := c.cmd(ctx, "quadlet", "rm", "--force", "--", name).CombinedOutput() // #nosec G204 -- argv slice, no shell; name behind "--"
	if err != nil {
		return fmt.Errorf("podman quadlet rm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// QuadletInstall installs a quadlet file or a multi-document .quadlets
// bundle via `podman quadlet install` (which also reloads systemd by default).
func (c Client) QuadletInstall(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := c.cmd(ctx, "quadlet", "install", "--", path).CombinedOutput() // #nosec G204 -- argv slice, no shell; path behind "--"
	if err != nil {
		return fmt.Errorf("podman quadlet install: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Inspect returns pretty-printed `podman inspect` output for a container.
func (c Client) Inspect(ctx context.Context, container string) (string, error) {
	out, err := c.run(ctx, "inspect", "--", container)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// SystemDf returns `podman system df --verbose` output (disk usage of
// images, containers, and volumes).
func (c Client) SystemDf(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "system", "df", "--verbose")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// EventsFollow starts `podman events --format json` (a streaming feed) and
// returns the process's stop function and its stdout stream.
func (c Client) EventsFollow(ctx context.Context) (stop func(), stream io.Reader, err error) {
	cmd := c.cmd(ctx, "events", "--format", "json") // #nosec G204 -- constant argv, no shell
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("podman events: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("podman events: %w", err)
	}
	stop = func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
	return stop, stdout, nil
}

// Version returns the installed podman client version, e.g. "6.1.1".
func (c Client) Version(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "version", "--format", "json")
	if err != nil {
		return "", err
	}
	var v struct {
		Client struct {
			Version string `json:"Version"`
		} `json:"Client"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("podman version: %w", err)
	}
	return v.Client.Version, nil
}

// VersionAtLeast reports whether installed podman is >= want (dotted
// numeric comparison, e.g. "6.1" <= "6.1.1").
func VersionAtLeast(have, want string) bool {
	return vercmp.AtLeast(have, want)
}

// SecretEntry represents one secret reported by `podman secret ls --format json`.
type SecretEntry struct {
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	CreatedAt string `json:"CreatedAt"`
	UpdatedAt string `json:"UpdatedAt"`
	Driver    string `json:"Driver"`
}

// SecretList runs `podman secret ls --format json` and returns all configured secrets.
func (c Client) SecretList(ctx context.Context) ([]SecretEntry, error) {
	out, err := c.run(ctx, "secret", "ls", "--format", "json")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var entries []SecretEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("podman secret ls: %w", err)
	}
	return entries, nil
}
