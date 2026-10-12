# quadman design system (v0.2)

This is the design contract the TUI implements. Code comments refer to its
sections ("DESIGN.md principle 1", "layout 2c", "section 9"); when the two
disagree, fix one of them in the same change.

## 1. Principles

1. **Never color alone.** Every state is a glyph *and* a word
   (`● active`, `✗ failed`, `▲ unhealthy`). Color only reinforces it, so the
   UI reads the same in the colorblind theme, in ASCII mode, and in a
   screenshot pasted into a ticket.
2. **The terminal stays in charge.** The main background is never painted.
   Only surfaces — top bar, status line, tiles, modals — and the selected
   row get a fill, so the user's own background, font, and transparency
   stay intact.
3. **Everything is one keypress away, nothing is in the way.** The footer is
   capped at two rows and always ends with `^P all commands  ? help`;
   whatever does not fit lives behind those two keys.
4. **Problems show the next action.** A problem is never only a red mark: it
   names what is wrong and the key that fixes it (`not-found — press R`).
5. **Say what will happen, and what stays.** Every state change that is not
   trivially undoable goes through a confirmation that lists both
   (section 7).
6. **Refusals explain.** When the session may not do something (readonly,
   SSH, a compartment, a served session), the status line says why and what
   to do instead. One capability policy (`internal/ui/policy.go`) decides
   for the keys and the Command Palette alike, so the two never disagree.

## 2. Layout

- **2a Top bar** — `quadman › scope › target` (`rootless` / `system`, then
  the SSH host, `as <user>`, or the served client), plus refresh and podman
  version chips on the right. The refresh chip shows the cadence in force:
  `15s idle` after a minute without input or while the terminal is
  unfocused; the next key, click, or focus refreshes at once.
- **2b Nav strip** — every view with its key and a badge count (problems,
  pending updates). Nav keys switch views from any screen.
- **2c Dashboard** (unit list only) — six tiles: units, running, failed,
  generator, linger, auto-update; then a "needs attention" block of at most
  four problems with the keys that fix them (principle 4). Below 100 columns
  or 30 rows it collapses to one summary line; `z` toggles it, and
  `dashboard: false` starts it collapsed.
- **2d Table** — problems sort first (failed → unhealthy → starting →
  not-found → active → inactive); `S` cycles to name or kind order. From 160
  columns an overview pane for the selected row sits on the right.
- **2e Status line and footer** — the status line carries the result of the
  last action and the session chips (linger, readonly, system, list
  position); the footer groups keys as UNIT · INSPECT · VIEWS · HOST (plus
  SELECT from 160 columns).

## 3. Palette

Thirteen tokens per theme. `surface` and `selBg` are the only backgrounds;
`onAccent` is text drawn on an accent fill.

| Token     | dark      | light     | colorblind |
|-----------|-----------|-----------|------------|
| surface   | `#161B24` | `#E9ECF0` | `#161B24`  |
| selBg     | `#1C2838` | `#D9E6F3` | `#1C2838`  |
| border    | `#262E3B` | `#CFD5DD` | `#262E3B`  |
| dim       | `#5D6878` | `#6B7584` | `#5D6878`  |
| muted     | `#8B95A5` | `#4F5867` | `#8B95A5`  |
| fg        | `#C9D1DB` | `#272D35` | `#C9D1DB`  |
| bright    | `#EEF2F6` | `#0E1117` | `#EEF2F6`  |
| accent    | `#52CFF3` | `#0074A6` | `#52CFF3`  |
| onAccent  | `#0E1117` | `#FFFFFF` | `#0E1117`  |
| ok        | `#6ED889` | `#0A7E3A` | `#609EFA`  |
| warn      | `#F3BD5C` | `#AA6A00` | `#F2E15F`  |
| err       | `#F97770` | `#C53637` | `#F88A3D`  |
| kind      | `#B6ADF3` | `#6451AC` | `#B6ADF3`  |

The colorblind theme avoids the red–green pair: ok is blue, warn yellow,
err orange. Themes are process-wide: `quadman serve` sets one theme for all
sessions before the first connects, and resolves `auto` to dark because a
daemon has no terminal of its own to probe.

## 4. Glyphs

