# quadman Research & Feature Catalog

<p align="left">
  <a href="research-2026-09.en.md"><b>English</b></a> | <a href="research-2026-09.md"><b>Bahasa Indonesia</b></a>
</p>

A compilation of ecosystem research (Podman/Quadlet, peer TUIs, Charm stack) conducted to define the development roadmap of quadman. Compiled on 2026-09-12 from primary sources (official documentation, Podman generator source code, GitHub, and Context7).

---

## 1. Strategic Positioning & Differentiation

| Tool | Approach | Quadlet Native? | Requires Podman Socket? |
|---|---|---|---|
| **quadman** (ours) | TUI, CLI-only (`systemctl`/`journalctl`/`loginctl`) | ✅ Core | ❌ No |
| podman-tui (official `containers` org) | TUI, REST API via Go bindings | ❌ **Not at all** | ✅ Mandatory |
| quadletman | Web UI (HTMX), PAM auth, "compartments" | ✅ | ❌ |
| podman desktop / Portainer / Dockge | Desktop GUI / Compose-only | ❌ / Partial | — |

**Positioning Conclusions:**
1. **The quadman niche is validated and uncontested**: podman-tui does not touch Quadlets at all (its issue tracker has zero Quadlet requests); the only direct competitor (quadletman) is a web UI, in beta, with 57 stars.
2. **"CLI-only, socketless" is a core differentiator worth preserving** — podman-tui *mandates* `systemctl --user start podman.socket`, which many rootless users avoid due to attack surface and complexity. All `podman` CLI commands we need (`stats`, `events`, `df`, `quadlet list`, `auto-update`) run without a daemon or socket because Podman is daemonless.
3. **The "journal = canonical log store" premise is validated**: Quadlets enforce `--log-driver=journald --rm`, making `journalctl` the only true log source. The follow-mode log feature aligns directly with upstream architecture rather than fighting it.

---

## 2. Podman / Quadlet Ecosystem Findings

### 2.1 `podman quadlet` Subcommand Suite (Podman 5.3+) — Key Finding

There is an official command family overlapping with what quadman previously tracked manually:

- `podman quadlet list` — output: name, **systemd unit name**, path, **status** (`Not loaded`, `loaded template`, `active/running`, `inactive/dead`, `failed/failed`, `activating/start`, `deactivating/stop`), application, pod; flags: `--filter name=|pod=|status=`, `--format json`.
- `podman quadlet install [FILE|URL|.quadlets]` — `--application` (app group), `--replace`, **`--reload-systemd` (default true)** = automatic daemon-reload.
- `podman quadlet rm` — `--all/--force/--recursive`, template `@` aware.
- `podman quadlet print` — displays quadlet content including comments.

**Recommendation:** Do not replace internal discovery (ours is faster and zero-dependency), but **use as optional enrichment** — detect `podman` in PATH, merge status from `podman quadlet list --format json`, and cleanly fallback to pure `systemctl show`. "Loaded template" status and application/pod groupings are metadata unavailable from systemctl alone.

### 2.2 Quadlet Directives to Surface in the UI

From `podman-systemd.unit(5)` — high-value operational features:

