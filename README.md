<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/logo/quadman-logo-dark.svg">
  <img src="docs/logo/quadman-logo-light.svg" alt="quadman" width="360">
</picture>

**A Terminal UI manager for rootless Podman [Quadlet](https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html) units.**

---

<p align="center">
  <a href="README.md"><b>English</b></a> | <a href="README.id.md"><b>Bahasa Indonesia</b></a>
</p>

[![CI](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml/badge.svg)](https://github.com/fahmikemal/quadman/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/fahmikemal/quadman?include_prereleases)](https://github.com/fahmikemal/quadman/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/fahmikemal/quadman.svg)](https://pkg.go.dev/github.com/fahmikemal/quadman)
[![golangci-lint](https://img.shields.io/badge/linted_with-golangci--lint-blue)](https://golangci-lint.run/)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

</div>

Quadlet is the recommended way to run rootless containers as systemd services — but its
tooling is scattered across `systemctl`, `journalctl`, and `loginctl`. quadman puts the
whole lifecycle in one TUI:

- Lists every Quadlet source file systemd's generator picks up (`*.container`, `*.pod`,
  `*.kube`, `*.volume`, `*.network`, `*.image`, `*.build`, `*.artifact`), across the
  generator's user search path (`$XDG_RUNTIME_DIR/containers/systemd` →
  `~/.config/containers/systemd` → `/etc/containers/systemd/users[/UID]` →
  `/usr/share/containers/systemd/users[/UID]`), including their subdirectories,
  with first-match shadowing like the generator.
- Maps each source file to the systemd unit the generator produces
  (`webapp.container` → `webapp.service`, `stack.pod` → `stack-pod.service`,
  `cache.volume` → `cache-volume.service`, …), honoring `ServiceName=` overrides
  and template units (`web@.container` → `web@.service`).
- Shows live state (active/inactive/failed, `not-found` when you forgot
  `daemon-reload`) and the configured image, right in the list — refreshed
  automatically every few seconds with the cursor pinned on your selection.
- `start` / `stop` (with confirmation — Quadlet runs containers `--rm`) /
  `restart` / `enable` / `disable` / `daemon-reload` with one keypress, each
  with a spinner while it runs; a slow first start (a big image pull) is
  waited on for up to 5 minutes and then reported as still running, not as
  failed.
- Warns when quadlet files changed after the last `daemon-reload`, so you
  never wonder why your edits do nothing — straight from systemd's own
  `NeedDaemonReload`, so it is right over SSH and in compartments too.
- **Follow-mode journals** — live `journalctl -f` tail per unit with
  stick-to-tail scrolling, pause (`f`), live grep filter (`F`), priority filter (`p`), export to `.log`/`.jsonl` (`S`), and OSC52 clipboard copy (`c`).
- **Command Palette** (`Ctrl+P`) — fuzzy-search and execute all unit lifecycle actions, screens, views, diagnostics, and user custom commands with live context-sensitive validation.
- **Native Wish SSH Daemon** (`quadman serve`) serves the TUI directly over SSH without installing quadman on the connecting machine: loopback-only by default, `authorized_keys` required for non-loopback binds, password auth (loopback-only, throttled), host key generation, readonly mode, and fully isolated sessions.
- **Dual Rootless / System Mode** (`--system`) — default rootless-first identity, with native rootful system support (`/etc/containers/systemd`, `/run/containers/systemd`) and automatic root detection (`sudo quadman`); `system: true` in config.yaml makes it the default.
- **Remote hosts** (`--ssh user@host`) — the whole TUI (and `list`) against another machine through your own `ssh`: one multiplexed connection, every command quoted for the remote shell, nothing installed or synced on the host.
- **Compartments** (`--as svc-web`) — manage another OS user's Quadlets (their search path, user manager, podman storage, and secrets) through non-interactive sudo, with a sudoers rule that lists exactly the CLIs quadman runs.
- **Safety by design** — one capability policy decides what each session may do (readonly, SSH, compartment, served); confirmations say what will happen and what stays, and anything that loses data (unit, volume, secret, prune) waits for its name to be typed.
- `/` fuzzy-filter the unit list as you type.
- Edit quadlet files in `$EDITOR` from the TUI; get nudged to regenerate
  when the file changed.
- **Auto-update screen** (`u`) — `podman auto-update --dry-run` preview and
  the `podman-auto-update.timer` state, toggleable with `U`.
- Container health: unhealthy containers surface in the STATE column, `h`
  runs `podman healthcheck run` on the selected unit. The container is
  found by Quadlet's `PODMAN_SYSTEMD_UNIT` label, so `ContainerName=` and
  `ServiceName=` overrides just work.
- **Built-in validation** — the Quadlet generator itself (dry-run) checks
  every file at startup and after each `daemon-reload` (on the SSH host, or
  as the compartment user, when the session is remote); units with
  problems get a ✗/⚠ marker and the `v` view explains each error,
  including version-gate hints for new keys.
- **Drop-in directories** — `foo.container.d/*.conf` (plus cascading
  `foo-.container.d/` and generic `container.d/`) shown merged in the file
  view, in the generator's merge order.
- **Dependency tree** (`t`) — units grouped by pod and podman app, each
  with the pod, image, networks, volumes and `[Unit]` deps it references.
  Missing references are marked `✗ missing`, healthy subtrees start folded
  (`←/→`, `z` all), `f` shows problems only, and `i` adds an impact line
  (what blocks the unit, what needs it).
- Mouse: click a row to select it, wheel-scroll logs; `y`/`Y` copy names
  and images via OSC52 (works over SSH).
- **Tabbed detail pane** (`[`/`]`) — an overview (state, restarts, result,
  boot, ports, volumes, secrets, dependencies, problems), the source,
  `systemctl status`, the live journal in time/source/message columns with
  levels as glyph + color, and `podman inspect`.
- **Dashboard** — six tiles (units, running, failed, generator, linger,
  auto-update) and a "needs attention" block that names each problem and
  the key that fixes it; problems sort first in the table.
- **Storage** (`g`) — `podman system df` as tiles, then images, volumes and
  containers as tabs, each linked to the quadlet units that use it; unused
  objects group under UNUSED with the space a prune would reclaim. Pull,
  remove (refused while a unit uses it) and prune, all behind a confirm;
  removing a volume or pruning waits for the name (or `prune`) typed.
- **Events** (`w`) — one live timeline merged from `podman events`, the
  service manager's own messages about your units (started, failed,
  scheduled restart) and quadman's actions; filter by type (`1`-`6`),
  unit (`u`) and window (`s`), pause (`f`), export (`S`).
