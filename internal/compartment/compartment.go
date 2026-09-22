// Package compartment manages quadman compartments: per-service OS users
// whose Quadlet workloads are administered from one TUI. Each compartment
// is an ordinary Linux user with its own Quadlet search path, systemd user
// instance, podman storage, and secret store; the operator reaches it
// through non-interactive sudo (`sudo -n -u <user>`), never a shell.
//
// A compartment needs no daemon and no new credentials: if
// `sudo -n -u <user> true` works (NOPASSWD sudoers or root operator),
// quadman works. Target users need a runtime dir (/run/user/<uid>),
// which lingering (`loginctl enable-linger <user>`) guarantees.
package compartment

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// ProbeTimeout bounds the sudo capability check.
const ProbeTimeout = 10 * time.Second

// Compartment is one administrable OS user.
type Compartment struct {
	// User is the login name (sudo -u target).
	User string
	// UID is the numeric id (users/UID dir matching, runtime dir).
	UID string
	// Home is the user's home (config dir root).
	Home string
	// RuntimeDir is /run/user/<uid> (generator output, containers).
	RuntimeDir string
}

// Self returns the compartment of the invoking user.
func Self() (Compartment, error) {
	u, err := user.Current()
	if err != nil {
		return Compartment{}, fmt.Errorf("current user: %w", err)
	}
	return fromUser(u), nil
}

// Resolve looks up a compartment by login name.
func Resolve(name string) (Compartment, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Compartment{}, fmt.Errorf("empty compartment user")
	}
	u, err := user.Lookup(name)
	if err != nil {
		return Compartment{}, fmt.Errorf("unknown user %q", name)
	}
	return fromUser(u), nil
}

func fromUser(u *user.User) Compartment {
	return Compartment{
		User:       u.Username,
		UID:        u.Uid,
		Home:       u.HomeDir,
		RuntimeDir: filepath.Join("/run", "user", u.Uid),
	}
}

// ConfigDir mirrors os.UserConfigDir for the compartment ($HOME/.config).
func (c Compartment) ConfigDir() string {
	return filepath.Join(c.Home, ".config")
}

// RuntimeReady reports whether the user's runtime dir exists. Without it
// there is no user systemd instance to talk to; lingering the user (or a
// fresh login) creates it.
func (c Compartment) RuntimeReady() bool {
	fi, err := os.Stat(c.RuntimeDir)
	return err == nil && fi.IsDir()
}

// Runner returns the sudo runner reaching this compartment, carrying the
// target's session environment. All quadman CLI clients (systemctl,
// journalctl, loginctl, podman, podlet) accept it like the SSH runner.
func (c Compartment) Runner() remote.Runner {
	return remote.Runner{
		As: c.User,
		Env: []string{
			"XDG_RUNTIME_DIR=" + c.RuntimeDir,
			"HOME=" + c.Home,
		},
		// "/" — never the operator's cwd (often 0700) nor the
		// target's home (Go chdirs as the operator pre-setuid, so
		// it must be traversable by the operator too).
		Dir: "/",
	}
}

// GeneratorDir is the systemd generator output dir inside the user's
// runtime dir, where Quadlet units land after a daemon-reload.
func (c Compartment) GeneratorDir() string {
	return filepath.Join(c.RuntimeDir, "systemd", "generator")
}

// ProbeSudo verifies non-interactive sudo to user works, failing fast with
// an actionable message instead of hanging on a password prompt.
func ProbeSudo(ctx context.Context, username string) error {
	ctx, cancel := context.WithTimeout(ctx, ProbeTimeout)
	defer cancel()
	r := remote.Runner{As: username}
	if _, err := r.Output(ctx, "true"); err != nil {
		return fmt.Errorf("sudo -n -u %s: %w (configure NOPASSWD sudoers or run as root)", username, err)
	}
	return nil
}

// UIDInt parses the numeric uid for callers that need it.
func (c Compartment) UIDInt() (int, error) {
	return strconv.Atoi(c.UID)
}
