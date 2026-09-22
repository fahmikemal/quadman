package ui

import (
	"context"
	"errors"
	"os"

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
// merged drop-ins locally; base content only in isolated sessions
// (drop-in enumeration needs local dir access), with a note saying so.
func (m Model) fileContent(u quadlet.Unit, base []byte) string {
	if !m.ssh.Isolated() {
		return withDropins(u, base)
	}
	return string(base) + "\n# ── isolated session: drop-ins not listed ──\n"
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
