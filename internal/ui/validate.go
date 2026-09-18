package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/vercmp"
)

// unitIssues indexes validation issues by quadlet base name (file name
// without extension), errors first within each unit.
type unitIssues map[string][]quadlet.Issue

func indexIssues(issues []quadlet.Issue) unitIssues {
	m := unitIssues{}
	for _, is := range issues {
		name := is.File
		if i := strings.LastIndex(name, "."); i > 0 {
			name = name[:i]
		}
		m[name] = append(m[name], is)
	}
	return m
}

// issueMarker returns the table marker for a unit's worst finding.
func (m unitIssues) marker(name string) string {
	worst := -1
	for _, is := range m[name] {
		if int(is.Severity) > worst {
			worst = int(is.Severity)
		}
	}
	switch quadlet.IssueSeverity(worst) {
	case quadlet.SeverityError:
		return " ✗"
	case quadlet.SeverityWarning:
		return " ⚠"
	}
	return ""
}

// validateView renders the problems screen: issues grouped by file with
// severity, plus version-gate annotations for known-new keys.
func (m Model) validateView() string {
	var b strings.Builder
	if len(m.issues) == 0 {
		b.WriteString("No validation issues — all Quadlet files converted cleanly.\n")
		if m.generator == "" {
			b.WriteString("(podman-system-generator not found; validation unavailable)\n")
		}
		return b.String()
	}
	b.WriteString("From the Quadlet generator itself (dry-run):\n\n")
	names := make([]string, 0, len(m.issues))
	for name := range m.issues {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(&b, "%s\n", name)
		for _, is := range m.issues[name] {
			sev := "warn"
			if is.Severity == quadlet.SeverityError {
				sev = "ERROR"
			}
			fmt.Fprintf(&b, "  [%s] %s\n", sev, is.Message)
			if hint := m.gateHint(is); hint != "" {
				fmt.Fprintf(&b, "        %s\n", hint)
			}
		}
	}
	if hints := m.smartHints(); len(hints) > 0 {
		b.WriteString("\nHINTS (from live state):\n")
		for _, h := range hints {
			b.WriteString("  • " + h + "\n")
		}
	}
	b.WriteString("\nesc back · edit the file with E to fix")
	return b.String()
}

// gatedKeys maps Quadlet keys to the Podman version that introduced them.
var gatedKeys = map[string]string{
	"ImageVolume": "6.1",
}

// gateHint annotates findings that stem from using a too-new key.
func (m Model) gateHint(is quadlet.Issue) string {
	for key, minVer := range gatedKeys {
		if strings.Contains(is.Message, "'"+key+"'") && m.podmanVersion != "" && !vercmp.AtLeast(m.podmanVersion, minVer) {
			return fmt.Sprintf("%s= requires Podman >= %s (installed: %s)", key, minVer, m.podmanVersion)
		}
	}
	return ""
}
