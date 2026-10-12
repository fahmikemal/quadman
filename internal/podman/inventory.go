package podman

// Structured inventory for the storage, stats, events and secrets screens.
// Every call goes through the session runner, so SSH and compartment
// sessions read the remote podman. Field names below were verified against
// podman 6.1.1 JSON output.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DfRow is one row of `podman system df --format json` (Images,
// Containers, Local Volumes).
type DfRow struct {
	Type           string `json:"Type"`
	Total          int    `json:"Total"`
	Active         int    `json:"Active"`
	RawSize        int64  `json:"RawSize"`
	RawReclaimable int64  `json:"RawReclaimable"`
}

// SystemDfSummary returns the per-type disk usage totals.
func (c Client) SystemDfSummary(ctx context.Context) ([]DfRow, error) {
	var rows []DfRow
	if err := c.runJSON(ctx, &rows, "system", "df", "--format", "json"); err != nil {
		return nil, fmt.Errorf("podman system df: %w", err)
	}
	return rows, nil
}

// ImageEntry is one row of `podman images --format json`. Dangling images
// have no Names.
type ImageEntry struct {
	ID         string   `json:"Id"`
	Names      []string `json:"Names"`
	Size       int64    `json:"Size"`
	Created    int64    `json:"Created"`
	Containers int      `json:"Containers"`
}

// Dangling reports an untagged image (<none>:<none>).
func (e ImageEntry) Dangling() bool { return len(e.Names) == 0 }

// Ref is the first repository:tag, or "<none>:<none>".
func (e ImageEntry) Ref() string {
	if len(e.Names) == 0 {
		return "<none>:<none>"
	}
	return e.Names[0]
}

// Images lists local images.
func (c Client) Images(ctx context.Context) ([]ImageEntry, error) {
	var rows []ImageEntry
	if err := c.runJSON(ctx, &rows, "images", "--format", "json"); err != nil {
		return nil, fmt.Errorf("podman images: %w", err)
	}
	return rows, nil
}

// VolumeEntry is one row of `podman volume ls --format json`.
type VolumeEntry struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	CreatedAt  string            `json:"CreatedAt"`
	MountCount int               `json:"MountCount"`
	Labels     map[string]string `json:"Labels"`
}

// Volumes lists named volumes.
func (c Client) Volumes(ctx context.Context) ([]VolumeEntry, error) {
	var rows []VolumeEntry
	if err := c.runJSON(ctx, &rows, "volume", "ls", "--format", "json"); err != nil {
		return nil, fmt.Errorf("podman volume ls: %w", err)
	}
	return rows, nil
}

// ContainerEntry is one row of `podman ps -a --size --format json`.
type ContainerEntry struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Created int64             `json:"Created"`
	Labels  map[string]string `json:"Labels"`
	Size    *struct {
		RootFs int64 `json:"rootFsSize"`
		RW     int64 `json:"rwSize"`
	} `json:"Size"`
}

// Name is the first container name.
func (c ContainerEntry) Name() string {
	if len(c.Names) == 0 {
		return shortID(c.ID)
	}
	return c.Names[0]
}

// SystemdUnit is the PODMAN_SYSTEMD_UNIT label Quadlet stamps on the
// containers it generates ("" for containers it did not create).
func (c ContainerEntry) SystemdUnit() string { return c.Labels["PODMAN_SYSTEMD_UNIT"] }

// Containers lists every container with its writable-layer size. --size
// walks each container's layer, so callers fetch it on demand only.
func (c Client) Containers(ctx context.Context, withSize bool) ([]ContainerEntry, error) {
	args := []string{"ps", "-a", "--format", "json"}
	if withSize {
		args = append(args, "--size")
	}
	var rows []ContainerEntry
	if err := c.runJSON(ctx, &rows, args...); err != nil {
		return nil, fmt.Errorf("podman ps: %w", err)
	}
	return rows, nil
}

// VolumeSizes reads per-volume sizes from `podman system df --verbose`
// (the only podman command that reports them; it refuses --format json).
func (c Client) VolumeSizes(ctx context.Context) (map[string]string, error) {
	text, err := c.SystemDf(ctx)
	if err != nil {
		return nil, err
	}
	return ParseVolumeSizes(text), nil
}

// ParseVolumeSizes extracts "VOLUME NAME  LINKS  SIZE" rows from the
// "Local Volumes space usage" section of `podman system df --verbose`.
// Volume names never contain spaces, so the row splits on fields.
func ParseVolumeSizes(text string) map[string]string {
	out := map[string]string{}
	in := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "Local Volumes space usage"):
			in = true
			continue
		case strings.HasSuffix(strings.TrimSpace(line), "space usage:"):
			in = false
			continue
		}
		f := strings.Fields(line)
		if !in || len(f) < 3 || f[0] == "VOLUME" {
			continue
		}
		out[f[0]] = f[len(f)-1]
	}
	return out
}

// HostInfo is the subset of `podman info` the stats tiles need.
type HostInfo struct {
	CPUs     int   `json:"cpus"`
	MemTotal int64 `json:"memTotal"`
	MemFree  int64 `json:"memFree"`
}

// Info returns the host CPU count and memory.
func (c Client) Info(ctx context.Context) (HostInfo, error) {
	var v struct {
		Host HostInfo `json:"host"`
	}
	if err := c.runJSON(ctx, &v, "info", "--format", "json"); err != nil {
		return HostInfo{}, fmt.Errorf("podman info: %w", err)
	}
	return v.Host, nil
}

