// Package remote runs quadman's CLI calls on a remote rootless host over
// SSH. It shells out to the user's own ssh binary (keys, agent,
// known_hosts, and ~/.ssh/config all keep working) with batch mode and a
// connect timeout, so there is no new credential handling: if
// `ssh <target> true` works, quadman works.
//
// Remote execution is argv-based: the remote command and its arguments are
// passed after a "--" separator, never through a remote shell string.
package remote

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DialTimeout bounds establishing the SSH connection.
const DialTimeout = 10 * time.Second

// Runner executes commands locally, on an SSH target, or as another local
// OS user via sudo (compartments). The zero value runs everything locally
// as the current user.
type Runner struct {
	// Target is "[user@]host[:port]" (or any ssh destination expression).
	// Empty means no SSH hop.
	Target string
	// As impersonates a local OS user: commands run as
	// `sudo -n -u As -- env <Env...> <cmd>`. Empty means the current user.
	// Target and As are mutually exclusive; Target wins when both are set.
	As string
	// Env carries KEY=value pairs applied under sudo impersonation
	// (XDG_RUNTIME_DIR, HOME of the target user). Ignored otherwise.
	Env []string
	// Dir is the working directory for sudo-impersonated commands. The
	// operator's cwd is often inaccessible to the target user (e.g. a
	// 0700 home), and some CLIs (podman) fail when they cannot access
	// cwd at all. Empty means "/" (always traversable).
	Dir string
	// SSHBin overrides the ssh binary path.
	SSHBin string
	// Timeout bounds each remote call; 0 means DialTimeout.
	Timeout time.Duration
}

// IsRemote reports whether commands run over SSH.
func (r Runner) IsRemote() bool { return r.Target != "" }

// Isolated reports whether commands run outside the operator's own
// session (SSH target or sudo compartment). Capability gates (file
// writes, interactive TTY, destructive host actions) use this: the
// operator's local files, terminal, and storage are not the target's.
func (r Runner) Isolated() bool { return r.Target != "" || r.As != "" }

func (r Runner) sshBin() string {
	if r.SSHBin != "" {
		return r.SSHBin
	}
	return "ssh"
}

func (r Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DialTimeout
}

// Available reports whether the ssh binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("ssh")
	return err == nil
}

// argv builds the ssh invocation for a remote command: options first, then
// the destination, then the remote command name and its arguments.
func (r Runner) argv(name string, args []string) []string {
	argv := []string{
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(r.timeout().Seconds())),
		"--", r.Target, name,
	}
	return append(argv, args...)
}

// sudoArgv builds the impersonation invocation: non-interactive sudo
// (-n fails fast instead of hanging on a password prompt), the target
// user, then `env` carrying the target's session variables, then the
// command. Fully argv-based, never a shell string.
func (r Runner) sudoArgv(name string, args []string) []string {
	argv := []string{"-n", "-u", r.As, "--"}
	if len(r.Env) > 0 {
		argv = append(argv, "env")
		argv = append(argv, r.Env...)
	}
	argv = append(argv, name)
	return append(argv, args...)
}

// Command returns the exec.Cmd for name with args: locally, over SSH, or
// under sudo impersonation. Callers use it exactly like
// exec.CommandContext: set Stdout/Stderr/extra fds, then Start/Run/Output.
// Callers bound execution with their own context timeout; SSH dialing is
// separately bounded by ConnectTimeout.
func (r Runner) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if r.IsRemote() {
		return exec.CommandContext(ctx, r.sshBin(), r.argv(name, args)...) // #nosec G204 -- argv slice, no shell; remote argv behind "--"
	}
	if r.As != "" {
		cmd := exec.CommandContext(ctx, "sudo", r.sudoArgv(name, args)...) // #nosec G204 -- argv slice, no shell; fixed sudo prefix
		cmd.Dir = r.Dir
		if cmd.Dir == "" {
			cmd.Dir = "/"
		}
		return cmd
	}
	return exec.CommandContext(ctx, name, args...) // #nosec G204 -- argv slice, no shell; callers pass fixed CLI binaries and "--"-separated args
}

// Output runs name with args and returns its stdout.
func (r Runner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := r.Command(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if se := strings.TrimSpace(stderr.String()); se != "" {
			return out, fmt.Errorf("%s: %w: %s", r.label(name), err, se)
		}
		return out, fmt.Errorf("%s: %w", r.label(name), err)
	}
	return out, nil
}

// Cat reads a remote (or local) file. Local reads use os.ReadFile through
// CatLocal; Cat always goes through the command runner so remote files work
// transparently.
func (r Runner) Cat(ctx context.Context, path string) ([]byte, error) {
	return r.Output(ctx, "cat", "--", path)
}

func (r Runner) label(name string) string {
	if r.Target != "" {
		return r.Target + ": " + name
	}
	if r.As != "" {
		return "sudo -u " + r.As + ": " + name
	}
	return name
}
