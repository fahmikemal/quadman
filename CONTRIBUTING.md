# Contributing

<p align="left">
  <a href="CONTRIBUTING.md"><b>English</b></a> | <a href="CONTRIBUTING.id.md"><b>Bahasa Indonesia</b></a>
</p>

## Release policy

quadman batches changes into meaningful releases — no version bump for every
small commit.

- **Feature batch** (new capability, e.g. follow-mode logs, edit flows):
  minor bump (`v0.N+1.0`).
- **Bugfix batch** (several accumulated fixes): patch bump (`v0.N.M+1`).
- **docs / test / chore / cosmetic commits**: pushed to `main`, **no tag**.

In practice: work on `main`, collect changes, and when the batch feels like
something a user would notice, tag `vX.Y.Z` — the release workflow
(`.github/workflows/release.yml` + goreleaser) publishes binaries and
checksums automatically.

## House rules

- `make fmt vet test` must stay green before every push (CI enforces it).
- Keep the security gates green too, at the versions CI pins
  (`.github/workflows/ci.yml`): `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...`
  and `go run github.com/securego/gosec/v2/cmd/gosec@v2.29.0 -quiet ./...`. Bump
  them deliberately in their own commit.
- Tests must not depend on the host: set `TERM`/glyph mode, the runtime dir,
  and binaries (fakes on `PATH`, `SSHBin`) explicitly, and run
  `go test -race -shuffle=on ./...` before pushing.
- Every new state-changing or privileged action declares its capabilities
  through `m.refuse(...)` (keys) and `m.allowed(...)` (palette) from
  `internal/ui/policy.go`; never test `m.readonly` or the session kind by
  hand. Destructive actions confirm with THIS WILL / KEEPS, and data loss
  needs the name typed (see `DESIGN.md`).
- Never add package-level mutable state reachable from a session: served
  sessions run concurrently in one process. Session-bound clients live on
  the model (`setSession`); process-wide presentation goes through
  `ui.Configure`, once, before any session starts.
- Commit prefixes `docs:` / `test:` / `ci:` / `chore:` are excluded from the
  generated changelog (see `.goreleaser.yaml`).
- New behavior should come with tests; wrappers around CLIs are tested with
  fake binaries (see `internal/systemd/systemd_test.go` for the pattern).
- Refresh README screenshots after UI changes: capture the list, journal,
  and stats views manually into `docs/screenshot-{list,logs,stats}.png`.
