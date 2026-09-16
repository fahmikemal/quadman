package quadlet

import (
	"os"
	"strings"
)

// QuadletDoc is one document inside a multi-document .quadlets bundle.
type QuadletDoc struct {
	Name    string // # FileName= value (base name without extension)
	Kind    Kind   // inferred from the first [Section] header
	Content string
}

// ParseQuadlets splits a .quadlets bundle into its documents. Documents are
// separated by lines containing only `---`; each must declare its name via a
// `# FileName=<base-name>` comment (no extension — the kind comes from the
// section header).
func ParseQuadlets(path string) ([]QuadletDoc, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- bundle path comes from quadlet discovery
	if err != nil {
		return nil, err
	}
	var docs []QuadletDoc
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		if d := parseQuadletDoc(strings.Join(cur, "\n")); d.Name != "" {
			docs = append(docs, d)
		}
		cur = nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "---" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return docs, nil
}

// parseQuadletDoc extracts the FileName and infers the kind from the first
// section header of one document.
func parseQuadletDoc(content string) QuadletDoc {
	d := QuadletDoc{Content: content}
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "#") {
			if fn, ok := strings.CutPrefix(t, "# FileName="); ok {
				d.Name = strings.TrimSpace(fn)
			}
			continue
		}
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			d.Kind = Kind(strings.ToLower(strings.Trim(t, "[]")))
			break // first section decides the kind
		}
	}
	return d
}
