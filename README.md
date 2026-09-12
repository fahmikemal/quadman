# quadman

**A terminal UI manager for rootless Podman [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units.**

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
- `start` / `stop` / `restart` / `enable` / `disable` / `daemon-reload` with
  one keypress, each with a spinner while it runs.
- Warns when quadlet files changed after the last `daemon-reload`, so you
  never wonder why your edits do nothing.
- Snapshot of the last 300 journal lines per unit.
- Optional enrichment via `podman quadlet list` (application/pod grouping)
  when podman is installed; fully functional without it.
- **Linger indicator and toggle** — `loginctl enable-linger` is the one setting every
  rootless container host needs (without it, your containers die at logout). quadman
  shows it in the status bar and toggles it with `L`.

quadman is engine-agnostic on purpose: it only drives the `systemctl --user` /
`journalctl --user` / `loginctl` CLIs, so it works with any Podman rootless setup and
never needs the Podman socket or API.

## Install

```sh
go install github.com/quadman-dev/quadman@latest
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
| `l`   | last 300 journal lines for the unit           |
| `s` / `x` / `r` | start / stop / restart the unit    |
| `e`   | enable the unit at boot and start it now      |
| `d`   | disable the unit from starting at boot        |
| `R`   | `systemctl --user daemon-reload` (regenerate after editing Quadlet files) |
| `L`   | toggle user linger (`loginctl enable-linger`) |
| `?`   | expand help                                   |
| `q` / `esc` | quit / back                             |

## Roadmap

- [ ] Follow mode for journals (`journalctl -f` streaming)
- [ ] Generate Quadlet files from `docker run` commands / compose files (via `podlet`)
- [ ] Manage drop-in directories (`*.container.d/*.conf`)
- [ ] SSH mode for remote rootless hosts
- [ ] Integration tests against a real Podman + Quadlet fixture

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
