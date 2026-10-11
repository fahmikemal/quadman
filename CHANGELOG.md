# Changelog

> Bahasa Indonesia: [CHANGELOG.id.md](CHANGELOG.id.md).

All notable changes to quadman are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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

### Added

- UPTIME, from systemd's `ActiveEnterTimestamp` (read with
  `systemctl show --timestamp=unix`).
- systemd status now carries `Result`, `ExecMainStatus` and `NRestarts`.

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

## [v0.1.0-dev1] - 2026-10-05

Initial prerelease.

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
