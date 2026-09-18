package ui

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/systemd"
)

// autoUpdateTimer is the user timer that drives `podman auto-update`.
const autoUpdateTimer = "podman-auto-update.timer"

type updatesMsg struct {
	entries      []podman.AutoUpdateEntry
	timerEnabled string // "enabled" / "disabled" / "static" / …
	timerActive  string // "active" / "inactive" / …
	err          error
}

type timerToggledMsg struct{ err error }

// updatesCmd previews `podman auto-update --dry-run` and reads the timer
// state. Works without podman: the screen then shows the timer only.
func updatesCmd(sys *systemd.Systemd) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg := updatesMsg{}
		if podman.Available() {
			entries, err := podman.AutoUpdateDryRun(ctx)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.entries = entries
		}
		msg.timerEnabled, _ = sys.IsEnabled(ctx, autoUpdateTimer)
		msg.timerActive, _ = sys.IsActive(ctx, autoUpdateTimer)
		return msg
	}
}

// updatesView renders the auto-update screen body.
func (m Model) updatesView() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Timer %s: %s · %s\n\n", autoUpdateTimer, orUnknown(m.timerEnabled), orUnknown(m.timerActive)))
	if len(m.updateEntries) == 0 {
		b.WriteString("No units with an AutoUpdate policy.\n")
		b.WriteString("Set AutoUpdate=registry (or =local) in a [Container] file to opt in.\n")
	} else {
		var tw strings.Builder
		w := tabwriter.NewWriter(&tw, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "UNIT\tPOLICY\tUPDATED\tIMAGE")
		for _, e := range m.updateEntries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Unit, e.Policy, e.Updated, e.Image)
		}
		_ = w.Flush()
		b.WriteString(tw.String())
	}
	b.WriteString("\nU toggles the timer (enable --now / disable --now) · r re-runs the dry-run preview · esc back")
	return b.String()
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
