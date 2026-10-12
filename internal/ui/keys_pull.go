package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// pullImageCmd pulls image now so the next start does not pay a cold pull
// inside systemd's TimeoutStartSec window. The exists fast-path keeps a
// redundant pull from fetching gigabytes: pulling is idempotent, but
// reporting "already present" is kinder than re-fetching.
func (m Model) pullImageCmd(image string) tea.Cmd {
	pm := m.pm
	return m.actionCmdHint("pull "+image, "image ready in local storage", func(ctx context.Context) (string, error) {
		ok, err := pm.ImageExists(ctx, image)
		if err != nil {
			return "", err
		}
		if ok {
			return "already in local storage", nil
		}
		return pm.PullImage(ctx, image)
	})
}

// unitImage returns the configured image for the selected unit, or "" when
// the poll has not resolved one yet.
func (m Model) unitImage() string {
	u, ok := m.selected()
	if !ok {
		return ""
	}
	return m.images[u.UnitName]
}