// Event is one decoded line of `podman events --format json`.
type Event struct {
	ID         string            `json:"ID"`
	Image      string            `json:"Image"`
	Name       string            `json:"Name"`
	Status     string            `json:"Status"`
	Type       string            `json:"Type"`
	Time       int64             `json:"time"`
	TimeNano   int64             `json:"timeNano"`
	Attributes map[string]string `json:"Attributes"`
	// HealthStatus is set on health_status events (podman 6.1 emits the
	// snake_case key, unlike the other fields).
	HealthStatus  string `json:"health_status"`
	ContainerExit *int   `json:"ContainerExitCode"`
}

// At is the event time.
func (e Event) At() time.Time {
	if e.TimeNano > 0 {
		return time.Unix(0, e.TimeNano)
	}
	return time.Unix(e.Time, 0)
}

// SystemdUnit is the Quadlet unit label carried by container events.
func (e Event) SystemdUnit() string { return e.Attributes["PODMAN_SYSTEMD_UNIT"] }

// ParseEvent decodes one events line; ok is false for non-JSON lines.
func ParseEvent(line string) (Event, bool) {
	var e Event
	if err := json.Unmarshal([]byte(line), &e); err != nil || e.Type == "" {
		return Event{}, false
	}
	return e, true
}

// EventsSince returns past events without following (--stream=false), so
// the events screen opens with the last hour already on it.
func (c Client) EventsSince(ctx context.Context, since string) ([]Event, error) {
	out, err := c.run(ctx, "events", "--since", since, "--stream=false", "--format", "json")
	if err != nil {
		return nil, err
	}
	var evs []Event
	for _, line := range strings.Split(string(out), "\n") {
		if e, ok := ParseEvent(strings.TrimSpace(line)); ok {
			evs = append(evs, e)
		}
	}
	return evs, nil
}

// SecretRemove deletes a secret.
func (c Client) SecretRemove(ctx context.Context, name string) error {
	_, err := c.run(ctx, "secret", "rm", "--", name)
	return err
}

// SecretCreate creates (or, with replace, rotates) a secret from value.
// The value travels on stdin, never in argv, so it does not show up in
// process listings or shell history.
func (c Client) SecretCreate(ctx context.Context, name string, value []byte, replace bool) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	args := []string{"secret", "create"}
	if replace {
		args = append(args, "--replace")
	}
	args = append(args, "--", name, "-")
	cmd := c.cmd(ctx, args...) // #nosec G204 -- argv slice, no shell; name is behind "--", value on stdin
	cmd.Stdin = bytes.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("podman secret create: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SecretValue reads a secret's plaintext (`podman secret inspect
// --showsecret`). Callers must never render it; it only feeds the
// clipboard.
func (c Client) SecretValue(ctx context.Context, name string) ([]byte, error) {
	out, err := c.run(ctx, "secret", "inspect", "--showsecret", "--format", "{{.SecretData}}", "--", name)
	if err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(out, []byte("\n")), nil
}

// AutoUpdateApply runs `podman auto-update` for real (pull, restart,
// roll back on failure) and returns what it did.
func (c Client) AutoUpdateApply(ctx context.Context) ([]AutoUpdateEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, PruneTimeout)
	defer cancel()
	out, err := c.cmd(ctx, "auto-update", "--format", "json").Output() // #nosec G204 -- constant argv, no shell
	if err != nil {
		return nil, fmt.Errorf("podman auto-update: %w", err)
	}
	var rows []AutoUpdateEntry
	if t := bytes.TrimSpace(out); len(t) > 0 && string(t) != "null" {
		if err := json.Unmarshal(t, &rows); err != nil {
			return nil, fmt.Errorf("podman auto-update: %w", err)
		}
	}
	return rows, nil
}

// runJSON runs podman and decodes its JSON stdout into v; empty or "null"
// output leaves v untouched.
func (c Client) runJSON(ctx context.Context, v any, args ...string) error {
	out, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	t := bytes.TrimSpace(out)
	if len(t) == 0 || string(t) == "null" {
		return nil
	}
	return json.Unmarshal(t, v)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// HumanBytes formats a byte count the way podman does ("1.42GB", "388MB").
func HumanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	suffix := []string{"kB", "MB", "GB", "TB", "PB"}[exp]
	switch {
	case v >= 100:
		return fmt.Sprintf("%.0f%s", v, suffix)
	case v >= 10:
		return fmt.Sprintf("%.1f%s", v, suffix)
	}
	return fmt.Sprintf("%.2f%s", v, suffix)
}

// ParseBytes reads podman's human sizes ("12.58MB", "1.2kB", "0B",
// "1.5GiB") back into bytes; unknown input yields 0.
func ParseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	var v float64
	if _, err := fmt.Sscanf(s[:i], "%g", &v); err != nil {
		return 0
	}
	mult := map[string]float64{
		"b": 1, "kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12,
		"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
	}[strings.ToLower(strings.TrimSpace(s[i:]))]
	return int64(v * mult)
}

// ImageRemove deletes an image by ID. It fails while a container uses it.
func (c Client) ImageRemove(ctx context.Context, id string) error {
	_, err := c.run(ctx, "rmi", "--", id)
	return err
}

// VolumeRemove deletes a named volume. It fails while a container uses it.
func (c Client) VolumeRemove(ctx context.Context, name string) error {
	_, err := c.run(ctx, "volume", "rm", "--", name)
	return err
}

// ContainerRemove deletes a stopped container by ID.
func (c Client) ContainerRemove(ctx context.Context, id string) error {
	_, err := c.run(ctx, "rm", "--", id)
	return err
}
