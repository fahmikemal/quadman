# Changelog

> Bahasa Indonesia: [CHANGELOG.id.md](CHANGELOG.id.md).

All notable changes to quadman are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- Switching to Updates, Storage, Timers, Secrets, or Stats no longer
  flashes the unit list's dashboard while the data loads: the screen opens
  on the keypress, with its last data or a loading state.
- A screen's data that lands after you left it no longer pulls you back
  (leaving Stats mid-sample used to reopen it).
- Creating, rotating, or removing a secret and removing storage reload the
  list after the action, not concurrently with it, so the list shows the
  result.
- Opening a screen from the journal or the events view (palette, nav keys)
  stops its `journalctl -f` / `podman events` stream.
- Switching session (compartment) drops every screen's data from the
  previous target.
- Switching between screens is fast:
  - data reopened within 30 s (updates: 5 minutes) is shown as is, with
    its age; `r` reloads;
  - a load you leave is cancelled, so its podman processes stop instead
    of piling up behind the next screen;
  - storage, timers, secrets, updates, and stats run their independent
    reads in parallel (storage: five podman calls at once);
  - stats samples one at a time and no longer runs `podman info` and
    `podman ps` on every tick.

### Changed

- An unattended quadman uses far less CPU: after a minute without input,
  or while the terminal is unfocused, the unit list polls every 15 s
  instead of every 2.5 s (the top bar shows `15s idle`); the next key,
  click, or focus refreshes at once. A slower `refresh_interval` is kept.
- Linger is read every 12 polls (and on full refreshes and session
  switches) instead of on every poll; `L` reports its own result.

## [v0.7.0-beta.1] - 2026-10-12

First beta. Every earlier version (`v0.1.0` to `v0.6.0`, and
`v0.1.0-dev1`) is retracted in `go.mod`: their code predates this
release line, and the Go module proxy keeps them cached, so this line
continues above them instead of reusing their numbers.

### Security

- **Served sessions are isolated.** `quadman serve` sessions shared the
  podman/podlet runner and the UI styles: one client switching
  compartment redirected every other session's podman calls (secrets,
  storage, prune) to another user, and concurrent connections raced on
  the styles. Every session now has its own clients, and presentation is
  configured once before the first session.
- A client of `quadman --as <user> serve` could leave the compartment for
  the operator's own session ("Use Own Session" was always enabled, even
  readonly) or switch to another configured compartment. Served sessions
  can no longer switch sessions at all.
- Password auth is loopback-only even next to `--authorized-keys` (a LAN
  bind with both used to accept the password from the network), and
  failed password attempts are throttled per client address.
- Readonly mode no longer lets `I` arm a bundle install. Key handlers and
  palette entries now share one capability policy, which also ends the
  palette/key disagreements for exec and prune.
- Repeating an action (`A`, `.`) used a bare `y` even for actions first
  confirmed by typing a name (delete, secret removal, prune) and could
  replay after a session switch against another host or user. Repeats now
  ask the original confirmation and are refused across sessions.
- Removing a volume and `system prune` now need the name (or `prune`)
  typed, like deleting a unit.
- Custom commands expanded templates before splitting, so a unit file's
  value (Image=…) could add arguments; words are now split first and
  each is expanded on its own. They also run through the session runner
  instead of always on the local host.
- Compartment sudo carries the session variables as sudo `VAR=value`
  arguments, so sudoers no longer has to allow `/usr/bin/env` (which
  permitted any command); the probe uses the same form.
- An unusable config.yaml (malformed or unknown keys) starts the TUI
  readonly and stops `serve`; a corrupt config.json no longer drops the
  YAML settings (including `readonly: true`).

### Changed

TUI redesign (DESIGN.md v0.2). The chrome and the unit list:

- New 13-token palette for the dark, light and colorblind themes. The
  main background is never painted; only the top bar, status line,
  tiles, modals and the selected row get a surface.
