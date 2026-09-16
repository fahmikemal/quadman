package quadlet

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

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

// Validate runs the Quadlet generator in dry-run mode over dirs and returns
// every finding it reports. It returns nil issues (not an error) when no
// generator binary is installed.
func Validate(ctx context.Context, dirs []string) ([]Issue, error) {
	bin := GeneratorBinary()
	if bin == "" || len(dirs) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-dryrun", "-user")
	cmd.Env = append(os.Environ(), "QUADLET_UNIT_DIRS="+strings.Join(dirs, ":"))
	out, _ := cmd.CombinedOutput() // findings live on both streams; exit code is noisy

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
	return issues, nil
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