| Meaning                    | Unicode | ASCII     | Color   |
|----------------------------|---------|-----------|---------|
| active                     | `●`     | `*`       | ok      |
| inactive                   | `○`     | `o`       | muted   |
| starting / stopping        | `◐`     | `~`       | accent  |
| failed                     | `✗`     | `x`       | err     |
| unhealthy                  | `▲`     | `!`       | warn    |
| not-found (needs reload)   | `◌`     | `?`       | dim     |
| template                   | `◇`     | `<>`      | muted   |
| warning                    | `⚠`     | `!`       | warn    |
| outside the generator path | `~`     | `~`       | dim     |
| marked for a bulk action   | `✱`     | `+`       | accent  |
| action done                | `✓`     | `ok`      | muted   |
| refresh interval           | `⟳`     | `refresh` | muted   |

ASCII glyphs turn on with `--ascii` and automatically on `TERM=linux`.

## 5. Columns

The unit table is STATE · QUADLET · KIND · SUB · UPTIME · IMAGE. STATE is 13
cells (fits `▲ unhealthy`), KIND 11, SUB 9, UPTIME 10; QUADLET clamps between
14 and 34; IMAGE takes what remains (at least 8). Below 100 columns UPTIME
and IMAGE are hidden. The systemd unit name lives in the detail views.

## 6. Responsive

| Width × height     | Behavior                                              |
|--------------------|-------------------------------------------------------|
| below 60 × 16      | a "terminal too small" notice with the compact logo   |
| below 100 columns  | compact chrome, unlabeled UNIT keys, palette as a full-width sheet, detail panels full screen on `↵` |
| 100 columns and up | full chrome, UPTIME and IMAGE columns                 |
| 160 columns and up | overview side pane (58 columns), SELECT footer group  |

## 7. Confirmations

- A confirmation is a modal over the screen it came from, with a **THIS
  WILL** list (what happens) and a **KEEPS** list (what stays untouched).
- Focus starts on **Cancel**; `←/→`, `tab`, `h`/`l` move it; any key that is
  not a confirm declines, like the old `y/N` prompt. `y` still confirms
  plain dialogs.
- Bulk stop and restart use a warn border. Data loss uses an err border
  and stays disabled until the exact word is typed: the unit name for
  delete, the volume name for volume removal, the secret name for secret
  removal, `prune` for `system prune`.
- Repeating an action from the actions screen (`A`, then `.`) asks the same
  confirmation the action first passed — a typed name stays a typed name —
  and is refused after a session switch.

## 8. Feedback

- Success messages fade after five seconds; errors stay until replaced.
- Every action records its trigger, duration, and result in the actions
  screen (`A`).
- A lifecycle job that systemd keeps running past quadman's wait is reported
  calmly as pending (not as a failure); the next refresh shows the outcome.
- Refusals follow principle 6: what was refused, why, and what works
  instead.

## 9. Secondary screens

Every screen beyond the unit list shares one skeleton (`screen.go`): nav
strip → summary tiles → optional sub-tabs and toolbar → a table with group
headings and problems first → a detail panel for the selected row (full
screen on `↵` below 100 columns) → its own footer group. A screen only
describes itself (`screenSpec`); layout, scrolling, `/` filtering, and
navigation behave the same everywhere.

Screens whose data comes from a command (updates, storage, timers,
secrets, stats) open on the keypress, never after the data lands: a
revisit shows the last data while it reloads, a first visit shows the
skeleton with `…` values and "loading…". Until data exists their own keys
do nothing but say why. A failed first load stays on the screen and names
the error and the retry key. A result only updates its own screen: one that
lands after the user left is kept for the next visit, never used to switch
back. A session switch drops every screen's data and returns to the new
target's unit list.

Reopening a screen within its freshness window — 30 s, five minutes for
updates (a dry run asks every image's registry), none for live stats —
shows its data as is and says how old it is; `r` (secrets: `^R`) always
reloads. Leaving a screen cancels its load, killing the commands still
running, so quick back-and-forth never stacks podman processes. A loader
runs its independent reads side by side, and the stats sampler runs one
sample at a time, reading host info once and container labels every few
samples.

## 10. Accessibility

- Colorblind theme (section 3) and glyph + word states (principle 1).
- ASCII fallback for consoles without the geometric-shapes block.
- Mouse is opt-in (`--mouse`, `mouse: true`) so text selection keeps
  working by default.
- Clipboard copies use OSC52, which works over SSH; copied secret values are
  cleared from the clipboard after 30 seconds and never rendered.
