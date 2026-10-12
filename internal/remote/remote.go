// Package remote runs quadman's CLI calls outside the operator's own
// session: on a remote host over SSH, or as another local OS user through
// non-interactive sudo (compartments).
//
// SSH shells out to the user's own ssh binary (keys, agent, known_hosts,
// and ~/.ssh/config all keep working) with batch mode and a connect
// timeout, so there is no new credential handling: if `ssh <target> true`
// works, quadman works.
//
// OpenSSH has no argv transport: it joins the remote command into one
// string that the remote login shell parses again. The runner therefore
// renders every word as a POSIX single-quoted literal (see Quote), so the
// remote side receives exactly the argv quadman built — spaces, tabs,
// quotes, globs, and $ included. The remote login shell must be
// POSIX-compatible (sh, dash, bash, zsh, ksh).
//
// Sudo stays argv-based end to end: `sudo -n -u <user> K=V… -- cmd args`.
package remote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DialTimeout bounds establishing the SSH connection.
const DialTimeout = 10 * time.Second

// controlPersist is how long a multiplexed SSH master stays up after its
// last session closes.
const controlPersist = "60"

// Runner executes commands locally, on an SSH target, or as another local
// OS user via sudo (compartments). The zero value runs everything locally
// as the current user.
type Runner struct {
	// Target is "[user@]host[:port]" (or any ssh destination expression).
	// Empty means no SSH hop.
	Target string
	// As impersonates a local OS user: commands run as
	// `sudo -n -u As K=V… -- <cmd>`. Empty means the current user.
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
	// Timeout bounds establishing each SSH connection; 0 means DialTimeout.
	Timeout time.Duration
	// Multiplex shares one SSH connection across calls (OpenSSH
	// ControlMaster with a private control socket). A refresh issues a
	// dozen short commands; without multiplexing each one pays a full
	// handshake.
	Multiplex bool
}

// IsRemote reports whether commands run over SSH.
func (r Runner) IsRemote() bool { return r.Target != "" }

// Isolated reports whether commands run outside the operator's own
// session (SSH target or sudo compartment). Capability gates (file
// writes, interactive TTY, destructive host actions) use this: the
// operator's local files, terminal, and storage are not the target's.
func (r Runner) Isolated() bool { return r.Target != "" || r.As != "" }

// Label names the session for messages: "host", "sudo -u svc", or "".
func (r Runner) Label() string {
	switch {
	case r.Target != "":
		return r.Target
	case r.As != "":
		return "sudo -u " + r.As
	}
	return ""
}

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

// Quote renders s as one POSIX shell word. Words made only of characters
// no shell treats specially stay bare (readable logs); everything else is
// single-quoted, with embedded single quotes closed, escaped, and reopened.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	if isBareWord(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isBareWord reports whether s needs no quoting in a POSIX shell.
func isBareWord(s string) bool {
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("@%+=:,./-_", c):
		default:
			return false
		}
	}
	return true
}

// validEnvKey reports a portable environment variable name.
func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, c := range k {
		switch {
		case c == '_', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// envPairs keeps the well-formed KEY=value entries of env, in order.
// Malformed entries come from programming errors, never user input, and
// are dropped rather than passed to a shell or sudo as a command word.
func envPairs(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if k, _, ok := strings.Cut(kv, "="); ok && validEnvKey(k) {
			out = append(out, kv)
		}
	}
	return out
}

// RemoteCommand renders env assignments, name, and args as the single
// command line OpenSSH hands to the remote login shell. Every word is
// quoted, so the remote shell reproduces the argv exactly.
func RemoteCommand(env []string, name string, args []string) string {
	var b strings.Builder
	for _, kv := range envPairs(env) {
		k, v, _ := strings.Cut(kv, "=")
		b.WriteString(k + "=" + Quote(v) + " ")
	}
	// A bare command word holding "=" would read as an assignment.
	if strings.Contains(name, "=") && isBareWord(name) {
		b.WriteString("'" + name + "'")
	} else {
		b.WriteString(Quote(name))
	}
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(Quote(a))
	}
	return b.String()
}

// sshArgv builds the ssh invocation: options, then "--" so the target can
// never be read as an option, then the destination and the quoted command.
func (r Runner) sshArgv(env []string, name string, args []string) []string {
	argv := []string{
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(r.timeout().Seconds())),
	}
	argv = append(argv, r.muxOptions()...)
	return append(argv, "--", r.Target, RemoteCommand(env, name, args))
}

