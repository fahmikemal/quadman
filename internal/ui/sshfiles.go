package ui

import (
	"context"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// remoteWriteMsg is shown when a file-mutating action is attempted over SSH.
const remoteWriteMsg = "not available over SSH: manage files directly on the host (ssh there and run quadman locally)"

// fileTimeout bounds one file read through the session runner.
const fileTimeout = 30 * time.Second

// readUnitFile returns a unit's source content: locally, or via the session
// runner (SSH cat / sudo cat) when isolated.
func (m Model) readUnitFile(u quadlet.Unit) ([]byte, error) {
	if u.Path == "" {
		return nil, os.ErrNotExist
	}
	return m.readPath(u.Path)
}

// readPath reads one file of the session: locally, or through the runner.
func (m Model) readPath(p string) ([]byte, error) {
	if !m.ssh.Isolated() {
		return os.ReadFile(p) // #nosec G304 -- a quadlet source or drop-in the session listed
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
	defer cancel()
	return m.ssh.Cat(ctx, p)
}

// unitSource is one file the generator merges into a unit.
type unitSource struct {
	path string
	data []byte
}

// readUnitSources returns u's file followed by its drop-ins in the
// generator's merge order, locally or through the session runner.
func (m Model) readUnitSources(u quadlet.Unit) ([]unitSource, error) {
	data, err := m.readUnitFile(u)
	if err != nil {
		return nil, err
	}
	srcs := []unitSource{{path: u.Path, data: data}}
	var drops []quadlet.Dropin
	if m.ssh.Isolated() {
		drops, _ = m.dropinsRemote(u)
	} else {
		drops = quadlet.Dropins(u)
	}
	for _, d := range drops {
		if b, err := m.readPath(d.Path); err == nil {
			srcs = append(srcs, unitSource{path: d.Path, data: b})
		}
	}
	return srcs, nil
}

// fileContent renders a unit's source for the file view: base content plus
// merged drop-ins, locally or through the session runner when isolated. A
// listed-but-empty unit renders base only; a failed enumeration keeps the
// note saying drop-ins could not be listed.
func (m Model) fileContent(u quadlet.Unit, base []byte) string {
	if !m.ssh.Isolated() {
		return withDropins(u, base)
	}
	drops, ok := m.dropinsRemote(u)
	if len(drops) > 0 {
		return renderDropins(base, drops, m.readPath)
	}
	if ok {
		return string(base)
	}
	return string(base) + "\n# ── isolated session: drop-ins not listed ──\n"
}

// dropinNames parses `ls -1Ap` output into sorted .conf basenames,
// skipping directories (trailing /) and anything not ending .conf.
func dropinNames(out []byte) []string {
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasSuffix(name, "/") || !strings.HasSuffix(name, ".conf") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// dropinsRemote enumerates drop-ins through the session runner (SSH or sudo):
// the same candidate dirs and merge order as quadlet.Dropins, read-only.
// Missing dirs and unreadable files are skipped like locally. ok reports
// whether at least one directory listed successfully.
func (m Model) dropinsRemote(u quadlet.Unit) (drops []quadlet.Dropin, ok bool) {
	if u.Path == "" {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileTimeout)
	defer cancel()
	for _, d := range quadlet.DropinDirs(u) {
		out, err := m.ssh.Output(ctx, "ls", "-1Ap", "--", d)
		if err != nil {
			continue // missing dirs are normal
		}
		ok = true
		for _, name := range dropinNames(out) {
			drops = append(drops, quadlet.Dropin{Path: path.Join(d, name), Dir: path.Base(d)})
		}
	}
	return drops, ok
}
func (m Model) parseUnitFile(u quadlet.Unit) (*quadlet.File, error) {
	if m.ssh.Isolated() {
		data, err := m.readUnitFile(u)
		if err != nil {
			return nil, err
		}
		return quadlet.ParseBytes(u.Path, data)
	}
	return quadlet.Parse(u.Path)
}
