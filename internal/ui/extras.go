package ui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/podlet"
	"github.com/kemal-labs/quadman/internal/podman"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/shellwords"
)

// --- Storage screen (g) ---------------------------------------------------

type storageMsg struct {
	content string
	err     error
}

func storageCmd() tea.Cmd {
	return func() tea.Msg {
		text, err := podman.SystemDf(context.Background())
		return storageMsg{content: text, err: err}
	}
}

// --- Events stream (w) ----------------------------------------------------

func (m Model) startEvents() (tea.Model, tea.Cmd) {
	m.stopEvents()
	stop, stream, err := podman.EventsFollow(context.Background())
	if err != nil {
		m.setStatus("podman events: "+err.Error(), true)
		return m, nil
	}
	sess := &logSession{stop: stop, scan: bufio.NewScanner(stream), unit: "events"}
	sess.scan.Buffer(make([]byte, 256*1024), 256*1024)
	m.eventSess = sess
	m.eventLines = nil
	m.mode = modeEvents
	m.viewport.SetContent("")
	m.viewport.GotoBottom()
	m.resize()
	m.clearStatus()
	return m, followLine(sess)
}

func (m *Model) stopEvents() {
	if m.eventSess != nil {
		m.eventSess.stop()
		m.eventSess = nil
	}
}

// --- Generate via podlet (n) ----------------------------------------------

type generateMsg struct {
	content string
	err     error
}

func generateCmd(input string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		trimmed := strings.TrimSpace(input)
		out, err := generatePodlet(ctx, trimmed)
		return generateMsg{content: out, err: err}
	}
}

// generatePodlet routes trimmed user input to podlet compose or run. A
// "compose <path>" prefix forces compose mode; otherwise a path to an
// existing .yml/.yaml file (or any existing file) is compose input, and
// anything else must parse as a run command: the first word has to be
// podman, docker, or run (so `podlet` never receives a bare image name or
// shell metacharacters by accident). Words split quote-aware so quoted
// values with spaces survive intact.
func generatePodlet(ctx context.Context, trimmed string) (string, error) {
	if trimmed == "" {
		return "", fmt.Errorf("empty input: paste a `podman run ...` command or a compose file path")
	}
	rest, ok := strings.CutPrefix(trimmed, "compose ")
	if ok {
		path := strings.TrimSpace(rest)
		if path == "" {
			return "", fmt.Errorf("usage: compose <path-to-compose-file>")
		}
		return podlet.Compose(ctx, path)
	}
	if strings.HasSuffix(trimmed, ".yml") || strings.HasSuffix(trimmed, ".yaml") || fileExists(trimmed) {
		return podlet.Compose(ctx, trimmed)
	}
	args, err := shellwords.Split(trimmed)
	if err != nil {
		return "", fmt.Errorf("cannot parse command: %w", err)
	}
	if len(args) == 0 {
		return "", fmt.Errorf("empty input: paste a `podman run ...` command or a compose file path")
	}
	switch args[0] {
	case "podman", "docker":
		if len(args) < 2 || args[1] != "run" {
			return "", fmt.Errorf("podlet converts `podman run ...` commands (got %q); for compose files use: compose <path>", args[0])
		}
	case "run":
		// bare `run ...` is accepted as shorthand for `podman run ...`
		args = append([]string{"podman"}, args...)
	default:
		return "", fmt.Errorf("not a run command (starts with %q): use `podman run ...`, `docker run ...`, or `compose <path>`", args[0])
	}
	return podlet.Generate(ctx, args)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// installGenerated writes the generated quadlet into the user's config
// search dir and reloads the generator.
func (m Model) installGenerated() (tea.Model, tea.Cmd) {
	name := podlet.FileNameOf(m.genContent)
	if name == "" {
		m.setStatus("cannot determine a FileName from the generated output", true)
		return m, nil
	}
	content := m.genContent
	return m, tea.Batch(m.setBusy("write "+name+".container"),
		actionCmdHint("generate "+name, "written to ~/.config/containers/systemd and reloaded", func(ctx context.Context) (string, error) {
			dir, err := os.UserConfigDir()
			if err != nil {
				return "", err
			}
			target := filepath.Join(dir, "containers", "systemd", name+".container")
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return "", err
			}
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil { // #nosec G306 -- quadlet files are user config, not secrets
				return "", err
			}
			_, err = m.sys.DaemonReload(ctx)
			return target, err
		}))
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
		if u.Kind == quadlet.KindContainer && m.timerEnabled != "enabled" {
			if f, err := quadlet.Parse(u.Path); err == nil {
				if f.Section("Container").Get("AutoUpdate") == "registry" {
					hints = append(hints, fmt.Sprintf(
						"%s: AutoUpdate=registry is set but podman-auto-update.timer is disabled — enable it from the updates screen (u, then U).",
						u.UnitName))
				}
			}
		}
	}
	return hints
}
