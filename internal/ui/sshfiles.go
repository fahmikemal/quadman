package ui

import (
	"context"
	"errors"
	"os"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

// errNoSuchUnit is returned when a tree lookup names a unit that is not in
// the current list.
var errNoSuchUnit = errors.New("no such unit")

// remoteWriteMsg is shown when a file-mutating action is attempted over SSH.
const remoteWriteMsg = "not available over SSH: manage files directly on the host (ssh there and run quadman locally)"

// refuseRemoteWrite reports whether file-mutating actions are unavailable
// (SSH mode), setting the status explanation when so. Remote mode keeps
// full read and lifecycle control; only local file edits are out of reach.
func (m *Model) refuseRemoteWrite() bool {
	if !m.ssh.IsRemote() {
		return false
	}
	m.setStatus(remoteWriteMsg, true)
	return true
}

// readUnitFile returns a unit's source content, locally or via SSH cat.
func (m Model) readUnitFile(u quadlet.Unit) ([]byte, error) {
	if m.ssh.IsRemote() {
		if u.Path == "" {
			return nil, os.ErrNotExist
		}
		return m.ssh.Cat(context.Background(), u.Path)
	}
	return os.ReadFile(u.Path)
}

// fileContent renders a unit's source for the file view: base content plus
// merged drop-ins locally; base content only over SSH (drop-in enumeration
// needs local dir access), with a note saying so.
func (m Model) fileContent(u quadlet.Unit, base []byte) string {
	if !m.ssh.IsRemote() {
		return withDropins(u, base)
	}
	return string(base) + "\n# ── remote host: drop-ins not listed in SSH mode ──\n"
}
func (m Model) parseUnitFile(u quadlet.Unit) (*quadlet.File, error) {
	if m.ssh.IsRemote() {
		data, err := m.readUnitFile(u)
		if err != nil {
			return nil, err
		}
		return quadlet.ParseBytes(u.Path, data)
	}
	return quadlet.Parse(u.Path)
}
