// Package quadlet discovers and parses Quadlet unit source files.
package quadlet

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Kind is the Quadlet file type, named after its extension.
type Kind string

const (
	KindContainer Kind = "container"
	KindPod       Kind = "pod"
	KindKube      Kind = "kube"
	KindVolume    Kind = "volume"
	KindNetwork   Kind = "network"
	KindImage     Kind = "image"
	KindBuild     Kind = "build"
	KindArtifact  Kind = "artifact"
	// KindQuadlets is a multi-document install bundle (*.quadlets); it maps
	// to no single unit until installed via `podman quadlet install`.
	KindQuadlets Kind = "quadlets"
)

// Unit describes one Quadlet source file and the systemd unit it generates.
type Unit struct {
	Name string // base name without extension, e.g. "webapp"
	Kind Kind
	Path string
	// UnitName is the systemd unit the Quadlet generator produces by default.
	// A ServiceName= override in the file is applied by Inspect.
	UnitName string
}

// ExtraDirs holds user-configured Quadlet source directories (from
// quadlet_dirs in config.yaml or --quadlet-dir flags). They are appended
// after the generator's own search path, so generator semantics (first
// match wins) stay intact: same-name files in the standard path keep
// shadowing the extra ones.
//
// It is process-wide configuration: set it once at startup through
// AddExtraDirs, before any session runs, and read it through
// ExtraDirList. Assigning it directly is reserved for tests.
//
// NOTE: systemd's generator does not read these directories, so units that
// exist only here cannot be started until their files are also visible to
// the generator (copy them or symlink them into a search-path directory).
// quadman lists, previews, and validates them anyway, and marks them.
var ExtraDirs []string

// extraMu guards ExtraDirs against a late AddExtraDirs racing readers.
var extraMu sync.RWMutex

// AddExtraDirs appends user-configured Quadlet source directories to the
// discovery search path. Empty and duplicate entries are ignored, so
// calling it from several startup paths is safe.
func AddExtraDirs(dirs []string) {
	extraMu.Lock()
	defer extraMu.Unlock()
	seen := map[string]bool{}
	for _, d := range ExtraDirs {
		seen[d] = true
	}
	for _, d := range dirs {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		ExtraDirs = append(ExtraDirs, d)
	}
}

// ExtraDirList returns a copy of the configured extra directories.
func ExtraDirList() []string {
	extraMu.RLock()
	defer extraMu.RUnlock()
	return slices.Clone(ExtraDirs)
}

// SearchDirs returns the Quadlet source directories for the current user, in
// generator lookup order. It mirrors podman's rootless search path: the
// runtime dir, the user config dir, then the admin/distro users directories.
// System-wide dirs like /etc/containers/systemd are not searched because the
// generator only picks those up for root (system) units, not --user units.
func SearchDirs() []string {
	return SearchDirsMode(false)
}

// SystemSearchDirs returns the Quadlet source directories for system (rootful)
// units, in generator lookup order:
// /run/containers/systemd, /etc/containers/systemd, /usr/share/containers/systemd.
func SystemSearchDirs() []string {
	return SearchDirsMode(true)
}

// SearchDirsMode returns the Quadlet source directories in generator lookup order
// for either user (rootless) or system (rootful) mode, followed by any ExtraDirs.
func SearchDirsMode(system bool) []string {
	if system {
		dirs := []string{
			"/run/containers/systemd",
			"/etc/containers/systemd",
			"/usr/share/containers/systemd",
		}
		return append(dirs, ExtraDirList()...)
	}
	var dirs []string
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		dirs = append(dirs, filepath.Join(rt, "containers", "systemd"))
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfg, "containers", "systemd"))
	}
	uid := strconv.Itoa(os.Getuid())
	for _, base := range []string{"/etc/containers/systemd", "/usr/share/containers/systemd"} {
		dirs = append(dirs, userDirs(base, uid)...)
	}
	return append(dirs, ExtraDirList()...)
}

// SearchDirsFor returns the Quadlet source directories for an arbitrary
// (not necessarily current) user: the compartment-aware mirror of the
// user branch above. home is the user's $HOME, uid their numeric id, and
// runtimeDir their XDG_RUNTIME_DIR ("" when unknown — runtime entries are
// skipped, like an unknown runtime dir for the current user).
func SearchDirsFor(home, uid, runtimeDir string) []string {
	var dirs []string
	if runtimeDir != "" {
		dirs = append(dirs, filepath.Join(runtimeDir, "containers", "systemd"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".config", "containers", "systemd"))
	}
	for _, base := range []string{"/etc/containers/systemd", "/usr/share/containers/systemd"} {
		dirs = append(dirs, userDirs(base, uid)...)
	}
	return append(dirs, ExtraDirList()...)
}

