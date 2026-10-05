# quadman Roadmap

> Back to [README](README.md).

Ships in batches (see [CONTRIBUTING.md](CONTRIBUTING.md)); minor bumps for
feature tiers, patch bumps for accumulated fixes.

### Tier 1: differentiators (✅ shipped)

- [x] Follow mode for journals (`journalctl -f` streaming, stick-to-tail)
- [x] `/` fuzzy filter over the unit list
- [x] Edit Quadlet files in `$EDITOR` (`tea.ExecProcess`) → offer `daemon-reload` on exit
- [x] Auto-update screen: per-unit `AutoUpdate=` state, `podman auto-update --dry-run`
      preview, user `podman-auto-update.timer` status/toggle
- [x] Health column + `h` runs `podman healthcheck run`
- [x] Stop confirmation — Quadlet runs containers `--rm`, stop deletes them
- [x] Surface new Podman 6.1 keys (e.g. `ImageVolume=`) in parsing
- [x] Tests for the non-interactive `list` command

### Tier 2: operational depth (✅ shipped)

- [x] Built-in Quadlet validation (generator dry-run, ✗/⚠ markers, problems view, version gating)
- [x] Manage drop-in directories (`*.container.d/*.conf`, cascading `foo-.container.d/`)
- [x] Dependency tree view (bubbles `tree`): pod↔container, implicit `Image=`/`Network=`/`Volume=` deps, `[Unit]` deps
- [x] Mouse (click select, wheel) + OSC52 clipboard (`y`/`Y`)
- [x] Template instantiation (`web@.container` → `web@prod.service`)
- [x] Delete units (`podman quadlet rm --force` with fallback)
- [x] Multi-document `.quadlets` files (`# FileName=` headers) — discover, preview, install via `podman quadlet install` (`I`)
- [x] Tabbed detail pane (`[`/`]`): source / status / journal / `podman inspect`
- [x] Storage & events screens (`g` = `podman system df --verbose`, `w` = streaming `podman events`)
- [x] Smart hints: `timed-out` → `TimeoutStartSec=`/`Pull=`, start-limit crash-loop, `AutoUpdate=` without the timer enabled
- [x] Generate Quadlet files via podlet (`n`): `docker run` / compose → preview → `y` write → reload

### Tier 3: scale (✅ shipped)

- [x] SSH mode for remote rootless hosts (`--ssh user@host`; list, lifecycle,
      logs, and screens over SSH — file edits stay local to the host)
- [x] YAML config (refresh rate, log tail/buffer, theme, mouse) + Go-template custom commands
- [x] Colorblind-safe theme + auto light/dark detection + opt-in mouse
- [x] Bulk mark + mass actions (`space` marks, `s`/`x`/`r` act on all marked)
- [x] `--readonly` mode
- [x] Recent-actions log (`A`: what ran, when, and whether it worked)
- [x] Headless Update/View test suite + 500-unit benchmark + VHS demo tape
      (`teatest` itself is not shipped in bubbletea v2.0.9, so the suite drives
      the Model directly instead)

### Tier 4: Enterprise, Multi-Mode & Remote Access (✅ shipped)

- [x] **Wish v2 SSH Daemon (`quadman serve`)** — embedded SSH server via Charm Wish v2, connect directly with `ssh -p 2222 host`, host key auto-generation, public key auth (`authorized_keys`), password auth, read-only mode, and session timeout management.
- [x] **Command Palette (`Ctrl+P`)** — fuzzy-searchable modal overlay for immediate discovery & execution of all unit lifecycle verbs, diagnostic tools, screen switching, and custom commands with context-sensitive state validation.
- [x] **Log Export & Live Filtering** — export journal logs to timestamped `.log` (plain text) and `.jsonl` (structured journal format) files (`S`), OSC52 clipboard copy (`c`), live grep filtering (`F`), and log priority cycling (`p`: err, warning, info, all).
- [x] **Native Rootful / System Mode (`--system`)** — first-class support for system-wide Quadlet units (`/etc/containers/systemd`, `/run/containers/systemd`, `/usr/share/containers/systemd`), auto-detection when executed as root (UID 0 / `sudo quadman`), dynamic `--user` CLI switching, and system linger safeguards.

