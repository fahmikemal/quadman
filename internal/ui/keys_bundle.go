package ui

import (
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

func (m Model) bundleKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	// .quadlets bundles get their own small action set: preview + install.
	if u, ok := m.selected(); ok && u.Kind == quadlet.KindQuadlets {
		switch msg.String() {
		case "enter":
			content, err := os.ReadFile(u.Path)
			if err != nil {
				m.setStatus(err.Error(), true)
				return m, nil, true
			}
			m.viewport.SetContent(withQuadletDocs(content))
			m.viewport.GotoTop()
			m.mode = modeDetail
			m.tab = tabSource
			m.resize()
			return m, nil, true
		case "I":
			m.pending = &pendingAction{verb: "install", unit: u}
			m.setStatus("install "+filepath.Base(u.Path)+" via podman quadlet install? [y/N]", false)
			return m, nil, true
		case "s", "x", "r", "e", "d", "l", "h", "E":
			m.setStatus("install the bundle first (I) to manage its units", false)
			return m, nil, true
		}
	}
	return m, nil, false
}
