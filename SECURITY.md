# Security Policy

> Bahasa Indonesia: [SECURITY.id.md](SECURITY.id.md).

## Supported versions

| Version     | Supported          |
|-------------|--------------------|
| v0.1.0-dev1 | Yes (latest, prerelease) |
| older       | No (please upgrade to the latest release) |

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

- **One capability policy.** Every privileged action declares what it
  needs (state change, quadlet file writes, local file writes, a
  terminal, a secret value, a session switch); `internal/ui/policy.go`
  decides per session kind (local, `--ssh`, `--as`, served, `--system`)
  and `--readonly`. Key handlers and Command Palette entries consult the
  same table, so the two can never disagree and a new action cannot skip
  a gate.
- `quadman serve` defaults to loopback (`127.0.0.1:2222`); any
  non-loopback bind **refuses to start** without `--authorized-keys`.
  Password auth is **loopback-only even next to keys**: a LAN bind
  authenticates with keys alone (`internal/server/server.go`, enforced in
  `Validate`, fail-fast in `main.go`).
- Failed password attempts are throttled per client address (a delay on
  every failure, a lockout after 5 failures within a minute); passwords
  are compared in constant time.
- `quadman serve` with neither `--authorized-keys` nor `--password`
  **forces readonly mode** and prints loud warnings, including that
  loopback is not a user boundary on a shared host; anonymous write
  access is never on by default (`main.go`).
- **Served sessions are isolated from each other.** Every session gets
  its own systemctl/loginctl/podman/podlet clients; nothing a session
  does re-targets another one. Presentation (theme, glyphs) is set once
  per process before the first session. A served session can never
  switch compartments or leave `quadman --as <user> serve`'s compartment
  for the operator's own session, and never runs an editor, a shell,
  a local file export, or a secret-value copy.
- `quadman serve` refuses to start when `config.yaml` is unusable
  (malformed or unknown keys) instead of serving guessed defaults; the
  TUI starts **readonly** in that case. A corrupt `config.json` (editor
  preference) never drops the YAML settings.
- Serve passwords can come from `--password-file`, `QUADMAN_SERVE_PASSWORD`,
  or config `password_file`, so the secret never appears in the process
  list; exactly one password source may be set (`main.go`).
- Host-key directory is created `0700`; `config.json` is written
  atomically with mode `0600` (`internal/server`, `internal/config`).
- A `config.yaml` holding a serve password that is readable beyond the
  owner triggers a `chmod 600` warning at startup (`main.go`).
- Remote execution shells out to the user's own `ssh` with
  `BatchMode=yes` and a connect timeout. OpenSSH re-parses the command
  line on the remote side, so every word is POSIX single-quoted: the
  remote host runs exactly the argv quadman built (the remote login shell
  must be POSIX-compatible). Connection multiplexing uses a private
  `0700` control-socket directory that is removed on exit.
- Sudo compartments use non-interactive `sudo -n -u <user> VAR=value --
  <cmd>`: the session variables travel as sudo's own arguments, so a
  sudoers rule can allow exactly the CLIs quadman runs (with `SETENV`)
  instead of a blanket `/usr/bin/env`. The startup probe uses the same
  form, so a missing `SETENV` fails early.
- Destructive actions confirm what they will do and keep. Deleting a
  unit, removing a volume, removing a secret, and `system prune` need the
  name (or `prune`) typed. Repeating an action from the actions screen
  asks the same confirmation again and is refused after a session switch.
- Custom commands are split into words before template expansion, so a
  value from a unit file can never add or split arguments; they run
  through the session runner, never a shell.
- Quadlet file edits are atomic (temp file, `fsync`, rename, directory
  `fsync`), follow symlinks to replace the real file, and keep its mode
  (and owner when running as root).
- Every `systemctl`/`journalctl` call is bounded (30s default; lifecycle
  jobs wait up to 5 minutes and report a job systemd keeps running as
  pending), podman/SSH calls are bounded (10s default), and a whole
  refresh is bounded (90s) with at most one poll refresh in flight.

## Supply chain

CI enforces on every push/PR to `main`: `go build`, `go vet`,
`go test -race -shuffle=on`, `gofmt` clean, golangci-lint on both
modules, and pinned `govulncheck` and `gosec` scans (pinned so a scanner
release cannot turn CI red by itself; bumped deliberately), plus CodeQL
and dependency review (see `.github/workflows/`).

## Scope

In scope: the quadman CLI, TUI, and `serve` daemon in this repository.
Out of scope: vulnerabilities in Podman, systemd, or the OS itself —
but reports showing how quadman mishandles their output are welcome.