- Every status shows a glyph and a word (`● active`, `✗ failed`,
  `▲ unhealthy`, `◐ starting`, `◌ not-found`, `◇ template`), never color
  alone. `--ascii` (automatic on `TERM=linux`) swaps in ASCII glyphs.
- The top bar shows `quadman › scope › target` with refresh and podman
  version chips. A nav strip lists every view with its key and badge
  count (problems, pending updates).
- The unit list gains a dashboard: six tiles (units, running, failed,
  generator, linger, auto-update) and a "needs attention" block of up to
  4 problems with the keys that fix them. Below 100 columns or 30 rows it
  collapses to one summary line; `z` toggles it, and `dashboard: false`
  in config.yaml starts it collapsed.
- Table columns are now STATE · QUADLET · KIND · SUB · UPTIME · IMAGE.
  Problems sort first (failed → unhealthy → starting → not-found →
  active → inactive) and `S` cycles to name or kind order. Below 100
  columns UPTIME and IMAGE are hidden. The systemd unit name moves to the
  detail views.
- From 160 columns, an overview pane for the selected unit sits beside
  the list.
- The footer is grouped (UNIT · INSPECT · VIEWS · HOST, plus SELECT
  from 160 columns), capped at 2 rows, and always ends with
  `^P all commands  ? help`. Linger, readonly and system chips moved to
  the status line, along with the list position.
- The command palette floats over the screen it was opened from: a
  centered 84-column box (a full-width sheet below 100 columns) with
  actions grouped by category, the target unit beside the input and a
  match count.
- Below 60×16 the UI shows a "terminal too small" notice with the compact
  logo.

Every other screen now shares one skeleton: nav strip, summary tiles, a
table with problems first, a detail panel for the selected row (full
screen on `↵` below 100 columns), a `/` filter, and its own footer group.
Nav-strip keys switch views from any screen.

- Storage (`g`): `podman system df` tiles, then images, volumes and
  containers as tabs (`[ ]`). A USED BY column links each object to the
  quadlet units that use it; unused objects group under UNUSED with the
  space a prune would reclaim. `p` pulls, `D` removes (refused while a
  unit uses it), `P` prunes, `S` sorts by size or name.
- Events (`w`): one live timeline merged from `podman events`, the
  service manager's messages about quadlet units (started, failed,
  scheduled restart) and quadman's own actions, newest first. Filters
  for type (`1`-`6`), unit (`u`) and window (`s`: 15m, 1h, 6h, 24h);
  `f` pauses, `S` exports TSV.
- Timers (`T`): schedules in words ("daily 03:00", "every 1d"),
  next/last run with exit code, and the last 24 runs of the triggered
  service as a ✓✗ strip. `n` runs now, `s`/`x` start/stop, `e`/`d`
  enable/disable, `l` shows the last run's log, `E` edits a timer file
  you own.
- Secrets (`K`): USED BY · AS (env or file target) from every Secret=
  line; missing and unused secrets rise to the top. Values are never
  rendered: `n` creates and `r` rotates through a masked prompt sent to
  podman on stdin (rotation restarts the running units that use it),
  `c` copies a value and clears the clipboard after 30s (refused in
  readonly and served sessions), `D` removes after typing the name.
- Actions (`A`): the session audit records duration, how each action
  was triggered (key, palette, confirmed) and cancelled confirmations; a
  failure shows its error, the equivalent `systemctl` command (`c`
  copies it) and the unit's problem. `.` repeats an action after a
  confirm; `S` exports TSV.
- Stats (`o`): host tiles with a CPU sparkline, inline CPU bars, ←/→
  sorting by CPU, memory, network, name or PIDs, idle containers folded
  under one heading (`i`), and a re-sample on every poll tick.
- Updates (`u`): timer, next/last run and pending tiles; one row per
  container with its result (↑ pending, = current, ✓ updated, ✗ failed,
  ↺ rolled back). `a` applies updates now after a confirm.
