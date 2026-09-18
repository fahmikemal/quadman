package ui

import (
	"path/filepath"
	"strings"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// externalMarker flags units that live outside the generator search path
// (quadlet_dirs / --quadlet-dir). The generator cannot see them, so their
// units cannot start until the files reach a search-path directory.
const externalMarker = "~"

// isExternalDir reports whether dir is a user-configured extra directory
// rather than part of the generator search path.
func isExternalDir(dir string) bool {
	clean := filepath.Clean(dir)
	for _, d := range quadlet.ExtraDirs {
		if filepath.Clean(d) == clean {
			return true
		}
	}
	return false
}

// isExternalUnit reports whether a unit's file lives in an extra directory.
func isExternalUnit(path string) bool {
	return isExternalDir(filepath.Dir(path))
}

// externalHint is appended to statuses for external units.
func externalHint() string {
	return "external dir: copy or symlink into a search-path dir, then R"
}

// externalSuffix marks external units in the table.
func externalSuffix(name string) string {
	if strings.HasSuffix(name, externalMarker) {
		return name
	}
	return name + externalMarker
}
