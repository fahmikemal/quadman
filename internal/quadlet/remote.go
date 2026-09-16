package quadlet

import (
	"context"
	"sort"
	"strings"

	"github.com/kemal-labs/quadman/internal/remote"
)

// DiscoverRemote lists Quadlet units on a remote host. It prefers
// `podman quadlet list` (name, kind inferred from the file extension,
// generated unit, source path on the remote) and falls back to
// `systemctl list-units` when podman is absent remotely. Unit.Path is the
// remote path: file contents are read on demand with Runner.Cat, never
// synced locally.
func DiscoverRemote(ctx context.Context, r remote.Runner) ([]Unit, error) {
	if out, err := r.Output(ctx, "podman", "quadlet", "list", "--format", "{{.Name}}\t{{.UnitName}}\t{{.Path}}"); err == nil {
		return parseQuadletList(out), nil
	}
	out, err := r.Output(ctx, "systemctl", "--user", "list-units", "--type=service", "--all", "--plain", "--no-legend", "--no-pager")
	if err != nil {
		return nil, err
	}
	return parseListUnits(out), nil
}

func parseQuadletList(out []byte) []Unit {
	var units []Unit
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		name, unit, path := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
		if name == "" {
			continue
		}
		kind := KindContainer
		if i := strings.LastIndex(name, "."); i >= 0 {
			if k, ok := kindByName(name[i+1:]); ok {
				kind = k
			}
		}
		base := name
		if i := strings.LastIndex(base, "."); i >= 0 {
			base = base[:i]
		}
		units = append(units, Unit{Name: base, Kind: kind, Path: path, UnitName: unit})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units
}

func kindByName(ext string) (Kind, bool) {
	for e, k := range kindByExt {
		if strings.TrimPrefix(e, ".") == ext {
			return k, true
		}
	}
	return "", false
}

func parseListUnits(out []byte) []Unit {
	var units []Unit
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if !strings.HasSuffix(name, ".service") {
			continue
		}
		base := strings.TrimSuffix(name, ".service")
		units = append(units, Unit{Name: base, Kind: KindContainer, Path: "", UnitName: name})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units
}

// ParseBytes parses Quadlet file content already read into memory (e.g. via
// Runner.Cat from a remote host). It shares the line parser with Parse.
func ParseBytes(path string, data []byte) (*File, error) {
	f := &File{Path: path, Sections: map[string]*Section{}}
	var cur *Section
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			i++
			line = strings.TrimRight(strings.TrimSuffix(line, "\\"), " \t") + " " + strings.TrimSpace(lines[i])
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			lower := strings.ToLower(name)
			if sec, exists := f.Sections[lower]; exists {
				cur = sec
			} else {
				cur = &Section{Name: name, keys: map[string][]string{}}
				f.Sections[lower] = cur
			}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || cur == nil {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		cur.keys[key] = append(cur.keys[key], strings.TrimSpace(val))
	}
	return f, nil
}
