package quadlet

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dropin is one *.conf drop-in file that applies to a unit.
type Dropin struct {
	Path string // absolute path of the .conf file
	Dir  string // the drop-in directory it came from (e.g. foo.container.d)
}

// Dropins returns every drop-in that the generator would merge into the
// unit's file, in merge order: the generic <kind>.d directory first, then
// cascading dashed prefixes from least to most specific, then the unit's own
// <name>.<kind>.d directory. Within each directory, *.conf files apply in
// alphabetical order.
//
// For web-api.container this yields: container.d/*.conf, web-.container.d/*.conf,
// web-api.container.d/*.conf — matching the generator's behavior.
func Dropins(u Unit) []Dropin {
	dir := filepath.Dir(u.Path)
	var out []Dropin
	kind := string(u.Kind)

	dirs := []string{filepath.Join(dir, kind+".d")}
	// Cascading dashed prefixes: for "web-api", add "web-.container.d" before
	// the unit's own dir (which is appended below).
	name := u.Name
	for i := strings.Index(name, "-"); i >= 0; {
		prefix := name[:i+1] // keep the trailing dash
		dirs = append(dirs, filepath.Join(dir, prefix+"."+kind+".d"))
		rest := name[i+1:]
		j := strings.Index(rest, "-")
		if j < 0 {
			break
		}
		i += j + 1
	}
	dirs = append(dirs, filepath.Join(dir, name+"."+kind+".d"))

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue // missing dirs are normal
		}
		var confs []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
				confs = append(confs, e.Name())
			}
		}
		sort.Strings(confs)
		for _, c := range confs {
			out = append(out, Dropin{Path: filepath.Join(d, c), Dir: filepath.Base(d)})
		}
	}
	return out
}