// sudoArgv builds the impersonation invocation: non-interactive sudo (-n
// fails fast instead of hanging on a password prompt), the target user,
// the session variables as sudo's own VAR=value arguments, then "--" and
// the command. sudo only reads VAR=value before "--", and setting them
// needs the SETENV tag (implied by a NOPASSWD: ALL rule), so sudoers can
// allow exactly the CLIs quadman runs instead of a blanket /usr/bin/env.
func (r Runner) sudoArgv(env []string, name string, args []string) []string {
	argv := []string{"-n", "-u", r.As}
	argv = append(argv, envPairs(r.Env)...)
	argv = append(argv, envPairs(env)...)
	argv = append(argv, "--", name)
	return append(argv, args...)
}

// Command returns the exec.Cmd for name with args: locally, over SSH, or
// under sudo impersonation. Callers use it exactly like
// exec.CommandContext: set Stdout/Stderr/Stdin, then Start/Run/Output.
// Callers bound execution with their own context timeout; SSH dialing is
// separately bounded by ConnectTimeout.
func (r Runner) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	return r.CommandEnv(ctx, nil, name, args...)
}

// CommandEnv is Command with extra KEY=value pairs for this one command
// (e.g. LC_ALL=C for parseable output). They reach the command in every
// mode: the local environment, a remote shell assignment, or sudo.
func (r Runner) CommandEnv(ctx context.Context, env []string, name string, args ...string) *exec.Cmd {
	if r.IsRemote() {
		return exec.CommandContext(ctx, r.sshBin(), r.sshArgv(env, name, args)...) // #nosec G204 -- argv slice; the remote command is fully quoted
	}
	if r.As != "" {
		cmd := exec.CommandContext(ctx, "sudo", r.sudoArgv(env, name, args)...) // #nosec G204 -- argv slice, no shell; fixed sudo prefix
		cmd.Dir = r.Dir
		if cmd.Dir == "" {
			cmd.Dir = "/"
		}
		return cmd
	}
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- argv slice, no shell; callers pass fixed CLI binaries and "--"-separated args
	if pairs := envPairs(env); len(pairs) > 0 {
		cmd.Env = append(os.Environ(), pairs...)
	}
	return cmd
}

// Output runs name with args and returns its stdout. On failure the error
// names the session and carries the command's stderr.
func (r Runner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return r.OutputEnv(ctx, nil, name, args...)
}

// OutputEnv is Output with extra KEY=value pairs (see CommandEnv).
func (r Runner) OutputEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := r.CommandEnv(ctx, env, name, args...)
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

// Cat reads a file through the runner, so remote and compartment files
// work transparently.
func (r Runner) Cat(ctx context.Context, path string) ([]byte, error) {
	return r.Output(ctx, "cat", "--", path)
}

func (r Runner) label(name string) string {
	if l := r.Label(); l != "" {
		return l + ": " + name
	}
	return name
}

// --- SSH connection multiplexing --------------------------------------------

// mux tracks the private control-socket directory and the masters this
// process may have started, so Shutdown can close them.
var mux struct {
	once    sync.Once
	dir     string
	err     error
	mu      sync.Mutex
	targets map[[2]string]bool // {ssh binary, target}
}

// controlDir returns the process's private (0700) control-socket dir.
func controlDir() (string, error) {
	mux.once.Do(func() {
		mux.dir, mux.err = os.MkdirTemp("", "quadman-ssh-")
	})
	return mux.dir, mux.err
}

// controlPath is the ControlPath for this process; %C is OpenSSH's hash
// of the connection parameters, so each target gets its own socket while
// the path stays well under the unix socket length limit.
func controlPath(dir string) string { return filepath.Join(dir, "%C") }

func (r Runner) muxOptions() []string {
	if !r.Multiplex {
		return nil
	}
	dir, err := controlDir()
	if err != nil {
		return nil // degrade to one connection per call
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil // removed by Shutdown: plain connections from here on
	}
	mux.mu.Lock()
	if mux.targets == nil {
		mux.targets = map[[2]string]bool{}
	}
	mux.targets[[2]string{r.sshBin(), r.Target}] = true
	mux.mu.Unlock()
	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + controlPath(dir),
		"-o", "ControlPersist=" + controlPersist,
	}
}

// Shutdown closes every multiplexed SSH master this process started and
// removes the control-socket directory. It is safe to call when
// multiplexing was never used, and more than once.
func Shutdown(ctx context.Context) {
	mux.mu.Lock()
	targets := mux.targets
	mux.targets = nil
	mux.mu.Unlock()
	if mux.dir == "" {
		return
	}
	for t := range targets {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(cctx, t[0], "-o", "ControlPath="+controlPath(mux.dir), "-O", "exit", "--", t[1]) // #nosec G204 -- argv slice, no shell; closes our own master
		_ = cmd.Run()
		cancel()
	}
	_ = os.RemoveAll(mux.dir)
}
