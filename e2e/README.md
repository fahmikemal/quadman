# e2e — PTY end-to-end test for the quadman TUI

<p align="left">
  <a href="README.md"><b>English</b></a> | <a href="README.id.md"><b>Bahasa Indonesia</b></a>
</p>

Drives the real TUI in a pseudo-terminal: sends actual keystrokes and
asserts what the screen renders, covering discovery, filter, start/stop,
healthcheck, follow logs, enable/disable at boot, the auto-update screen,
the stale banner, daemon-reload, linger toggling, help, file view,
systemd timers schedules, podman secrets inspection and pre-flight validation,
and AI agent skill export.

This module is intentionally separate (its own go.mod) so the main module
stays free of test-only dependencies, and CI does not run it — it needs a
live systemd user session, podman, and quadlet units on the host.

## Run

```sh
# from the repo root, with demo units present (see below)
go build -o /tmp/qe2e ./e2e
make build
cp quadman /tmp/quadman-under-test
cd /tmp && /tmp/qe2e
```

The harness executes `./quadman` from its working directory, so run it
from a directory containing the freshly built binary (or adjust the path
in `main.go`).

## Fixture

The test expects three units in `~/.config/containers/systemd/`:
`demo-web.container` (busybox with a healthcheck and `AutoUpdate=registry`),
`demo-data.volume`, and `demo-net.network`. It mutates them (start/stop,
[Install] add/remove) and restores boot state at the end; linger is
toggled twice and restored.

### Compartment fixture (optional)

`scenarioCompartmentLive` needs a second local user with passwordless sudo
and skips gracefully without it:

```sh
sudo useradd -m svc-test
sudo loginctl enable-linger svc-test
cat <<'SUDO' | sudo tee /etc/sudoers.d/quadman-svc-test >/dev/null
tesseract ALL=(svc-test) NOPASSWD:SETENV: /usr/bin/env, /usr/bin/systemctl, /usr/bin/journalctl, /usr/bin/loginctl, /usr/bin/podman, /usr/bin/podlet, /usr/bin/cat, /usr/bin/true
SUDO
sudo chmod 440 /etc/sudoers.d/quadman-svc-test
sudo -u svc-test mkdir -p /home/svc-test/.config/containers/systemd
printf '[Container]\nImage=docker.io/library/busybox:latest\nExec=sleep infinity\n' | sudo -u svc-test tee /home/svc-test/.config/containers/systemd/e2e-comp.container >/dev/null
```

(Replace `tesseract` with the operator username. `sleep infinity` keeps the
fixture runnable for exec tests; the scenario itself only asserts listing.)

Set `QE2E_ONLY=<substr>` to run only matching extra scenarios (the main flow always runs).
