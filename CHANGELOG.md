# Changelog

> Bahasa Indonesia: [CHANGELOG.id.md](CHANGELOG.id.md).

All notable changes to quadman are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