- **Health**: `HealthCmd=`, `HealthOnFailure=kill` (restart via systemd!), `HealthOnFailure=restart|none`, `HealthStartPeriod=`, `HealthLogDestination=`, `Notify=healthy` (READY state waits for healthcheck pass).
- **Auto-update**: `AutoUpdate=registry|local` per unit; user timer `podman-auto-update.timer` (runs daily, requires linger for unattended execution); `podman auto-update --dry-run --format json` for preview; `--rollback` default on (accurate failure detection requires `Notify=sdnotify`).
- **Drop-in directories**: `foo.container.d/*.conf` alphabetical merge; generic `container.d/` for all units of that type; **cascading dashed prefixes** `foo-.container.d/`, `foo-bar-.container.d/` (specific overrides generic); template resolution reads dual sources (`foo@inst.container.d` + `foo@.container.d`). The UI must treat a unit as: base file + merged drop-ins.
- **`.quadlets` multi-document bundles**: Multiple quadlets in a single file, separated by `---`, with `# FileName=<name>` headers.
- **Dependency graph**: `[Unit]` Wants/Requires/BindsTo/PartOf/Upholds/Conflicts/Before/After automatically translated when referencing other Quadlet units; implicit dependencies via `Image=foo.image|foo.build`, `Network=`, `Volume=`, `Mount=`, `Pod=`, `Artifact=`. → Opportunity for **tree topology** rendering.
- **`[Install]` WantedBy=/RequiredBy=/Alias=** — implies **enable/disable + start-on-boot is a core lifecycle action** (`systemctl --user enable --now <unit>` operates on generator-produced units).
- **`[Quadlet] DefaultDependencies=false`** — disables implicit `network-online.target` dependency.
- **`[Service]` pass-through**: `TimeoutStartSec=` (mitigate slow image pulls), `Restart=always`, `RestartSec=`.

### 2.3 User Pain Points (2014–2026) & Corresponding Features

| Pain Point | Source | quadman Solution |
|---|---|---|
| Edit file → forget `daemon-reload` | All tutorials | Auto-detect file mtime > generator run mtime → warning banner "reload needed?"; prompt on save |
| `systemctl stop` **destroys** container (`--rm`), writable layer lost | podman#28002, discuss#26709 | Confirmation and hint on stop: "container will be removed; state lives in volumes" |
| Slow image pull → `activating (timed-out)` at default 90s | podman docs, discuss#19521 | Detect `timed-out` sub-state → hint `TimeoutStartSec`/`Pull=` |
| Historic container logs missing in `podman logs` | Community | **Follow-mode journal** — journalctl is the true canonical log store |
| `podman system prune` deletes stopped Quadlet containers | toolbox#1005, discuss#26899 | Warning before prune when inactive Quadlet units exist |
| Slow rootless storage (fuse-overlayfs / NFS home) | Mailing list, Fedora discuss | Storage screen: `podman system df --verbose`, surface storage driver |
| Linger & pasta/slirp4netns migration | Arch wiki, Reddit | Linger indicator in status bar + one-key toggle (`L`) |
| Migration from docker-compose | Common | `podlet compose` integration |

### 2.4 podlet (Active under `containers` org, v0.3.2)

Supports: `docker run`/`podman run` (including scripts & URLs) → Quadlet; **compose → per-service .container + automatic dependency wiring** (`--create-pods/networks/volumes`); conversion from live objects (`container/pod inspect`, `kube play`, `network/volume create`, `image build/pull`); `podman artifact pull` → `.artifact`.

**Recommendation:** Shell out to the `podlet` CLI when detected in PATH rather than reimplementing generator logic. UI workflow: paste command / select compose file → preview result → write to config dir → auto `daemon-reload`.

### 2.5 Useful Socketless Podman CLI Commands

- `podman stats --no-stream --format json` — CPU/memory per container.
- `podman events --format json --since 1h` — Live event streaming.
- `podman system df --format --verbose` — Disk usage across images, containers, and volumes.
- `podman healthcheck run <container>` — Exit codes 0/1/125.
- `podman ps -a --format json`, `podman image prune --filter until=24h`.
- `podman kube generate <container|pod>` → YAML generation for `.kube`.
- ⚠️ `podman generate systemd` **deprecated** — never build features on top of it.
- `podman secret create/ls/rm` (rootless) — per-user secret store.

---

## 3. Top TUI UX Patterns (lazydocker, k9s, lazygit, btop, podman-tui)

Ranked patterns by leverage for quadman:

