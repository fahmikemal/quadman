// Package loginctl wraps loginctl for user-linger management. Linger is what
// keeps rootless containers running after the user logs out — every rootless
// Quadlet host needs it enabled once.
package loginctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// DefaultTimeout bounds each loginctl call so a hung binary cannot stall a
// refresh forever.
const DefaultTimeout = 30 * time.Second

// Loginctl talks to loginctl through its CLI.
type Loginctl struct {
	// Bin overrides the loginctl binary path.
	Bin string
	// Timeout bounds each call; 0 means DefaultTimeout.
	Timeout time.Duration
}

// New returns a Loginctl using the loginctl binary from PATH.
func New() *Loginctl { return &Loginctl{Bin: "loginctl"} }

func (l *Loginctl) bin() string {
	if l.Bin != "" {
		return l.Bin
	}
	return "loginctl"
}

func (l *Loginctl) timeoutCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	t := l.Timeout
	if t <= 0 {
		t = DefaultTimeout
	}
	return context.WithTimeout(ctx, t)
}

// CurrentUser returns the invoking user's account name.
func CurrentUser() (string, error) {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username, nil
	}
	if u := os.Getenv("USER"); u != "" {
		return u, nil
	}
	return "", fmt.Errorf("cannot determine current user")
}

// Enabled reports whether linger is on for userName (empty = current user).
func (l *Loginctl) Enabled(ctx context.Context, userName string) (bool, error) {
	userName, err := resolve(userName)
	if err != nil {
		return false, err
	}
	ctx, cancel := l.timeoutCtx(ctx)
	defer cancel()
	out, err := exec.CommandContext(ctx, l.bin(), // #nosec G204 -- argv slice, no shell; user name is behind a "--" separator
		"show-user", "--property=Linger", "--value", "--", userName).Output()
	if err != nil {
		return false, fmt.Errorf("loginctl show-user %s: %w", userName, err)
	}
	return strings.TrimSpace(string(out)) == "yes", nil
}

// Set turns linger on or off for userName (empty = current user).
func (l *Loginctl) Set(ctx context.Context, userName string, on bool) error {
	userName, err := resolve(userName)
	if err != nil {
		return err
	}
	verb := "disable-linger"
	if on {
		verb = "enable-linger"
	}
	ctx, cancel := l.timeoutCtx(ctx)
	defer cancel()
	out, err := exec.CommandContext(ctx, l.bin(), verb, "--", userName).CombinedOutput() // #nosec G204 -- argv slice, no shell; user name is behind a "--" separator
	if err != nil {
		return fmt.Errorf("loginctl %s: %w: %s", verb, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resolve(userName string) (string, error) {
	if userName != "" {
		return userName, nil
	}
	return CurrentUser()
}
