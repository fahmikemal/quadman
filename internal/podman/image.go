package podman

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// PullTimeout bounds `podman pull`: cold registries routinely exceed a
// minute, and a killed pull leaves a half-fetched image behind.
const PullTimeout = 10 * time.Minute

// ImageExists reports whether image is in local storage, using
// `podman image exists` (exit 0 = present, exit 1 = absent). Any other
// failure (no daemon, broken storage) is returned as an error so callers
// never mistake "podman is broken" for "image is missing".
func ImageExists(ctx context.Context, image string) (bool, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return false, fmt.Errorf("empty image reference")
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()
	err := runCmd(ctx, "podman", "image", "exists", image).Run() // #nosec G204 -- argv slice, no shell
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("podman image exists %s: %w", image, err)
}

// PullImage runs `podman pull` and returns its output. Long timeout, and
// the UI gates it behind readonly checks; the image reference comes from
// the unit file via argv, never a shell.
func PullImage(ctx context.Context, image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", fmt.Errorf("empty image reference")
	}
	ctx, cancel := context.WithTimeout(ctx, PullTimeout)
	defer cancel()
	out, err := runCmd(ctx, "podman", "pull", image).CombinedOutput() // #nosec G204 -- argv slice, no shell
	if err != nil {
		return "", fmt.Errorf("podman pull %s: %w: %s", image, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// IsLocalImage reports whether image looks locally built (localhost/
// prefix): such references are produced by .build units, so "missing from
// storage" is a build problem, not a pull problem, and the UI skips the
// pull hint for them.
func IsLocalImage(image string) bool {
	return strings.HasPrefix(image, "localhost/")
}