1. **Follow-mode logs** (lazydocker, k9s): Tail + ring buffer. Standard defaults: k9s `tail: 100` / `buffer: 1000`; lazydocker `since: 60m`.
2. **Auto-refresh polling** (k9s `refreshRate` default 2s): Re-enumerate units, **cursor pinned by unit name** (not index) so refreshes do not disrupt selection.
3. **Filter-first `/`** (k9s, lazygit, lazydocker): Regex/fuzzy search across lists.
4. **Destructive operation tiers** (k9s): Typed or explicit confirmation for destructive actions; global **readonly mode**.
5. **Uppercase = Scope Escalation** (lazydocker/lazygit): Lowercase = selected unit, uppercase = global/host scope (e.g. `r` restart vs `R` daemon-reload).
6. **Tabbed detail pane `[`/`]`, dual zone enter/esc** (lazydocker): Source / status / journal / inspect tabs.
7. **`tea.ExecProcess` for $EDITOR**: Clean handover of the terminal and seamless resume.
8. **Contextual Help Overlay** (k9s `?`): Surface only keybindings valid for the current state.
9. **Config YAML + custom commands** (lazydocker): Contextual commands using Go templates (e.g. `systemctl --user status {{ .UnitName }}`).
10. **Mouse off-by-default, full keyboard parity** (k9s `enableMouse: false`): Opt-in mouse support to prevent hijacking terminal text selection.
11. **Colorblind-safe + symbol redundancy**: Never encode status purely by color; pair colors with glyphs (`✓`, `✗`, `⚠`).
12. **OSC52 clipboard support** (k9s): Copy unit names, images, or logs seamlessly over SSH and tmux.
13. **Bulk / marked operations** (k9s space-mark): Select multiple rows with `space` → bulk start/stop/enable.
14. **Health indicator styling**: `long`/`short`/`icon` styles adaptable to environments without nerd fonts.
15. **Recent-actions audit log** (`A`): Record commands executed, timestamps, and outcomes.

---

## 4. Charm Stack v2 Inventory (bubbles v2.2.1, bubbletea v2.0.9, lipgloss v2.0.6)

### 4.1 Bubbles Components Evaluation

| Component | Status in quadman | Opportunity |
|---|---|---|
| `list` | ❌ | Built-in fuzzy filtering (`SetFilteringEnabled(true)`) + pagination + spinner |
| `table` | ✅ Used | Fast rendering; filter must be layered or handled manually |
| `viewport` | ✅ Used | `AtBottom()` + `GotoBottom()` = exact primitives for follow-mode scrolling; `MouseWheelEnabled` |
| `tree` (new in v2) | ✅ Used | Dependency graph: quadlet ↔ unit ↔ pod/volume/network |
| `textinput` | ✅ Used | Command palette (`Ctrl+P`), filter prompts, interactive arguments |
| `textarea` | ❌ | Draft editing (though $EDITOR via ExecProcess is preferred) |
| `key` + `help` | ✅ Used | Context-sensitive footer help via `help.Model` |
| `spinner` | ✅ Used | Animated feedback during restart and daemon-reload operations |
| `progress` | ❌ | Uptime/metric bar visualization |
| `filepicker` | ❌ | Constrained file selector for Quadlet directories |
| `stopwatch`/`timer` | ❌ | Failure duration counter |

### 4.2 Follow-Logs Streaming Pattern

Single in-flight message re-issued on Update (idiomatic, prevents goroutine leaks):

```go
case logLineMsg:
    wasAtBottom := m.viewport.AtBottom()      // Stick to tail only if already at bottom
    m.logs = append(m.logs, msg.line)          // Capped ring buffer (e.g. 1000 lines)
    m.viewport.SetContent(strings.Join(m.logs, "\n"))
    if wasAtBottom || m.following {
        m.viewport.GotoBottom()
    }
    return m, followNextLine(unit)             // Trigger next read
```

### 4.3 `tea.ExecProcess` for $EDITOR Handover

```go
editor := os.Getenv("EDITOR")
if editor == "" {
    editor = "vi"
}
return m, tea.ExecProcess(exec.Command(editor, u.Path), func(err error) tea.Msg {
    return editorFinishedMsg{err}  // Re-read file and offer daemon-reload
})
```

