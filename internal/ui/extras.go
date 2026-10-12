package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/shellwords"
)

// --- Generate via podlet (n) ----------------------------------------------

// generatePodlet routes trimmed user input to podlet compose, live-object
// generate ("container <name>" and friends), or run. A "compose <path>"
// prefix forces compose mode; otherwise a path to an existing .yml/.yaml
// file (or any existing file) is compose input; "<kind> <name>" for a
// convertible kind (container|pod|network|volume|image) converts that live
// object; anything else must parse as a run command: the first word has to
// be podman, docker, or run (so `podlet` never receives a bare image name
// or shell metacharacters by accident). Words split quote-aware so quoted
// values with spaces survive intact.
func generatePodlet(ctx context.Context, pl podlet.Client, trimmed string) (string, error) {
	rest, ok := strings.CutPrefix(trimmed, "compose ")
	if ok {
		path := strings.TrimSpace(rest)
		if path == "" {
			return "", fmt.Errorf("usage: compose <path-to-compose-file>")
		}
		return pl.Compose(ctx, path)
	}
	if strings.HasSuffix(trimmed, ".yml") || strings.HasSuffix(trimmed, ".yaml") || fileExists(trimmed) {
		return pl.Compose(ctx, trimmed)
	}
	args, err := shellwords.Split(trimmed)
	if err != nil {
		return "", fmt.Errorf("cannot parse command: %w", err)
	}
	if len(args) == 0 {
		return "", fmt.Errorf("empty input: paste a `podman run ...` command, a compose file path, or `<kind> <name>` (container|pod|network|volume|image)")
	}
	if len(args) == 2 && podlet.IsObjectKind(args[0]) {
		return pl.GenerateObject(ctx, args[0], args[1])
	}
	switch args[0] {
	case "podman", "docker":
		if len(args) < 2 || args[1] != "run" {
			return "", fmt.Errorf("podlet converts `podman run ...` commands (got %q); for compose files use: compose <path>; for live objects use: <kind> <name>", args[0])
		}
	case "run":
		// bare `run ...` is accepted as shorthand for `podman run ...`
		args = append([]string{"podman"}, args...)
	default:
		return "", fmt.Errorf("not a run command (starts with %q): use `podman run ...`, `docker run ...`, `compose <path>`, or `<kind> <name>`", args[0])
	}
	return pl.Generate(ctx, normalizeRunArgs(args))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// kubeYamlExists reports whether a .kube unit's Yaml= path resolves to a
// file. Relative paths resolve against the unit file's own directory, like
// the generator does. Remote sessions skip the check (no cheap remote
// stat); the caller gates on !IsRemote.
func kubeYamlExists(u quadlet.Unit, yml string) bool {
	if filepath.IsAbs(yml) {
		_, err := os.Stat(yml)
		return err == nil
	}
	_, err := os.Stat(filepath.Join(filepath.Dir(u.Path), yml))
	return err == nil
}

// kubeYamlExists reports whether a .kube unit's Yaml= path exists. Remote
// and compartment sessions read through the session runner (Cat errors
// when the file is missing); local sessions stat the resolved path
// directly.
func (m Model) kubeYamlExists(u quadlet.Unit, yml string) bool {
	if m.ssh.Isolated() {
		p := yml
		if !filepath.IsAbs(p) {
			p = filepath.Join(filepath.Dir(u.Path), yml)
		}
		_, err := m.readPath(p)
		return err == nil
	}
	return kubeYamlExists(u, yml)
}

// generatedFileName resolves the output file for generated Quadlet text from
// the podlet `# FileName=` header. The header is reduced to a bare base name
// so a hostile or malformed value cannot escape the target directory, and
// `.container` is appended only when the header carries no extension of its
// own (podlet may emit either `web` or `web.container`).
func generatedFileName(raw string) (string, error) {
	base := filepath.Base(strings.TrimSpace(raw))
	if base == "" || base == "." || base == "/" || base == ".." || strings.HasPrefix(base, ".") {
		return "", fmt.Errorf("invalid FileName header %q", raw)
	}
	if filepath.Ext(base) == "" {
		base += ".container"
	}
	switch filepath.Ext(base) {
	case ".container", ".pod", ".kube", ".volume", ".network", ".image", ".build", ".artifact":
		return base, nil
	}
	return "", fmt.Errorf("FileName %q is not a Quadlet unit (.container, .pod, .kube, .volume, .network, .image, .build, .artifact)", base)
}

// podletAvailable indirection keeps tests independent of the real PATH.
var podletAvailable = podlet.Available

// --- Smart hints ------------------------------------------------------------

// smartHints turns live state into actionable suggestions, shown in the
// problems view after the generator's own errors.
func (m Model) smartHints() []string {
	var hints []string
	for _, u := range m.units {
		if u.UnitName == "" {
			continue
		}
		st := m.status[u.UnitName]
		sub := strings.ToLower(st.SubState)
		switch {
		case strings.Contains(sub, "timed-out"):
			hints = append(hints, fmt.Sprintf(
				"%s: start timed out — the image pull probably exceeds systemd's 90s default. Add TimeoutStartSec=300 to [Service], or pre-pull / set Pull=missing in [Container].",
				u.UnitName))
		case strings.Contains(sub, "start-limit"):
			hints = append(hints, fmt.Sprintf(
				"%s: restart crash-loop (start-limit hit) — read the journal (l) for the root cause before lowering RestartSec= or changing Restart=.",
				u.UnitName))
		}
		if (u.Kind == quadlet.KindContainer || u.Kind == quadlet.KindKube) && m.timerEnabled != "enabled" {
			if f, err := m.parseUnitFile(u); err == nil {
				if pol := f.Section(u.Kind.Section()).Get("AutoUpdate"); pol == "registry" || pol == "local" {
					hints = append(hints, fmt.Sprintf(
						"%s: AutoUpdate=%s is set but podman-auto-update.timer is disabled — enable it from the updates screen (u, then U).",
						u.UnitName, pol))
				}
			}
		}
		if u.Kind == quadlet.KindKube {
			if f, err := m.parseUnitFile(u); err == nil {
				if yml := f.KubeYaml(); yml != "" && !m.kubeYamlExists(u, yml) {
					hints = append(hints, fmt.Sprintf(
						"%s: [Kube] Yaml=%s does not exist — kube play will fail at start; fix the path or restore the file.",
						u.UnitName, yml))
				}
			}
		}
	}
	return hints
}
