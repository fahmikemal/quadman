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
	"sync"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
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
	// Remote runs the CLI over SSH or as a sudo compartment user. The
	// zero value runs everything locally.
	Remote remote.Runner
	// User pins the account an empty user name means. Unset, it is the
	// session's own user: the sudo target, the SSH login user, or the
	// local user — never the operator's local name sent to another host.
	User string

	mu   sync.Mutex
	self string // cached remote login name (SSH)
}

// run executes the loginctl CLI (locally or over SSH) like
// exec.CommandContext.
func (l *Loginctl) run(ctx context.Context, name string, args ...string) *exec.Cmd {
	return l.Remote.Command(ctx, name, args...)
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

// Enabled reports whether linger is on for userName (empty = the
// session's own user).
func (l *Loginctl) Enabled(ctx context.Context, userName string) (bool, error) {
	userName, err := l.resolve(ctx, userName)
	if err != nil {
		return false, err
	}
	ctx, cancel := l.timeoutCtx(ctx)
	defer cancel()
	out, err := l.run(ctx, l.bin(), // #nosec G204 -- argv slice, no shell; user name is behind a "--" separator
		"show-user", "--property=Linger", "--value", "--", userName).Output()
	if err != nil {
		return false, fmt.Errorf("loginctl show-user %s: %w", userName, err)
	}
	return strings.TrimSpace(string(out)) == "yes", nil
}

// Set turns linger on or off for userName (empty = the session's own
// user).
func (l *Loginctl) Set(ctx context.Context, userName string, on bool) error {
	userName, err := l.resolve(ctx, userName)
	if err != nil {
		return err
	}
	verb := "disable-linger"
	if on {
		verb = "enable-linger"
	}
	ctx, cancel := l.timeoutCtx(ctx)
	defer cancel()
	out, err := l.run(ctx, l.bin(), verb, "--", userName).CombinedOutput() // #nosec G204 -- argv slice, no shell; user name is behind a "--" separator
	if err != nil {
		return fmt.Errorf("loginctl %s: %w: %s", verb, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// resolve maps an empty user name to the session's own account. Over SSH
// the login name on the remote host is asked once (`id -un`) and cached:
// the local operator name may not exist there, or belong to someone else.
func (l *Loginctl) resolve(ctx context.Context, userName string) (string, error) {
	switch {
	case userName != "":
		return userName, nil
	case l.User != "":
		return l.User, nil
	case l.Remote.IsRemote(): // Target wins over As, like the runner
		return l.remoteSelf(ctx)
	case l.Remote.As != "":
		return l.Remote.As, nil
	}
	return CurrentUser()
}

func (l *Loginctl) remoteSelf(ctx context.Context) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.self != "" {
		return l.self, nil
	}
	ctx, cancel := l.timeoutCtx(ctx)
	defer cancel()
	out, err := l.Remote.Output(ctx, "id", "-un")
	if err != nil {
		return "", fmt.Errorf("resolve remote user: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("resolve remote user: empty id -un output")
	}
	l.self = name
	return name, nil
}