---

## 5. Feature Catalog & Implementation Roadmap

### Tier 1 — Core Differentiators (✅ Shipped)
- [x] **Follow-mode logs**: `journalctl -f -n 200`, 1000-line ring buffer, `AtBottom`/`GotoBottom`, pause toggle `f`.
- [x] **Filter `/`**: Live fuzzy table re-filtering.
- [x] **Edit file via $EDITOR** (`tea.ExecProcess`) → prompt daemon-reload on exit.
- [x] **Auto-update screen**: `AutoUpdate=` per-unit state, `podman auto-update --dry-run` preview, user timer toggle `U`.
- [x] **Health column**: `podman healthcheck run` / status from `podman ps --format json`; shortcut `h`.

### Tier 2 — Operational Depth (✅ Shipped)
- [x] **Drop-in directories**: Parse and merge `foo.container.d/*.conf` + cascading prefixes; effective unit view.
- [x] **`.quadlets` multi-document support**: Parse multi-doc files with `# FileName=` headers.
- [x] **Dependency tree**: Topology view using `bubbles/tree` (pods, containers, images, volumes, networks, `[Unit]` deps).
- [x] **Tabbed detail pane** (`[`/`]`): Source, systemctl status, journal, podman inspect.
- [x] **Storage & events**: `podman system df --verbose` (`g`), live event stream `podman events` (`w`).
- [x] **Smart hints**: Hints for `timed-out` starts (`TimeoutStartSec=`), crash-loops, and missing timers.
- [x] **Generate via podlet** (`n`): Convert `docker run` / compose commands directly into Quadlet files.

### Tier 3 — Infrastructure & Scale (✅ Shipped)
- [x] **SSH remote mode**: Manage remote rootless hosts over SSH (`quadman --ssh user@host`).
- [x] **YAML config + custom commands**: Go-template command runner with keybinding customization.
- [x] **Themes**: Colorblind-safe theme, automatic light/dark detection.
- [x] **Bulk operations**: Multi-row selection with `space` + bulk start/stop/restart.
- [x] **Headless test suite & E2E harness**: PTY test runner and 500-unit benchmark.

### Tier 4 — Enterprise, Multi-Mode & Remote Access (✅ Shipped)
- [x] **Wish v2 SSH Daemon (`quadman serve`)**: Embedded SSH server via Charm Wish v2, public key and password authentication, timeout controls, and read-only monitoring mode.
- [x] **Command Palette (`Ctrl+P`)**: Fuzzy command overlay for lifecycle actions, diagnostics, screen switches, and user commands.
- [x] **Log Export & Live Filtering**: Export logs to timestamped `.log` and `.jsonl` files (`S`), OSC52 copy (`c`), live grep filtering (`F`), and priority filters (`p`).
- [x] **Native Rootful / System Mode (`--system`)**: First-class support for `/etc/containers/systemd`, auto-detection for root (UID 0 / `sudo quadman`), dynamic CLI flag management, and linger protection.

---

## 6. Primary References

- podman-tui: <https://github.com/containers/podman-tui>
- quadletman: <https://github.com/mikkovihonen/quadletman>
- podlet: <https://github.com/containers/podlet> (v0.3.2)
- podman-systemd.unit(5): <https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html>
- podman-auto-update(1): <https://docs.podman.io/en/latest/markdown/podman-auto-update.1.html>
- Upstream Podman quadlet generator: `pkg/systemd/quadlet/{quadlet,unitdirs}.go`
- lazydocker: <https://github.com/jesseduffield/lazydocker>
- k9s: <https://github.com/derailed/k9s>
- lazygit: <https://github.com/jesseduffield/lazygit>
- btop: <https://github.com/aristocratos/btop>
- Charm stack v2: bubbletea v2.0.9, bubbles v2.2.1, lipgloss v2.0.6, wish v2.0.4
