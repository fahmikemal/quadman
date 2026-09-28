<div align="center">

# quadman

**A terminal UI manager for rootless Podman [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units.**

<p align="center">
  <a href="README.md"><b>English</b></a> | <a href="README.id.md"><b>Bahasa Indonesia</b></a>
</p>

[![CI](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml/badge.svg)](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fahmikemal/quadman)](https://github.com/fahmikemal/quadman/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/fahmikemal/quadman.svg)](https://pkg.go.dev/github.com/fahmikemal/quadman)
[![Go Report Card](https://goreportcard.com/badge/github.com/fahmikemal/quadman)](https://goreportcard.com/report/github.com/fahmikemal/quadman)
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
  stick-to-tail scrolling, pause (`f`), live grep filter (`F`), priority filter (`p`), export to `.log`/`.jsonl` (`S`), and OSC52 clipboard copy (`c`).
- **Command Palette** (`Ctrl+P`) — fuzzy-search and execute all unit lifecycle actions, screens, views, diagnostics, and user custom commands with live context-sensitive validation.
- **Native Wish SSH Daemon** (`quadman serve`) serves the TUI directly over SSH without installing quadman on the connecting machine: loopback-only by default, `authorized_keys` required for non-loopback binds, with password auth, host key generation, and readonly mode.
- **Dual Rootless / System Mode** (`--system`) — default rootless-first identity, with native rootful system support (`/etc/containers/systemd`, `/run/containers/systemd`) and automatic root detection (`sudo quadman`).
- `/` fuzzy-filter the unit list as you type.
- Edit quadlet files in `$EDITOR` from the TUI; get nudged to regenerate
  when the file changed.
- **Auto-update screen** (`u`) — `podman auto-update --dry-run` preview and
  the `podman-auto-update.timer` state, toggleable with `U`.
- Container health: unhealthy containers surface in the STATE column, `h`
  runs `podman healthcheck run` on the selected unit.
- **Built-in validation** — the Quadlet generator itself (dry-run) checks
  every file on refresh; units with problems get a ✗/⚠ marker and the `v`
  view explains each error, including version-gate hints for new keys.
- **Drop-in directories** — `foo.container.d/*.conf` (plus cascading
  `foo-.container.d/` and generic `container.d/`) shown merged in the file
  view, in the generator's merge order.
- **Dependency tree** (`t`) — pods → containers → image/network/volume
  units and `[Unit]` dependencies, rendered with the bubbles tree.
- Mouse: click a row to select it, wheel-scroll logs; `y`/`Y` copy names
  and images via OSC52 (works over SSH).
- **Tabbed detail pane** (`[`/`]`) — source, `systemctl status`, live journal,
  and `podman inspect` of the selected unit in one view.
- **Storage & events** — `g` for `podman system df --verbose`, `w` for a
  live `podman events` stream (pause with `f`).
- **Smart hints** — `timed-out` starts suggest `TimeoutStartSec=`/`Pull=`;
  crash-loops and `AutoUpdate=`-without-timer also get called out in the
  problems view.
- **Generate from anything** (`n`) — paste a `docker run` command or a
  compose file path; [podlet](https://github.com/containers/podlet) converts
  it, you preview the result, `y` writes it and reloads the generator.
- **Templates & bundles** — instantiate `web@.container` as
  `web@prod.service` (`i`), and preview/install multi-document `.quadlets`
  bundles (`I`).
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

![quadman unit list](docs/screenshot-list.png)

Selecting a unit streams its journal live (`journalctl -f`) without leaving
the TUI — pause with `f`, search with `/`:

![quadman journal view](docs/screenshot-logs.png)

Resource usage per container (`podman stats`), one keypress away:

![quadman resource stats](docs/screenshot-stats.png)

## Install

Download a prebuilt binary (Linux amd64/arm64) from the
[Releases](https://github.com/fahmikemal/quadman/releases) page:

```sh
curl -LO https://github.com/fahmikemal/quadman/releases/latest/download/quadman_0.6.0_linux_amd64.tar.gz
tar -xzf quadman_0.6.0_linux_amd64.tar.gz && sudo install quadman /usr/local/bin/
```

Or with Go:

```sh
go install github.com/fahmikemal/quadman@latest
```

Or build from a clone:

```sh
make build   # ./quadman
```

Requirements: Linux with systemd, a user session (rootless-first by design), and
`journalctl` for log view. No daemon; one optional YAML settings file (see
Config below).

## Usage

```sh
quadman                          # TUI (rootless user session by default)
quadman --system                 # native system-wide (rootful) mode (/etc/containers/systemd)
sudo quadman                     # auto-detects root and activates system mode
quadman serve                    # serve TUI over SSH via Wish daemon (default 127.0.0.1:2222)
quadman serve -p 2222 --readonly # serve as a read-only monitoring dashboard over SSH
quadman --readonly               # TUI with all state-changing actions disabled
quadman --ssh user@host          # manage a remote rootless host over SSH
quadman --theme colorblind       # auto, dark, light, or colorblind
quadman --mouse                  # opt-in click-to-select
quadman --quadlet-dir ~/quadlets # extra Quadlet source dir (repeatable)
quadman list                     # non-interactive overview for scripts and pipes
quadman --system list            # list system-wide quadlets (flags go before the command)
quadman --as svc-web list        # list another user's units via sudo (flags go before the command)
quadman --skill                  # export AI Agent Skill specification (markdown)
quadman --skill --skill-format=json # export AI Agent Skill specification in JSON format
quadman -version
```

### Keys

| Key   | Action                                        |
| ----- | --------------------------------------------- |
| `↑/↓` `j/k` | move in the list                       |
| `enter` | view the Quadlet source file               |
| `Ctrl+P` | command palette (fuzzy search and run any action, incl. pull image, or custom command) |
| `/`   | fuzzy-filter the list (type to narrow, `esc` clears) |
| `l`   | live journal tail (`f` pause, `/` search, `F` grep filter, `p` priority, `S` export, `c` copy) |
| `s` / `x` / `r` | start / stop (confirms) / restart the unit — or all `space`-marked units at once |
| `space` | mark/unmark the row for bulk actions (`esc` clears marks) |
| `e`   | enable at boot (appends `[Install]` to the file, asks first) |
| `d`   | disable from boot (removes `[Install]`, asks first) |
| `E`   | edit the Quadlet file in `$EDITOR`            |
| `X`   | open `/bin/sh` in the unit's container (`podman exec`, exact name match) |
| `u`   | auto-update screen (`U` toggles the timer)    |
| `h`   | run `podman healthcheck` on the unit          |
| `v`   | problems view — generator validation of every Quadlet file |
| `t`   | dependency tree (pods, images, networks, volumes, [Unit] deps) |
| `i`   | instantiate a template unit (`web@.container` → `web@prod.service`) |
| `D`   | delete the unit (stops it and removes the file, asks first) |
| `y` / `Y` | copy unit name / image to the clipboard (OSC52, works over SSH) |
| `[` / `]` | cycle detail tabs: source / status / journal / inspect |
| `I`   | install a `.quadlets` bundle (`podman quadlet install`) |
| `g`   | storage screen (`podman system df --verbose`, `r` refresh) |
| `o`   | resource stats screen (`podman stats`, `r` refresh) |
| `P`   | `podman system prune` (asks first, warns about inactive Quadlet units) |
| `T`   | systemd timers screen (view scheduled timers, countdowns, and triggers) |
| `K`   | secrets screen (view Podman secret store, drivers, and metadata) |
| `w`   | live podman events stream (`f` pause) |
| `n`   | generate a quadlet via podlet (`podman run ...`, `docker run ...`, `run ...` shorthand, `compose <path>`, or `<kind> <name>` for container\|pod\|network\|volume\|image) |
| `A`   | recent-actions log (what ran, when, and whether it worked) |
| `R`   | `systemctl --user daemon-reload` (regenerate after editing Quadlet files) |
| `L`   | toggle user linger (`loginctl enable-linger`) |
| `?`   | expand help                                   |
| `q` / `esc` | quit / back                             |

## Config

Optional YAML settings at `~/.config/quadman/config.yaml` (all keys
optional; a corrupt file is reported in the status line instead of
silently ignored):

```yaml
refresh_interval: 5s   # list poll tick (default 2.5s)
log_tail: 200          # journal snapshot lines for a new follow stream
log_buffer: 1000       # follow-view line cap
readonly: false        # same as quadman --readonly
theme: auto            # auto, dark, light, or colorblind
mouse: false           # same as quadman --mouse (off keeps text selection working)
quadlet_dirs:          # extra Quadlet source dirs (same as --quadlet-dir)
  - ~/quadlets
serve:
  authorized_keys: ~/.ssh/authorized_keys # required for non-loopback binds
  # address: 127.0.0.1:2222 # default; 0.0.0.0:2222 for LAN (needs authorized_keys)
  # port: "2222" # shorthand when address is unset (loopback)
  # host_key: ~/.config/quadman/host_ed25519 # default; generated when missing
  # password_file: /run/secrets/quadman-pass # or password: ... (only one)
  # readonly: false # serve read-only sessions

custom_commands:
  - name: status
    key: S
    run: systemctl --user status {{.UnitName}}
  - name: image
    key: G
    run: podman image inspect {{.Image}}
```

Custom keys must avoid the built-in list-mode keys (`s x r e d E u h v t i I D y Y g o P T K w n A R L ? q l X` and friends):
quadman warns at startup when a custom command is shadowed and the built-in
always wins.

Custom commands run without a shell: the `run` string is expanded as a Go
template (`{{.Name}}`, `{{.UnitName}}`, `{{.Kind}}`, `{{.Image}}`), split
quote-aware, and executed directly. Their results land in the status line
and the recent-actions log (`A`) like any built-in action. The editor
choice from first use still lives in `config.json` next to it.

## SSH mode

```sh
quadman --ssh user@host
```

Every CLI call (`systemctl`, `journalctl`, `loginctl`, `podman`) runs through
your own `ssh` binary — keys, agent, `known_hosts`, and `~/.ssh/config` all
keep working, and nothing new needs configuring. If `ssh user@host true`
works, quadman works.

Remote mode keeps full read and lifecycle control: list, live state, start /
stop / restart, journals, storage, stats, events, updates, health, linger, and the
dependency tree (files are read with `cat` on demand, never synced). Actions
that edit files on the host (edit, enable/disable at boot, instantiate,
generate-write, install, delete) are refused with an explanation — manage
files by running quadman on that host directly. Interactive or destructive
host actions are also refused remotely: `podman exec` (`X`) needs a local
TTY and `system prune` (`P`) must run on the host. Drop-ins are enumerated
remotely (read-only) and merged in the file view.

## Compartments (many OS users, one TUI)

A compartment is an ordinary Linux user whose Quadlet workloads you administer
without leaving quadman. `quadman --as svc-web` re-points every CLI call
(`systemctl`, `journalctl`, `loginctl`, `podman`) through non-interactive sudo
(`sudo -n -u svc-web`), scoped to that user's own Quadlet search path,
systemd user instance, podman storage, and secret store. The title bar shows
`[svc-web]`; file edits stay disabled, like SSH mode.

Requirements: `sudo -n -u <user> true` must succeed (NOPASSWD sudoers entry
or a root operator), and the target needs a runtime dir (`/run/user/<uid>` —
linger the user or log in once). When either is missing, quadman explains
why and keeps showing your own session instead of half-switching.

Configured compartments (`compartments: [svc-web, svc-db]` in config.yaml)
appear in the Command Palette (`Ctrl+P`) as "Use Compartment ..." actions,
plus "Use Own Session" to go back. Switching compartments over an SSH
session is refused (no nested sudo hops), and `--system` cannot be combined
with `--as` at all (system units live outside any user's session).
Non-interactive scripts use `quadman --as <user> list`.

```yaml
compartments:
  - svc-web
  - svc-db
```

## SSH Server (Wish daemon)

```sh
quadman serve                                          # listens on 127.0.0.1:2222 (loopback only)
quadman serve -p 2222 --readonly                       # read-only dashboard over SSH (loopback)
quadman serve -a 0.0.0.0:2222 --authorized-keys ~/.ssh/authorized_keys # LAN access, keys required
quadman serve --password secret123                     # password protected (loopback only)
quadman serve --password-file /run/secrets/quadman-pass # password from file, never in argv (loopback only)
quadman --as svc-web serve --readonly                  # serve another user's session via sudo compartment
```

quadman includes a native SSH server powered by [Charm Wish](https://github.com/charmbracelet/wish) (`wish/v2`). By default it binds loopback only. To share with your team on the LAN, bind explicitly with public-key auth, because non-loopback binds refuse to start without `--authorized-keys`:

```sh
quadman serve -a 0.0.0.0:2222 --authorized-keys ~/.ssh/authorized_keys
ssh -p 2222 user@host
```

- **Authentication Options**: Supports `authorized_keys` file verification or password protection (loopback only). Passwords come from exactly one of `--password`, `--password-file`, `QUADMAN_SERVE_PASSWORD`, or config `password`/`password_file`; prefer the file or env forms so the secret never appears in the process list. With neither configured on loopback, the server still starts but **forces all sessions into readonly mode** and says so loudly, and anonymous write access is never on by default, and open-auth/password-only binds never listen beyond loopback.
- **Readonly Dashboard Mode**: Pass `--readonly` to safely expose quadman as an observability dashboard for teammates without granting permission to start/stop units.
- **Safe Execution**: Local `$EDITOR` process hijacking is safely disabled in SSH server sessions.

## Quadlet locations

quadman scans the generator's rootless search path
(`$XDG_RUNTIME_DIR/containers/systemd` → `~/.config/containers/systemd` →
`/etc/containers/systemd/users[/UID]` → `/usr/share/containers/systemd/users[/UID]`),
plus your own directories from `quadlet_dirs` in config.yaml or repeatable
`--quadlet-dir` flags (e.g. a git checkout of your stack). Extra dirs are
listed after the standard path, so same-name files in the standard path keep
shadowing them — generator semantics stay intact.

Units that exist only in an extra dir carry a `~` marker: the generator
cannot see them, so starting one is refused with an explanation until the
file is copied or symlinked into a search-path directory and reloaded (`R`).
Applies to the TUI and `quadman list` alike.

## Compatibility

quadman targets **Podman 6.x** (latest: 6.1.1, Sep 2026) and works with Podman
5.3+ — the release that introduced `podman quadlet list`. It is stack-current:
bubbletea v2.0.10, bubbles v2.2.1, lipgloss v2.0.6 (all latest as of Sep 2026).

## quadman Roadmap

Moved to [ROADMAP.md](ROADMAP.md).

## Related projects

- [podman-tui](https://github.com/containers/podman-tui) — official TUI for the Podman
  runtime (containers, pods, images). quadman complements it at the systemd/Quadlet
  layer.
- [podlet](https://github.com/k9withabone/podlet) — CLI generator for Quadlet files.
- [quadletman](https://github.com/mikkovihonen/quadletman) — web UI for Quadlet
  management.

Not affiliated with the Podman project.

## Security

See [SECURITY.md](SECURITY.md) for supported versions, how to report a
vulnerability, and the shipped hardening defaults.

## License

[MIT](LICENSE)
