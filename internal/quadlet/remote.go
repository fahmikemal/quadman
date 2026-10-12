package quadlet

import (
	"context"
	"path"
	"sort"
	"strings"

	"github.com/fahmikemal/quadman/internal/remote"
)

// DiscoverRemote lists Quadlet units on a remote host for the user
// (rootless) session.
func DiscoverRemote(ctx context.Context, r remote.Runner) ([]Unit, error) {
	return DiscoverRemoteMode(ctx, r, false)
}

// DiscoverRemoteMode lists the Quadlet units of an isolated session (SSH
// host or sudo compartment) for either the user or the system instance.
//
// It prefers `podman quadlet list`, which reports every quadlet the
// generator sees together with its source path. Without podman it asks
// systemd: the units the generator produced (`list-unit-files
// --state=generated`) and, from `systemctl show`, the SourcePath= Quadlet
// stamps on each of them. Only units whose source is a Quadlet file are
// kept, so other generators' output (and ordinary services) never show up
// as quadlets. Unit.Path is the path on the session's side: file contents
// are read on demand through the runner, never synced locally.
func DiscoverRemoteMode(ctx context.Context, r remote.Runner, system bool) ([]Unit, error) {
	if out, err := r.Output(ctx, "podman", "quadlet", "list", "--format", "{{.Name}}\t{{.UnitName}}\t{{.Path}}"); err == nil {
		return parseQuadletList(out), nil
	}
	scope := func(args ...string) []string {
		if system {
			return args
		}
		return append([]string{"--user"}, args...)
	}
	out, err := r.Output(ctx, "systemctl", scope("list-unit-files", "--type=service", "--state=generated", "--no-legend", "--plain", "--no-pager")...)
	if err != nil {
		return nil, err
	}
	names := generatedUnitNames(out)
	if len(names) == 0 {
		return nil, nil
	}
	show, err := r.Output(ctx, "systemctl", scope(append([]string{"show", "--property=Id,SourcePath", "--no-pager", "--"}, names...)...)...)
	if err != nil {
		return nil, err
	}
	return parseSourcePaths(show), nil
}

// generatedUnitNames reads the unit names (first column) of
// `systemctl list-unit-files --no-legend --plain`.
func generatedUnitNames(out []byte) []string {
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && strings.HasSuffix(f[0], ".service") {
			names = append(names, f[0])
		}
	}
	return names
}

// parseSourcePaths turns `systemctl show --property=Id,SourcePath` blocks
// into units, keeping those generated from a Quadlet source file.
func parseSourcePaths(out []byte) []Unit {
	var units []Unit
	id, src := "", ""
	flush := func() {
		if id != "" && src != "" {
			ext := path.Ext(src)
			if kind, ok := kindByExt[ext]; ok && kind != KindQuadlets {
				units = append(units, Unit{
					Name:     strings.TrimSuffix(path.Base(src), ext),
					Kind:     kind,
					Path:     src,
					UnitName: id,
				})
			}
		}
		id, src = "", ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			if strings.TrimSpace(line) == "" {
				flush()
			}
			continue
		}
		switch k {
		case "Id":
			flush()
			id = v
		case "SourcePath":
			src = v
		}
	}
	flush()
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units
}

// InspectRemote resolves one unit's Info by reading its source file through
// the session runner (SSH or sudo compartment). Failures keep the default
// generated name, matching the uncached local fallback.
func InspectRemote(ctx context.Context, r remote.Runner, u Unit) Info {
	info, _ := InspectRemoteFile(ctx, r, u)
	return info
}

// InspectRemoteFile is InspectRemote that also returns the parsed file
// (nil when it could not be read), so one read per refresh serves both the
// list and validation.
func InspectRemoteFile(ctx context.Context, r remote.Runner, u Unit) (Info, *File) {
	if u.Path == "" {
		return InfoOf(u, nil), nil
	}
	data, err := r.Cat(ctx, u.Path)
	if err != nil {
		return InfoOf(u, nil), nil
	}
	f, err := ParseBytes(u.Path, data)
	if err != nil {
		return InfoOf(u, nil), nil
	}
	return InfoOf(u, f), f
}

func parseQuadletList(out []byte) []Unit {
	var units []Unit
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			continue
		}
		name, unit, p := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2])
		if name == "" {
			continue
		}
		kind := KindContainer
		base := name
		if i := strings.LastIndex(name, "."); i >= 0 {
			if k, ok := kindByExt[name[i:]]; ok {
				kind = k
				base = name[:i]
			}
		}
		if kind == KindQuadlets {
			unit = ""
		}
		units = append(units, Unit{Name: base, Kind: kind, Path: p, UnitName: unit})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	return units
}
