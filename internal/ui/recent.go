package ui

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// errUnhealthy marks a finished healthcheck with a failing result, so the
// recent-actions log shows it as failed while the status line keeps its
// dedicated UNHEALTHY wording.
var errUnhealthy = errors.New("unhealthy")

// actionRecord is one entry of the recent-actions log: what ran, when, and
// whether it worked. Shown on the actions screen (A).
type actionRecord struct {
	at   time.Time
	desc string
	ok   bool
	err  string
}

// maxActions bounds the in-memory recent-actions log.
const maxActions = 20

// recordAction appends a finished action to the recent-actions log.
func (m *Model) recordAction(desc string, err error) {
	rec := actionRecord{at: time.Now(), desc: desc, ok: err == nil}
	if err != nil {
		rec.err = err.Error()
	}
	m.actions = append(m.actions, rec)
	if len(m.actions) > maxActions {
		m.actions = m.actions[len(m.actions)-maxActions:]
	}
}

// actionsView renders the recent-actions screen body.
func (m Model) actionsView() string {
	var b strings.Builder
	if len(m.actions) == 0 {
		b.WriteString("No actions yet this session.\n")
		b.WriteString("Start, stop, reload, linger, health, and timer actions are logged here with their time and result.\n")
	} else {
		for _, a := range m.actions {
			mark := "ok"
			if !a.ok {
				mark = "FAIL"
			}
			line := fmt.Sprintf("%s  %-4s  %s", a.at.Format("15:04:05"), mark, a.desc)
			if !a.ok && a.err != "" {
				line += " - " + a.err
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\nesc back")
	return b.String()
}
