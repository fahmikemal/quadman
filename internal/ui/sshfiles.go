package ui

import (
	"context"
	"errors"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// errNoSuchUnit is returned when a tree lookup names a unit that is not in
// the current list.
var errNoSuchUnit = errors.New("no such unit")

// remoteWriteMsg is shown when a file-mutating action is attempted over SSH.
const remoteWriteMsg = "not available over SSH: manage files directly on the host (ssh there and run quadman locally)"

// refuseRemoteWrite reports whether file-mutating actions are unavailable
// (SSH or compartment mode), setting the status explanation when so.
// Isolated modes keep full read and lifecycle control; only local file
// edits are out of reach.
func (m *Model) refuseRemoteWrite() bool {
	if !m.ssh.Isolated() {
		return false
	}
	if m.compOn {
		m.setStatus("not available in compartment "+m.comp.User+": files belong to "+m.comp.User+", not the operator", true)
		return true
	}
	m.setStatus(remoteWriteMsg, true)
	return true
}

// isolatedReason reports whether the session is isolated (SSH target or
// sudo compartment) plus the user-facing reason capability gates display
// when refusing. Local sessions return false.
func (m Model) isolatedReason() (bool, string) {
	if m.ssh.IsRemote() {
		return true, "unavailable over SSH"
	}
	if m.compOn {
		return true, "unavailable in compartment"
	}
	return false, ""
}

// readUnitFile returns a unit's source content: locally, or via the session
// runner (SSH cat / sudo cat) when isolated.
func (m Model) readUnitFile(u quadlet.Unit) ([]byte, error) {
	if m.ssh.Isolated() {
		if u.Path == "" {
			return nil, os.ErrNotExist
		}
		return m.ssh.Cat(context.Background(), u.Path)
	}
	return os.ReadFile(u.Path)
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
		return renderDropins(base, drops, func(p string) ([]byte, error) {
			return m.ssh.Cat(context.Background(), p)
		})
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
	ctx := context.Background()
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
