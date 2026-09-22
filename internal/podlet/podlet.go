// Package podlet wraps the optional podlet CLI for generating Quadlet files
// from docker/podman run commands and compose files.
package podlet

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// DefaultTimeout bounds podlet calls (it is a local converter, no network).
const DefaultTimeout = 15 * time.Second

// DefaultRunner executes the podlet CLI. It is local by default; the UI sets
// it to an SSH runner in --ssh mode.
var DefaultRunner remote.Runner

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
	out, err := DefaultRunner.Command(ctx, "podlet", args...).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podlet: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Compose converts a compose file into Quadlet text (one per service).
func Compose(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := DefaultRunner.Command(ctx, "podlet", "compose", "-f", path).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podlet compose: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// objectKinds are the `podlet generate` subcommands quadman accepts from
// the generate prompt: "<kind> <name>", e.g. "container web".
var objectKinds = map[string]bool{
	"container": true,
	"pod":       true,
	"network":   true,
	"volume":    true,
	"image":     true,
}

// IsObjectKind reports whether kind is a convertible live-object type.
func IsObjectKind(kind string) bool { return objectKinds[kind] }

// GenerateObject converts a live podman object into Quadlet text via
// `podlet generate <kind> <name>`. Name must be a single word: the UI
// rejects shell metacharacters before calling, and argv never touches a
// shell here either.
func GenerateObject(ctx context.Context, kind, name string) (string, error) {
	if !IsObjectKind(kind) {
		return "", fmt.Errorf("podlet cannot generate from %q: want container|pod|network|volume|image", kind)
	}
	if name == "" || strings.ContainsAny(name, " \t\n;|&$`(){}<>!*?~#") {
		return "", fmt.Errorf("invalid object name %q: single name without shell characters", name)
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	out, err := DefaultRunner.Command(ctx, "podlet", "generate", kind, name).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podlet generate %s %s: %w: %s", kind, name, err, strings.TrimSpace(string(out)))
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
