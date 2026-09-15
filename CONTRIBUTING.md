# Contributing

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
- Commit prefixes `docs:` / `test:` / `ci:` / `chore:` are excluded from the
  generated changelog (see `.goreleaser.yaml`).
- New behavior should come with tests; wrappers around CLIs are tested with
  fake binaries (see `internal/systemd/systemd_test.go` for the pattern).
- Regenerate README screenshots after UI changes:
  `QUADMAN_SCREENSHOTS=1 go test ./internal/ui -run Screenshots`
