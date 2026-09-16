// Package podlet wraps the optional podlet CLI for generating Quadlet files
// from docker/podman run commands and compose files.
package podlet

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout bounds podlet calls (it is a local converter, no network).
const DefaultTimeout = 15 * time.Second

// Available reports whether a podlet binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("podlet")
	return err == nil
}

// Generate converts a docker/podman run command into Quadlet text.
// args are the words after "podlet", e.g. ["podman", "run", "--name", "web", "nginx"].
func Generate(ctx context.Context, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "podlet", args...).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podlet: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Compose converts a compose file into Quadlet text (one per service).
func Compose(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "podlet", "compose", "-f", path).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podlet compose: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// FileNameOf extracts the `# FileName=<name>` header podlet emits, or "".
func FileNameOf(generated string) string {
	for _, line := range strings.Split(generated, "\n") {
		t := strings.TrimSpace(line)
		if name, ok := strings.CutPrefix(t, "# FileName="); ok {
			return strings.TrimSpace(name)
		}
		if t != "" && !strings.HasPrefix(t, "#") {
			break
		}
	}
	return ""
}
