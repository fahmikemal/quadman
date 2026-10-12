package ui

import (
	"path/filepath"
	"strings"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// isExternalDir reports whether dir is a user-configured extra directory
// (or below one) rather than part of the generator search path.
func isExternalDir(dir string) bool {
	clean := filepath.Clean(dir)
	for _, d := range quadlet.ExtraDirList() {
		root := filepath.Clean(d)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// isExternalUnit reports whether a unit's file lives in an extra directory.
func isExternalUnit(path string) bool {
	return path != "" && isExternalDir(filepath.Dir(path))
}

// externalHint is appended to statuses for external units.
func externalHint() string {
	return "external dir: copy or symlink into a search-path dir, then R"
}