### Tier 5: Secrets, Timers & Agent Tooling (✅ shipped)

- [x] **Podman Secrets Integration** — inspect Podman secrets store (`K`), and automatic pre-flight reference validation for `Secret=` directives in `.container` files with warnings in the Problems (`v`) view.
- [x] **Standalone Systemd Timers View (`T`)** — inspect all active and scheduled calendar timers (`systemctl list-timers`), execution triggers, and countdowns directly in the TUI.
- [x] **AI Agent Skill Export (`quadman --skill`)** — export machine-readable JSON tool schemas and comprehensive Markdown documentation for LLM coding agents.

### Tier 6: Operate, Harden & Compartments (✅ shipped)

- [x] **Exec shell (`X`)** — open `/bin/sh` in the unit's container (exact name match, `systemd-<name>` prefill), refused over SSH/served sessions with an explanation.
- [x] **Resource stats (`o`)** — `podman stats --no-stream --all` table (CPU/MEM/NET/BLOCK/PIDS), read-only and remote-safe.
- [x] **System prune (`P`)** — `podman system prune -f` behind a confirmation naming inactive Quadlet units at risk.
- [x] **Generate from live objects** — `n` accepts `container|pod|network|volume|image <name>` via `podlet generate`, plus **Pull Unit Image** palette action with an already-present fast-path.
- [x] **Problems depth** — missing `[Kube] Yaml=` detection (local + remote), `AutoUpdate=local` and `.kube` timer hints, `Upholds=`/`Conflicts=` in the dependency tree.
- [x] **Serve hardening** — open access (no keys/password) forces readonly + loud warnings; constant-time password compare; YAML password world-readable warning; log export never clobbers (`-1`, `-2`…).
- [x] **Compartments (`--as <user>`, `quadman --as <user> list`)** — manage another OS user's Quadlets through non-interactive sudo with that user's own search path, systemd instance, storage, and secrets; palette switcher, `[user]` title chip, isolated-session guards throughout.
- [x] **Custom-key guard** — startup warning when a `custom_commands` key is shadowed by a built-in binding (built-ins always win).

### Serve hardening batch (✅ shipped)

- [x] **Serve bind policy**: loopback-only default (`127.0.0.1:2222`); non-loopback binds refuse to start without `--authorized-keys`, so open-auth and password-only stay loopback-only; open-auth still forces readonly everywhere; a missing or empty keys file fails loudly at startup; an existing host key is tightened to `0600`.
- [x] **CLI guards**: misplaced flags after `list` rejected; Makefile `e2e` target fixed (binary path plus phony).
- [x] **Skill fixes**: self-documenting export entries, single-v version header, `--system list` form, serve bind policy description.
- [x] **Charm stack refresh**: bubbletea v2.0.9 → v2.0.10; `govulncheck` and `gosec` green.

### v0.1.0-dev1: serve passwords, serve --as & guards (✅ shipped)

- [x] **Serve password sources**: `--password-file`, `QUADMAN_SERVE_PASSWORD`, config `password_file` (exactly one); the secret never appears in argv.
- [x] **serve --as**: serve another user's session through a sudo compartment with startup validation; isolated-session guards still apply.
- [x] **Flag guard coverage**: `version` and `skill` reject trailing flags (`skill` keeps its own `--format`/`--json`).
- [x] **Remote drop-ins (read-only)**: enumerated through the session runner for SSH, compartment, and served sessions.
- [x] **Mode guard**: `--system` and `--as` rejected together (CLI plus palette), since system units live outside any user's session.

### Notable ecosystem notes (Sep 2026)

- [podman-tui](https://github.com/containers/podman-tui) v2.0.0 (Sep 6, 2026)
  supports Podman 6 — and **still has no Quadlet management**, keeping this niche open.
- podlet v0.3.2 (May 2026) covers Podman ≤ 5.8; Podman 6 keys like
  `ImageVolume=` are not yet generated by it.
- `.artifact` units are no longer flagged experimental in the current docs;
  quadman already names them correctly (`foo.artifact` → `foo-artifact.service`).