- Problems (`v`): each finding located in its file with a quoted
  excerpt and a `^` marker, plus numbered fixes ("did you mean
  pgdata.volume?"). `E` opens the editor at that line (vi, nano, emacs,
  helix, VS Code and others).
- Tree (`t`): units grouped by pod and podman app, with their pod,
  image, network, volume and `[Unit]` references; missing references are
  marked, healthy subtrees start folded (`←/→`, `z` all), `f` shows
  problems only, `i` adds a "blocked by / needed by" impact line.
- Detail: a new Overview tab (state, restarts, result, boot, ports,
  volumes, secrets, dependencies, problems), a header with uptime and
  restart count (a crash-loop hint from 3 restarts), and a journal in
  time · source · message columns with levels as glyph + color and
  container IDs shortened. Line numbers stay on the source tab only.
- Generate (`n`): one tab per generated file, a check row (generator
  dry-run against your search path, host ports another unit publishes,
  files that already exist), and toggles for boot (`b`) and start after
  writing (`space`).
- Confirmations are a modal over the screen that lists what WILL happen
  and what it KEEPS, with focus on Cancel. Bulk stop/restart list the
  units and those that depend on them; delete (err border) stays disabled
  until the unit name is typed. `y` still confirms the plain dialogs.
- `quadman -version` prints the block-art logo with podman, systemd, Go
  and commit versions on a terminal, and the plain one-line version when
  piped.
- New logo (1a, "Quad Grid"): a 2×2 grid whose bottom-right cell is the
  one you manage, beside the same half-block wordmark the splash prints.
  The README header uses it as SVG with light and dark variants
  (`docs/logo/`); the one-colour version draws that cell hollow. It
  replaces the hexagon logo in the header, and the `### quadman` title
  under it, which the wordmark now carries.
- `--ssh` behavior: remote commands are quoted for the remote POSIX shell
  (OpenSSH re-parses the command line); calls share one multiplexed
  connection (`ssh_multiplex: false` opts out); template instances, bundle
  install, and prune now work over SSH; `quadman --ssh host list` lists
  the host. `--ssh` with `--as`, and `serve` with `--ssh`, are rejected.
- `quadman --as <user>` stops at startup when the compartment cannot be
  reached (like `list` and `serve`) instead of opening the operator's own
  session. Root operators may use `--as`; compartments are always user
  sessions, also when config.yaml sets `system: true`.
- Discovery walks subdirectories of every search path like the
  generator; symlinked directories and files are followed.
- Section and key names are case-sensitive, like systemd and Quadlet.
- Enable at boot uses `multi-user.target` for system units, adds the key
  to an existing `[Install]` section, and respects `RequiredBy=`/`UpheldBy=`;
  disable keeps other `[Install]` keys and refuses (with the path) when a
  drop-in enables the unit.
- Generated files go to `/etc/containers/systemd` in system mode, only
  Quadlet extensions are accepted from podlet's `# FileName=`, and start
  after writing honors `ServiceName=`.
- Start/stop/restart wait up to 5 minutes; a job systemd keeps running
  past that is reported as pending, not failed.
- The timers screen reads `list-units --plain` names and `show
  --timestamp=unix` times instead of the localized `list-timers` table.
- Log export levels match whole words ("interrupt" is no longer an
  error).
- CI pins govulncheck and gosec, shuffles the race tests, and builds the
  e2e module.

### Added

- UPTIME, from systemd's `ActiveEnterTimestamp` (read with
  `systemctl show --timestamp=unix`).
- systemd status now carries `Result`, `ExecMainStatus` and `NRestarts`.
- `NeedDaemonReload` in the status (drives the stale-reload banner), the
  `ssh_multiplex` setting, `quadman --ssh host list`, and `DESIGN.md`.

### Fixed

- `podlet compose` received `-f`, which podlet 0.3 rejects; the compose
  file is now passed positionally.
