# Security Policy

> Bahasa Indonesia: [SECURITY.id.md](SECURITY.id.md).

## Supported versions

| Version | Supported          |
|---------|--------------------|
| 0.5.x   | Yes (latest: 0.5.0) |
| < 0.5.0 | Best effort — please upgrade and re-test first |

Security fixes ship as patch releases on `main` and are published through
GitHub Releases with checksums (see `.github/workflows/release.yml`).

## Reporting a vulnerability

Please **do not** open a public issue with exploit details. Instead use
GitHub's private channel for this repository: **Security → Report a
vulnerability** (creates a private Security Advisory).

Include:

- quadman version (`quadman -version`) and install source (release tarball, `go install`, built from source),
- operating mode (`--system`, `--ssh`, `--as`, `serve`, or local TUI),
- steps to reproduce and the impact you see,
- anything you already ruled out.

Coordinated disclosure is appreciated: give the maintainers a chance to
ship a fix before publishing details.

## Hardening defaults (shipped)

These are implemented in the tree, not just advised:

- `quadman serve` defaults to loopback (`127.0.0.1:2222`); any
  non-loopback bind **refuses to start** without `--authorized-keys`, so
  open-auth and password-only binds are loopback-only
  (`internal/server/server.go`, enforced in `Validate`, fail-fast in `main.go`).
- `quadman serve` with neither `--authorized-keys` nor `--password`
  **forces readonly mode** and prints loud warnings; anonymous write
  access is never on by default (`main.go`).
- Served-session passwords use constant-time comparison
  (`internal/server/server.go`).
- Serve passwords can come from `--password-file`, `QUADMAN_SERVE_PASSWORD`,
  or config `password_file`, so the secret never appears in the process
  list; exactly one password source may be set (`main.go`).
- Host-key directory is created `0700`; `config.json` is written `0600`
  (`internal/server`, `internal/config`).
- A `config.yaml` holding a serve password that is readable beyond the
  owner triggers a `chmod 600` warning at startup (`main.go`).
- Remote execution shells out to the user's own `ssh` with
  `BatchMode=yes` and a connect timeout, fully argv-based (no remote
  shell string); sudo compartments use non-interactive `sudo -n`
  (`internal/remote`).
- Every `systemctl`/`journalctl` call is bounded (30s default),
  podman/SSH calls are bounded (10s default).
- Listening on all interfaces requires `--authorized-keys` (refused
  otherwise) and with write access still prints a warning; prefer
  `-a 127.0.0.1:2222` or `--readonly`.
- Isolated sessions (SSH target, `--as` compartment, served sessions)
  refuse local file writes, nested sudo hops, and `$EDITOR` process
  hijacking, with an explanation in the UI.

## Supply chain

CI enforces on every push/PR to `main`: `go build`, `go vet`,
`go test -race`, `gofmt` clean, `govulncheck`, `gosec`, plus weekly
CodeQL and dependency review (see `.github/workflows/`).

## Scope

In scope: the quadman CLI, TUI, and `serve` daemon in this repository.
Out of scope: vulnerabilities in Podman, systemd, or the OS itself —
but reports showing how quadman mishandles their output are welcome.
