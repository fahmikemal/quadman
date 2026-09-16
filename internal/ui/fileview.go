package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

// withDropins renders a unit file followed by every drop-in the generator
// would merge into it, each under its own separator header.
func withDropins(u quadlet.Unit, base []byte) string {
	drops := quadlet.Dropins(u)
	if len(drops) == 0 {
		return string(base)
	}
	var b strings.Builder
	b.Write(base)
	for _, d := range drops {
		content, err := os.ReadFile(d.Path) // #nosec G304 -- drop-in paths come from quadlet discovery
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "\n# ── drop-in: %s/%s ──\n", d.Dir, baseName(d.Path))
		b.Write(content)
		if !strings.HasSuffix(string(content), "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func baseName(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// withQuadletDocs renders a .quadlets bundle with one separator header per
// document.
func withQuadletDocs(content []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "---" {
			b.WriteString("\n# " + strings.Repeat("─", 40) + "\n")
			continue
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
