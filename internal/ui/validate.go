package ui

import (
	"fmt"
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
