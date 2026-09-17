// Package quadlet discovers and parses Quadlet unit source files.
package quadlet

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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
// NOTE: systemd's generator does not read these directories, so units that
// exist only here cannot be started until their files are also visible to
// the generator (copy them or symlink them into a search-path directory).
// quadman lists, previews, and validates them anyway, and marks them.
var ExtraDirs []string

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
		return append(dirs, ExtraDirs...)
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
	return append(dirs, ExtraDirs...)
}

// userDirs lists the per-user directories below one generator base dir, in
// lookup order: the users dir itself, its non-numeric subdirectories (shared
// units), then the invoking user's own UID dir. Numeric subdirectories that
// belong to other users are skipped, like the generator does.
func userDirs(base, uid string) []string {
	users := filepath.Join(base, "users")
	dirs := []string{users}
	if entries, err := os.ReadDir(users); err == nil {
		for _, e := range entries {
			if !e.IsDir() || isNumeric(e.Name()) {
				continue
			}
			dirs = append(dirs, filepath.Join(users, e.Name()))
		}
	}
	return append(dirs, filepath.Join(users, uid))
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

// staleTolerance absorbs filesystem timestamp granularity when comparing
// source and generated unit mtimes.
const staleTolerance = time.Second

// StaleUnits returns the units whose source file is newer than the unit the
// generator produced in genDir (or that have no generated unit yet) — in
// other words, the ones a `systemctl --user daemon-reload` would change.
// An empty genDir (runtime dir unknown) never reports stale units.
func StaleUnits(units []Unit, genDir string) []Unit {
	if genDir == "" {
		return nil
	}
	var stale []Unit
	for _, u := range units {
		gen, err := os.Stat(filepath.Join(genDir, u.UnitName))
		if err != nil || !gen.ModTime().Add(staleTolerance).After(u.mtime()) {
			stale = append(stale, u)
		}
	}
	return stale
}

// mtime is the source file's modification time (zero when unreadable).
func (u Unit) mtime() time.Time {
	if fi, err := os.Stat(u.Path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
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

// DiscoverDirs scans an explicit list of directories for Quadlet files.
func DiscoverDirs(dirs []string) ([]Unit, error) {
	var units []Unit
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue // drop-in directories (.d) are not scanned for now
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
	return units, nil
}
