<div align="center">

# quadman

**A terminal UI manager for rootless Podman [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units.**

[![CI](https://github.com/kemal-labs/quadman/actions/workflows/ci.yml/badge.svg)](https://github.com/kemal-labs/quadman/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kemal-labs/quadman)](https://github.com/kemal-labs/quadman/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/kemal-labs/quadman.svg)](https://pkg.go.dev/github.com/kemal-labs/quadman)
[![Go Report Card](https://goreportcard.com/badge/github.com/kemal-labs/quadman)](https://goreportcard.com/report/github.com/kemal-labs/quadman)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

Quadlet is the recommended way to run rootless containers as systemd services — but its
tooling is scattered across `systemctl`, `journalctl`, and `loginctl`. quadman puts the
whole lifecycle in one TUI:

- Lists every Quadlet source file systemd's generator picks up (`*.container`, `*.pod`,
  `*.kube`, `*.volume`, `*.network`, `*.image`, `*.build`, `*.artifact`), across the
  generator's user search path (`$XDG_RUNTIME_DIR/containers/systemd` →
  `~/.config/containers/systemd` → `/etc/containers/systemd/users[/UID]` →
  `/usr/share/containers/systemd/users[/UID]`), with first-match shadowing like the
  generator.
- Maps each source file to the systemd unit the generator produces
  (`webapp.container` → `webapp.service`, `stack.pod` → `stack-pod.service`,
  `cache.volume` → `cache-volume.service`, …), honoring `ServiceName=` overrides
  and template units (`web@.container` → `web@.service`).
- Shows live state (active/inactive/failed, `not-found` when you forgot
  `daemon-reload`) and the configured image, right in the list — refreshed
  automatically every few seconds with the cursor pinned on your selection.
- `start` / `stop` (with confirmation — Quadlet runs containers `--rm`) /
  `restart` / `enable` / `disable` / `daemon-reload` with one keypress, each
  with a spinner while it runs.
- Warns when quadlet files changed after the last `daemon-reload`, so you
  never wonder why your edits do nothing.
- **Follow-mode journals** — live `journalctl -f` tail per unit with
  stick-to-tail scrolling and pause (`f`).
- `/` fuzzy-filter the unit list as you type.
- Edit quadlet files in `$EDITOR` from the TUI; get nudged to regenerate
  when the file changed.
- **Auto-update screen** (`u`) — `podman auto-update --dry-run` preview and
  the `podman-auto-update.timer` state, toggleable with `U`.
- Container health: unhealthy containers surface in the STATE column, `h`
  runs `podman healthcheck run` on the selected unit.
- Optional enrichment via `podman quadlet list` (application/pod grouping)
  when podman is installed; fully functional without it.
- **Linger indicator and toggle** — `loginctl enable-linger` is the one setting every
  rootless container host needs (without it, your containers die at logout). quadman
  shows it in the status bar and toggles it with `L`.

quadman is engine-agnostic on purpose: it only drives the `systemctl --user` /
`journalctl --user` / `loginctl` CLIs, so it works with any Podman rootless setup and
never needs the Podman socket or API.

## Screenshots

The main list — every Quadlet source, its systemd unit, live state, and image,
with a `daemon-reload` warning when files changed on disk:

![quadman unit list](docs/screenshot-list.svg)

Selecting a unit streams its last 300 journal lines without leaving the TUI:

![quadman journal view](docs/screenshot-logs.svg)

## Install

Download a prebuilt binary (Linux amd64/arm64) from the
[Releases](https://github.com/kemal-labs/quadman/releases) page:

```sh
curl -LO https://github.com/kemal-labs/quadman/releases/latest/download/quadman_0.1.2_linux_amd64.tar.gz
tar -xzf quadman_0.1.2_linux_amd64.tar.gz && sudo install quadman /usr/local/bin/
```

Or with Go:

```sh
go install github.com/kemal-labs/quadman@latest
```

Or build from a clone:

```sh
make build   # ./quadman
```

Requirements: Linux with systemd, a user session (rootless-first by design), and
`journalctl` for log view. No daemon, no config file.

## Usage

```sh
quadman          # TUI
quadman list     # non-interactive overview for scripts and pipes
quadman -version
```

### Keys

| Key   | Action                                        |
| ----- | --------------------------------------------- |
| `↑/↓` `j/k` | move in the list                       |
| `enter` | view the Quadlet source file               |
| `/`   | fuzzy-filter the list (type to narrow, `esc` clears) |
| `l`   | live journal tail for the unit (`f` pauses follow, `/` searches with highlight, `n`/`N` jumps between matches) |
| `s` / `x` / `r` | start / stop (confirms) / restart the unit |
| `e`   | enable at boot (appends `[Install]` to the file, asks first) |
| `d`   | disable from boot (removes `[Install]`, asks first) |
| `E`   | edit the Quadlet file in `$EDITOR`            |
| `u`   | auto-update screen (`U` toggles the timer)    |
| `h`   | run `podman healthcheck` on the unit          |
| `R`   | `systemctl --user daemon-reload` (regenerate after editing Quadlet files) |
| `L`   | toggle user linger (`loginctl enable-linger`) |
| `?`   | expand help                                   |
| `q` / `esc` | quit / back                             |

## Compatibility

quadman targets **Podman 6.x** (latest: 6.1.1, Sep 2026) and works with Podman
5.3+ — the release that introduced `podman quadlet list`. It is stack-current:
bubbletea v2.0.9, bubbles v2.2.1, lipgloss v2.0.6 (all latest as of Sep 2026).

## Roadmap

Ships in batches (see [CONTRIBUTING.md](CONTRIBUTING.md)); minor bumps for
feature tiers, patch bumps for accumulated fixes.

### v0.2.0 — Tier 1: differentiators (✅ shipped)

- [x] Follow mode for journals (`journalctl -f` streaming, stick-to-tail)
- [x] `/` fuzzy filter over the unit list
- [x] Edit Quadlet files in `$EDITOR` (`tea.ExecProcess`) → offer `daemon-reload` on exit
- [x] Auto-update screen: per-unit `AutoUpdate=` state, `podman auto-update --dry-run`
      preview, user `podman-auto-update.timer` status/toggle
- [x] Health column + `h` runs `podman healthcheck run`
- [x] Stop confirmation — Quadlet runs containers `--rm`, stop deletes them
- [x] Surface new Podman 6.1 keys (e.g. `ImageVolume=`) in parsing
- [x] Tests for the non-interactive `list` command

### v0.3.0 — Tier 2: operational depth

- [ ] Manage drop-in directories (`*.container.d/*.conf`, cascading `foo-.container.d/`)
- [ ] Multi-document `.quadlets` files (`# FileName=` headers)
- [ ] Dependency tree view (bubbles `tree`): pod↔container, implicit `Image=`/`Network=`/`Volume=` deps
- [ ] Tabbed detail pane: source / status / journal / `podman inspect`
- [ ] Storage & events screens (`podman system df`, streaming `podman events`)
- [ ] Smart hints: `activating (timed-out)` → suggest `TimeoutStartSec=`/`Pull=`;
      Quadlet-diagnostics via generator `--dryrun` output (clean STDERR since Podman 6.1)
- [ ] Generate Quadlet files via [podlet](https://github.com/containers/podlet)
      (`docker run` / compose → preview → install → reload)

### v0.4.0+ — Tier 3: scale

- [ ] SSH mode for remote rootless hosts
- [ ] YAML config (refresh rate, log tail/buffer, theme) + Go-template custom commands
- [ ] Colorblind-safe themes, auto light/dark, OSC52 clipboard, opt-in mouse
- [ ] Bulk mark + mass actions; `--readonly` mode
- [ ] `teatest` e2e suite; VHS demo GIF; 500+ unit benchmark

### Notable ecosystem notes (Sep 2026)

- [podman-tui](https://github.com/containers/podman-tui) v2.0.0 (Sep 6, 2026)
  supports Podman 6 — and **still has no Quadlet management**, keeping this niche open.
- podlet v0.3.2 (May 2026) covers Podman ≤ 5.8; Podman 6 keys like
  `ImageVolume=` are not yet generated by it.
- `.artifact` units are no longer flagged experimental in the current docs;
  quadman already names them correctly (`foo.artifact` → `foo-artifact.service`).

## Related projects

- [podman-tui](https://github.com/containers/podman-tui) — official TUI for the Podman
  runtime (containers, pods, images). quadman complements it at the systemd/Quadlet
  layer.
- [podlet](https://github.com/k9withabone/podlet) — CLI generator for Quadlet files.
- [quadletman](https://github.com/mikkovihonen/quadletman) — web UI for Quadlet
  management.

Not affiliated with the Podman project.

## License

[MIT](LICENSE)