- A compose file with several services was written into one file; each
  generated file is now written separately.
- A pasted `podman run -d …` failed in podlet (it requires
  `--detach=true`); leading `-d`/`--detach` flags are rewritten.
- `list-timers` parsing cut right-aligned columns at header offsets
  ("6 days" became "days", PASSED became "in ago"); rows are now parsed
  by their structure.
- podman errors now include podman's own stderr explanation.
- `podman quadlet list --format` was split by the remote shell over SSH, so
  discovery fell back to listing every user service (dbus, pipewire, …) as
  a "container"; the fallback without podman now keeps only units whose
  `SourcePath=` is a Quadlet file.
- Linger over SSH queried the local user name on the remote host, and in a
  compartment the operator's linger; both now use the session's own user.
- `sudo quadman --as <user>` failed with "--system and --as cannot be
  combined"; `quadman --ssh host list` silently listed local units.
- The stale-reload banner was always on in compartments (the operator
  cannot stat another user's runtime dir); stale detection now uses
  systemd's `NeedDaemonReload`, correct for local, SSH, and compartment
  sessions. Validation and secret checks run through the session too.
- Health, healthcheck, inspect, events, and stats guessed the container as
  `systemd-<file name>`, missing `ContainerName=` and `ServiceName=`; the
  `PODMAN_SYSTEMD_UNIT` label (then `ContainerName=`, then `systemd-%N`)
  now names it everywhere.
- Enable/disable at boot replaced a symlinked quadlet with a regular file,
  detaching it from its git checkout; writes now follow the link, fsync,
  and keep the mode (and owner as root).
- Poll ticks stacked refreshes on slow sessions; one poll refresh runs at
  a time, every refresh is bounded, and results from a previous session or
  out of order are dropped.
- Deleting a unit passed a path to `podman quadlet rm`, which takes a
  name; it is now used only when podman reports that exact file.
- Secrets screen: `Secret = x` (spaces) and drop-in references are found.
- Two tests depended on `TERM` and on `/run/user/<uid>`; gosec G115 in the
  journal byte-array decoder.
- `DESIGN.md`, referenced by the code and this changelog, is now in the
  repository.

## [v0.1.0-dev1] - 2026-10-05 [YANKED]

Initial prerelease, withdrawn; everything in it ships in `v0.7.0-beta.1`.

### Added

- Project logo (`quadman_final_blue.png`) in the README header.
- This changelog.

### Changed

- Install docs point at the `v0.1.0-dev1` prerelease assets via an
  explicit tag URL (`/latest/` skips prereleases).
- Security support table tracks `v0.1.0-dev1`; older lines unsupported.
- Roadmap reframed around tiers, with the collected serve batch
  shipped as `v0.1.0-dev1`.

### Fixed

Full-audit hardening batch:

- Mouse clicks no longer move the cursor while a list prompt
  (generate/instance/exec) is focused.
- Updates screen probes podman through the session runner, so SSH and
  compartment sessions read the remote podman.
- Generated Quadlet file names are sanitized to a base name and gain a
  `.container` suffix only when extensionless.
- `serve` warns on a corrupt config instead of silently serving defaults.
- Atomic file writes use unique temp files; log export uses exclusive
  (`O_EXCL`) creation with suffix retry.
- `$EDITOR` values with arguments (`code --wait`) are split quote-aware.
- Linger key matching simplified to the `L` / `shift+l` forms.
- `list-timers` parsing forces `LC_ALL=C` (local env, `env` prefix remote).
- Container health matches `(healthy|unhealthy|starting)` anywhere in
  the status string; list filter matches per rune (non-ASCII safe).
- Bulk actions report each unit's own failure reason (capped, `+N more`).
- podman/podlet session runners are atomic (no data race between
  session switches and in-flight refreshes).
- Extra Quadlet dirs handling unified in `quadlet.AddExtraDirs`.
