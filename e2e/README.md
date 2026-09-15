# e2e — PTY end-to-end test for the quadman TUI

Drives the real TUI in a pseudo-terminal: sends actual keystrokes and
asserts what the screen renders, covering discovery, filter, start/stop,
healthcheck, follow logs, enable/disable at boot, the auto-update screen,
the stale banner, daemon-reload, linger toggling, help, and file view.

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
