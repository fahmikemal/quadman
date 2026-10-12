package quadlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/remote"
)

// validateTimeout bounds one generator dry-run.
const validateTimeout = 15 * time.Second

// IssueSeverity classifies a validation finding.
type IssueSeverity int

const (
	SeverityWarning IssueSeverity = iota
	SeverityError
)

// Issue is one validation finding for a Quadlet source file, as reported by
// the generator itself in dry-run mode.
type Issue struct {
	File     string // source file base name, e.g. webapp.container
	Path     string // absolute path when the generator reports it, else ""
	Severity IssueSeverity
	Message  string
}

// generatorPaths are the usual install locations of podman-system-generator.
var generatorPaths = []string{
	"/usr/lib/systemd/system-generators/podman-system-generator",
	"/usr/libexec/podman/podman-system-generator",
	"/usr/local/lib/systemd/system-generators/podman-system-generator",
}

// GeneratorBinary returns the first podman-system-generator found on disk,
// or "" when none is installed.
func GeneratorBinary() string {
	for _, p := range generatorPaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

var (
	// converting "webapp.container": unsupported key 'Foo' in group 'Container' in /path/webapp.container
	convertErrRe = regexp.MustCompile(`^converting "([^"]+)": (.+?) in (\S+)$`)
	// Warning: webapp.container specifies the image "x" which ...
	warningRe = regexp.MustCompile(`^Warning: (\S+) (.+)$`)
)

// Validate runs the Quadlet generator in dry-run mode over dirs in user (rootless) mode.
func Validate(ctx context.Context, dirs []string) ([]Issue, error) {
	return ValidateMode(ctx, dirs, false)
}

// ValidateMode runs the Quadlet generator in dry-run mode over dirs.
// When system is true, it omits the -user flag for system-wide validation.
func ValidateMode(ctx context.Context, dirs []string, system bool) ([]Issue, error) {
	bin := GeneratorBinary()
	if bin == "" || len(dirs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, dryRunArgs(system)...) // #nosec G204 -- bin comes from the fixed generatorPaths list; argv slice, no shell
	cmd.Env = append(os.Environ(), "QUADLET_UNIT_DIRS="+strings.Join(dirs, ":"))
	out, _ := cmd.CombinedOutput() // findings live on both streams; exit code is noisy
	return parseIssues(out), nil
}

// ValidateSession validates the Quadlet files of a session. Local sessions
// run ValidateMode over dirs. Isolated sessions (SSH host, sudo
// compartment) run the generator through the runner, as the target user
// and with the generator's own default search path: the target's files are
// neither readable by nor described by the operator's dirs. The first
// installed generator among the usual locations is used; none found (or
// not permitted by sudoers) yields no issues and no error, like a host
// without podman.
func ValidateSession(ctx context.Context, r remote.Runner, dirs []string, system bool) ([]Issue, error) {
	if !r.Isolated() {
		return ValidateMode(ctx, dirs, system)
	}
	ctx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()
	for _, bin := range generatorPaths {
		out, err := r.Command(ctx, bin, dryRunArgs(system)...).CombinedOutput()
		if err != nil && missingGenerator(err, out, bin) {
			continue
		}
		return parseIssues(out), nil
	}
	return nil, nil
}

func dryRunArgs(system bool) []string {
	if system {
		return []string{"-dryrun"}
	}
	return []string{"-dryrun", "-user"}
}

// missingGenerator reports a failure that means "no generator here" rather
// than "the generator ran and found problems": the remote shell's 127, or
// sudo/sh complaining about the path itself.
func missingGenerator(err error, out []byte, bin string) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 127 {
		return true
	}
	text := string(out)
	if !strings.Contains(text, bin) {
		return false
	}
	for _, s := range []string{"not found", "No such file", "not allowed"} {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

// parseIssues reads the generator's dry-run findings.
func parseIssues(out []byte) []Issue {
	var issues []Issue
	for _, line := range strings.Split(string(out), "\n") {
		line = stripGeneratorPrefix(line)
		if m := convertErrRe.FindStringSubmatch(line); m != nil {
			issues = append(issues, Issue{File: m[1], Path: m[3], Severity: SeverityError, Message: m[2]})
			continue
		}
		if m := warningRe.FindStringSubmatch(line); m != nil {
			issues = append(issues, Issue{File: m[1], Severity: SeverityWarning, Message: m[2]})
		}
	}
	return issues
}

// stripGeneratorPrefix removes the "quadlet-generator[pid]: " journal prefix.
func stripGeneratorPrefix(line string) string {
	line = strings.TrimSpace(line)
	if i := strings.Index(line, "]: "); i >= 0 && strings.HasPrefix(line, "quadlet-generator[") {
		return line[i+3:]
	}
	return line
}

// ByFile groups issues by source file base name, errors first.
func ByFile(issues []Issue) map[string][]Issue {
	m := map[string][]Issue{}
	for _, is := range issues {
		m[is.File] = append(m[is.File], is)
	}
	return m
}

// ValidateError renders a compact summary line for logs/tests.
func ValidateError(issues []Issue) string {
	errs, warns := 0, 0
	for _, is := range issues {
		if is.Severity == SeverityError {
			errs++
		} else {
			warns++
		}
	}
	return fmt.Sprintf("%d error(s), %d warning(s)", errs, warns)
}

// ValidateSecrets checks if any Quadlet file references a secret that is not
// present in availableSecrets. If availableSecrets is nil, validation is skipped.
func ValidateSecrets(files []*File, availableSecrets []string) []Issue {
	if availableSecrets == nil {
		return nil
	}
	known := make(map[string]bool, len(availableSecrets))
	for _, s := range availableSecrets {
		known[s] = true
	}

	var issues []Issue
	for _, f := range files {
		if f == nil {
			continue
		}
		base := filepath.Base(f.Path)
		for _, sec := range f.Secrets() {
			if !known[sec.Name] {
				issues = append(issues, Issue{
					File:     base,
					Path:     f.Path,
					Severity: SeverityWarning,
					Message:  fmt.Sprintf("secret %q not found in podman secret store (run: podman secret create %s <file>)", sec.Name, sec.Name),
				})
			}
		}
	}
	return issues
}
