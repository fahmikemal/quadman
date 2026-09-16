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

// Runner executes commands locally or on an SSH target. The zero value
// runs everything locally.
type Runner struct {
	// Target is "[user@]host[:port]" (or any ssh destination expression).
	// Empty means local execution.
	Target string
	// SSHBin overrides the ssh binary path.
	SSHBin string
	// Timeout bounds each remote call; 0 means DialTimeout.
	Timeout time.Duration
}

// IsRemote reports whether commands run over SSH.
func (r Runner) IsRemote() bool { return r.Target != "" }

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
// the destination, then "--" and the remote argv untouched.
func (r Runner) argv(name string, args []string) []string {
	argv := []string{
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(r.timeout().Seconds())),
		"--", r.Target, "--", name,
	}
	return append(argv, args...)
}

// Command returns the exec.Cmd for name with args, locally or over SSH.
// Callers use it exactly like exec.CommandContext: set Stdout/Stderr/extra
// fds, then Start/Run/Output. Callers bound execution with their own
// context timeout; SSH dialing is separately bounded by ConnectTimeout.
func (r Runner) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if !r.IsRemote() {
		return exec.CommandContext(ctx, name, args...)
	}
	return exec.CommandContext(ctx, r.sshBin(), r.argv(name, args)...) // #nosec G204 -- argv slice, no shell; remote argv behind "--"
}

// Output runs name with args and returns its stdout.
func (r Runner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := r.Command(ctx, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if se := strings.TrimSpace(stderr.String()); se != "" {
			return out, fmt.Errorf("%s: %w: %s", remoteLabel(r.Target, name), err, se)
		}
		return out, fmt.Errorf("%s: %w", remoteLabel(r.Target, name), err)
	}
	return out, nil
}

// Cat reads a remote (or local) file. Local reads use os.ReadFile through
// CatLocal; Cat always goes through the command runner so remote files work
// transparently.
func (r Runner) Cat(ctx context.Context, path string) ([]byte, error) {
	return r.Output(ctx, "cat", "--", path)
}

func remoteLabel(target, name string) string {
	if target == "" {
		return name
	}
	return target + ": " + name
}