- **Timers** (`T`), **Secrets** (`K`), **Actions** (`A`) — human schedules
  with a 24-run ✓✗ history; which unit uses which secret and how, with
  values never rendered (masked entry on stdin, clipboard cleared after
  30s); a session audit with duration, trigger, the equivalent command and
  a repeat key (a repeat asks the original confirmation again and never
  replays into another session).
- **Smart hints** — `timed-out` starts suggest `TimeoutStartSec=`/`Pull=`;
  crash-loops and `AutoUpdate=`-without-timer also get called out in the
  problems view.
- **Generate from anything** (`n`) — paste a `docker run` command or a
  compose file path; [podlet](https://github.com/containers/podlet) converts
  it, you preview the result, `y` writes it (atomically, into
  `~/.config/containers/systemd` or `/etc/containers/systemd` in system
  mode) and reloads the generator.
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

The main list — dashboard tiles, a *needs attention* block that names the key
for each problem, then every Quadlet with its live state, uptime, and image
(problems sort first):

![quadman unit list](docs/screenshot-list.png)

Press `l` on a unit to stream its journal live (`journalctl -f`) without
leaving the TUI — warnings and errors are marked, pause with `f`, search with
`/`, filter by priority with `p`:

![quadman journal view](docs/screenshot-logs.png)

Resource usage per container (`podman stats`, key `o`) with a CPU history,
each container linked back to its Quadlet unit:

![quadman resource stats](docs/screenshot-stats.png)

## Install

Download a prebuilt binary (Linux amd64/arm64) from the
[Releases](https://github.com/fahmikemal/quadman/releases) page:

```sh
curl -LO https://github.com/fahmikemal/quadman/releases/download/v0.1.0-dev1/quadman_0.1.0-dev1_linux_amd64.tar.gz
tar -xzf quadman_0.1.0-dev1_linux_amd64.tar.gz && sudo install quadman /usr/local/bin/
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
Config below). Optional, per feature:

| Feature | Needs |
| ------- | ----- |
| app/pod grouping, health, storage, secrets, stats, events | `podman` 5.3+ (6.x recommended) |
| problems view (generator dry-run) | `podman-system-generator` (ships with podman) |
| `n` generate | [`podlet`](https://github.com/containers/podlet) |
| `--ssh user@host` | your `ssh` client; a POSIX-compatible login shell on the host |
| `--as <user>` | non-interactive sudo to that user (root, or a `NOPASSWD:SETENV` rule — see Compartments) and a runtime dir for them |

Building from source needs Go 1.26.8 or newer.

## Usage

```sh
quadman                          # TUI (rootless user session by default)
quadman --system                 # native system-wide (rootful) mode (/etc/containers/systemd)
sudo quadman                     # auto-detects root and activates system mode
quadman serve                    # serve TUI over SSH via Wish daemon (default 127.0.0.1:2222)
quadman serve -p 2222 --readonly # serve as a read-only monitoring dashboard over SSH
quadman --readonly               # TUI with all state-changing actions disabled
quadman --ssh user@host          # manage a remote rootless host over SSH
quadman --ssh user@host list     # non-interactive overview of the remote host
quadman --theme colorblind       # auto, dark, light, or colorblind
quadman --mouse                  # opt-in click-to-select
quadman --quadlet-dir ~/quadlets # extra Quadlet source dir (repeatable)
quadman list                     # non-interactive overview for scripts and pipes
quadman --system list            # list system-wide quadlets (flags go before the command)
quadman --as svc-web list        # list another user's units via sudo (flags go before the command)
quadman --skill                  # export AI Agent Skill specification (markdown)
quadman --skill --skill-format=json # export AI Agent Skill specification in JSON format
quadman --ascii                  # ASCII status glyphs (automatic on TERM=linux)
quadman -version                 # block-art splash on a terminal, one line when piped
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
| `e`   | enable at boot (adds `WantedBy=default.target` — `multi-user.target` in system mode — to `[Install]`, asks first) |
| `d`   | disable from boot (removes the boot keys from `[Install]`, keeps the rest, asks first) |
| `E`   | edit the Quadlet file in `$EDITOR`            |
| `X`   | open `/bin/sh` in the unit's container (`podman exec`, exact name match; not over SSH or in served sessions) |
| `u`   | auto-update screen (`U` toggles the timer)    |
| `h`   | run `podman healthcheck` on the unit          |
| `v`   | problems view — generator validation of every Quadlet file |
| `t`   | dependency tree (pods, images, networks, volumes, [Unit] deps) |
| `i`   | instantiate a template unit (`web@.container` → `web@prod.service`) |
| `D`   | delete the unit (stops it and removes the file; type the unit name to confirm) |
| `y` / `Y` | copy unit name / image to the clipboard (OSC52, works over SSH) |
| `[` / `]` | cycle detail tabs: overview / source / status / journal / inspect |
| `I`   | install a `.quadlets` bundle (`podman quadlet install`) |
| `g`   | storage: images / volumes / containers with USED BY (`[ ]` tabs, `p` pull, `D` remove — type the volume name for volumes, `S` sort) |
| `o`   | resource stats: CPU bars, sort with `←/→`, `i` idle containers, live re-sample |
| `P`   | `podman system prune` (type `prune` to confirm; warns about inactive Quadlet units) |
| `T`   | timers: schedule, next/last run, run history (`n` run now, `s`/`x`, `e`/`d`, `l` last run log) |
| `K`   | secrets: USED BY · AS, missing and unused first (`n` create, `r` rotate, `c` copy value, `D` remove) |
| `w`   | events timeline (`f` pause, `1`-`6` type, `u` this unit, `s` window, `S` export) |
| `n`   | generate a quadlet via podlet (`podman run ...`, `docker run ...`, `run ...` shorthand, `compose <path>`, or `<kind> <name>` for container\|pod\|network\|volume\|image) |
| `A`   | session actions: result, duration, trigger, command (`.` repeat with the original confirmation, `c` copy command) |
| `S`   | cycle the list order: problems first / name / kind |
| `z`   | collapse or expand the dashboard tiles |
| `R`   | `systemctl --user daemon-reload` (`systemctl daemon-reload` in system mode; regenerate after editing Quadlet files) |
| `L`   | toggle linger (`loginctl enable-linger`) for the session's own user — you, the remote login user, or the compartment user |
| `?`   | expand help                                   |
| `q` / `esc` | quit / back                             |

## Config

Optional YAML settings at `~/.config/quadman/config.yaml` (all keys
optional). The file is read strictly and fails closed: a malformed file or
an unknown key (a typo such as `read_only:`) is reported, the TUI starts
**readonly** until it is fixed, and `quadman serve` refuses to start —
quadman never guesses settings that decide who may change what.

```yaml
refresh_interval: 5s   # list poll tick (default 2.5s)
log_tail: 200          # journal snapshot lines for a new follow stream
log_buffer: 1000       # follow-view line cap
readonly: false        # same as quadman --readonly
theme: auto            # auto, dark, light, or colorblind
mouse: false           # same as quadman --mouse (off keeps text selection working)
dashboard: true        # false starts the list with the tiles collapsed (z toggles)
system: false          # true: manage system units by default (like --system; never applies to --as)
ssh_multiplex: true    # --ssh shares one connection (OpenSSH ControlMaster); false opts out
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
    key: C
    run: systemctl --user status {{.UnitName}}
  - name: image
    key: M
    run: podman image inspect {{.Image}}
```

Custom keys must avoid the built-in list-mode keys
(`/ ? A d D e E g h i I l L n o P q r R s S t T K u v w x X y Y z` and space):
quadman warns at startup when a custom command is shadowed, and the
built-in always wins. A custom command on one of the table's navigation
keys (`j k b f g G u d`) works but takes that navigation key over; quadman
says so at startup too.

Custom commands run without a shell: the `run` string is first split
quote-aware into words, then each word is expanded as a Go template
(`{{.Name}}`, `{{.UnitName}}`, `{{.Kind}}`, `{{.Image}}`), so a value from a
unit file stays inside the word it was written in and can never add
arguments. They run through the session — on the SSH host or as the
compartment user, like every built-in action — and their results land in
the status line and the recent-actions log (`A`). The editor choice from
first use still lives in `config.json` next to it.

## SSH mode

```sh
quadman --ssh user@host
```

Every CLI call (`systemctl`, `journalctl`, `loginctl`, `podman`) runs through
your own `ssh` binary — keys, agent, `known_hosts`, and `~/.ssh/config` all
keep working, and nothing new needs configuring. If `ssh user@host true`
works, quadman works. Calls share one multiplexed connection (OpenSSH
`ControlMaster` with a private socket, closed when quadman exits), so a
refresh costs one handshake, not one per command; set `ssh_multiplex:
false` to opt out. OpenSSH hands the remote login shell a single command
line, so quadman quotes every word: the remote login shell must be
POSIX-compatible (sh, dash, bash, zsh, ksh).

Remote mode keeps full read and lifecycle control: list (also `quadman --ssh
user@host list`), live state, start / stop / restart, template instances,
journals, storage, stats, events, updates, health, linger (for the remote
login user), problems (the host's own generator dry-run), the stale-reload
banner, bundle install (`podman quadlet install` on the host), prune (type
`prune` to confirm), and the dependency tree (files are read with `cat` on
demand, never synced). Actions that edit files through quadman itself
(edit, enable/disable at boot, generate-write, delete) are refused with an
explanation — run quadman on that host for those. `podman exec` (`X`) is
refused too: the batch SSH runner carries no interactive terminal. Drop-ins
are enumerated remotely (read-only) and merged in the file view. `--ssh`
cannot be combined with `--as` (no nested sudo hops) or with `serve`.

## Compartments (many OS users, one TUI)

A compartment is an ordinary Linux user whose Quadlet workloads you administer
without leaving quadman. `quadman --as svc-web` re-points every CLI call
(`systemctl`, `journalctl`, `loginctl`, `podman`) through non-interactive sudo
(`sudo -n -u svc-web`), scoped to that user's own Quadlet search path,
systemd user instance, podman storage, and secret store. The top bar shows
`quadman › rootless › as svc-web`; file edits stay disabled, like SSH mode.

Requirements: non-interactive sudo to the user must work with the session
variables quadman passes (`sudo -n -u <user> XDG_RUNTIME_DIR=… HOME=… -- <cmd>`),
and the target needs a runtime dir (`/run/user/<uid>` — linger the user or
log in once). A root operator needs nothing else; anyone else needs a
`NOPASSWD:SETENV` rule for the CLIs quadman runs as that user — no
`/usr/bin/env`, no shell:

```
operator ALL=(svc-web) NOPASSWD:SETENV: /usr/bin/systemctl, /usr/bin/journalctl, \
    /usr/bin/loginctl, /usr/bin/podman, /usr/bin/podlet, /usr/bin/cat, /usr/bin/ls, \
    /usr/bin/true, /usr/libexec/podman/podman-system-generator
```

(Adjust paths to your distribution; the generator path enables the
problems view.) `quadman --as <user>`, `--as <user> list`, and `--as <user>
serve` stop at startup with the reason when sudo, the user, or the runtime
dir is missing; switching from the Command Palette keeps your own session
instead of half-switching.

Configured compartments (`compartments: [svc-web, svc-db]` in config.yaml)
appear in the Command Palette (`Ctrl+P`) as "Use Compartment ..." actions,
plus "Use Own Session" to go back. Switching compartments over an SSH
session is refused (no nested sudo hops), served sessions never switch (a
`quadman --as svc-web serve` client stays on svc-web), and `--system`
cannot be combined with `--as` at all (system units live outside any user's
session). Compartments are always user sessions, also for a root operator
and also when config.yaml sets `system: true`.
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

- **Authentication Options**: Supports `authorized_keys` file verification or password protection. Passwords are **loopback-only, even next to keys** — a LAN bind authenticates with keys alone — and failed attempts are throttled per client address (a delay per failure, a lockout after 5 failures within a minute). Passwords come from exactly one of `--password`, `--password-file`, `QUADMAN_SERVE_PASSWORD`, or config `password`/`password_file`; prefer the file or env forms so the secret never appears in the process list. With neither configured on loopback, the server still starts but **forces all sessions into readonly mode** and says so loudly; anonymous write access is never on by default. Loopback is not a user boundary: on a shared host every local account can connect, so prefer keys there.
- **Readonly Dashboard Mode**: Pass `--readonly` to safely expose quadman as an observability dashboard for teammates without granting permission to start/stop units.
- **Isolated sessions**: every connection gets its own clients and state; nothing one client does re-targets another. Served sessions never launch an editor or a shell, never write files on the daemon's host (log export), never copy secret values, and never switch compartments.
- **Theme**: the daemon has no terminal of its own, so `auto` resolves to dark; pick `--theme light` or `colorblind` explicitly.
- **Config**: `serve` refuses to start when config.yaml is unusable instead of serving guessed defaults.

## Quadlet locations

quadman scans the generator's rootless search path
(`$XDG_RUNTIME_DIR/containers/systemd` → `~/.config/containers/systemd` →
`/etc/containers/systemd/users[/UID]` → `/usr/share/containers/systemd/users[/UID]`),
plus your own directories from `quadlet_dirs` in config.yaml or repeatable
`--quadlet-dir` flags (e.g. a git checkout of your stack). Like the
generator, every directory is searched recursively (`apps/web/web.container`
counts; `*.d` drop-in directories and other users' numeric
`users/<UID>` directories do not), and a symlinked directory or file is
followed. Extra dirs are listed after the standard path, so same-name files
in the standard path keep shadowing them — generator semantics stay intact.
Editing a symlinked unit (enable/disable at boot) replaces the link's target
in place and keeps the link.

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

quadman is built to be left running against production hosts:

- **One policy, every surface** — keys and Command Palette ask the same
  capability policy, so readonly, SSH, compartment, and served sessions
  refuse the same things, always with the reason.
- **Isolated sessions** — every SSH-served session has its own clients;
  nothing one client does re-targets another, and a served compartment
  stays pinned to its user.
- **Fail closed** — an unusable `config.yaml` starts the TUI readonly and
  stops `serve`; network binds need `authorized_keys`, passwords are
  loopback-only and throttled.
- **No shells** — every command is an argv; over SSH each word is quoted
  for the remote shell; custom commands split before template expansion;
  secrets travel on stdin and are never rendered.
- **Safe edits** — quadlet files are written atomically, symlinks are
  followed, mode and owner are kept.

See [SECURITY.md](SECURITY.md) for supported versions, how to report a
vulnerability, and every shipped hardening default, and
[DESIGN.md](DESIGN.md) for the interaction rules (glyphs, confirmations,
layout).

## License

[MIT](LICENSE)