// userDirs lists the per-user search-path entries below one generator
// base dir, in the generator's lookup order: the shared users dir, then
// the invoking user's own UID dir. Discovery walks each entry recursively;
// below users/ it skips the numeric dirs, which belong to individual users
// (the own one is searched as its own entry).
func userDirs(base, uid string) []string {
	users := filepath.Join(base, "users")
	return []string{users, filepath.Join(users, uid)}
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

var kindByExt = map[string]Kind{
	".container": KindContainer,
	".pod":       KindPod,
	".kube":      KindKube,
	".volume":    KindVolume,
	".network":   KindNetwork,
	".image":     KindImage,
	".build":     KindBuild,
	".artifact":  KindArtifact,
	".quadlets":  KindQuadlets,
}

// kindSuffix is the name suffix the generator appends for each kind:
// webapp.container → webapp.service, webapp.volume → webapp-volume.service, …
var kindSuffix = map[Kind]string{
	KindContainer: "",
	KindKube:      "",
	KindPod:       "-pod",
	KindVolume:    "-volume",
	KindNetwork:   "-network",
	KindImage:     "-image",
	KindBuild:     "-build",
	KindArtifact:  "-artifact",
}

// UnitFileName maps a Quadlet base name and kind to the generated systemd
// unit name, following the generator's naming rules — including template
// units ("web@.container" → "web@.service").
func UnitFileName(name string, kind Kind) string {
	suffix := kindSuffix[kind]
	if strings.HasSuffix(name, "@") {
		return name[:len(name)-1] + suffix + "@.service"
	}
	return name + suffix + ".service"
}

// Discover scans the Quadlet search directories for the user (rootless) session.
// Like the generator, the first directory in lookup order wins for a given
// name+kind pair.
func Discover() ([]Unit, error) {
	return DiscoverMode(false)
}

// DiscoverSystem scans the Quadlet search directories for the system (rootful) session.
func DiscoverSystem() ([]Unit, error) {
	return DiscoverMode(true)
}

// DiscoverMode scans the Quadlet search directories for either user or system session.
func DiscoverMode(system bool) ([]Unit, error) {
	units, err := DiscoverDirs(SearchDirsMode(system))
	if err != nil {
		return nil, err
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units, nil
}

// DiscoverDirs scans an explicit list of search-path entries for Quadlet
// files, exactly like the generator: each entry is walked recursively
// (see unitDirs), and the first file found for a given name and kind wins.
func DiscoverDirs(dirs []string) ([]Unit, error) {
	var units []Unit
	seen := map[string]bool{}
	extra := map[string]bool{}
	for _, d := range ExtraDirList() {
		extra[filepath.Clean(d)] = true
	}
	for _, base := range dirs {
		// Extra dirs are often git checkouts: their hidden subtrees
		// (.git, .cache) hold no quadlets and would cost a full walk on
		// every refresh. The generator never reads extra dirs anyway.
		walked, err := unitDirs(base, extra[filepath.Clean(base)])
		if err != nil {
			return nil, err
		}
		for _, dir := range walked {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue // vanished or unreadable subdir: the generator skips it too
			}
			for _, e := range entries {
				if !isUnitFile(dir, e) {
					continue
				}
				ext := filepath.Ext(e.Name())
				kind, ok := kindByExt[ext]
				if !ok {
					continue // skips drop-ins (.conf) and unrelated files
				}
				name := strings.TrimSuffix(e.Name(), ext)
				key := name + "/" + string(kind)
				if seen[key] {
					continue
				}
				seen[key] = true
				unitName := UnitFileName(name, kind)
				if kind == KindQuadlets {
					unitName = "" // install bundles map to N units, not one
				}
				units = append(units, Unit{
					Name:     name,
					Kind:     kind,
					Path:     filepath.Join(dir, e.Name()),
					UnitName: unitName,
				})
			}
		}
	}
	return units, nil
}

// isUnitFile reports a regular file, or a symlink to one (Quadlet supports
// symlinks inside the search paths).
func isUnitFile(dir string, e fs.DirEntry) bool {
	switch {
	case e.Type().IsRegular():
		return true
	case e.Type()&fs.ModeSymlink != 0:
		fi, err := os.Stat(filepath.Join(dir, e.Name()))
		return err == nil && fi.Mode().IsRegular()
	}
	return false
}

// unitDirs expands one search-path entry into the directories the
// generator loads, in its order: the entry itself, then every
// subdirectory depth-first in lexical order. Drop-in directories (*.d) are
// skipped, and so are the numeric per-UID dirs directly below a
// <prefix>/containers/systemd/users entry (other users' units; the own UID
// dir is an entry of its own). skipHidden also prunes dot-directories. A
// symlinked entry is followed, but paths keep the configured spelling.
// A missing or unreadable entry yields no dirs; other errors are returned.
func unitDirs(base string, skipHidden bool) ([]string, error) {
	root, err := filepath.EvalSymlinks(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", base, err)
	}
	usersEntry := strings.HasSuffix(filepath.Clean(base), filepath.Join("containers", "systemd", "users"))
	var dirs []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // unreadable subtree: skip it, keep the rest
		}
		if !d.IsDir() {
			return nil
		}
		if p != root {
			if strings.HasSuffix(d.Name(), ".d") || (skipHidden && strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			if usersEntry && filepath.Dir(p) == root && isNumeric(d.Name()) {
				return fs.SkipDir
			}
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return nil
		}
		dirs = append(dirs, filepath.Join(base, rel))
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", base, err)
	}
	return dirs, nil
}
