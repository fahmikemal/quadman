package ui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// bulkMsg is the result of one unit inside a mass action.
type bulkMsg struct {
	desc  string
	done  int
	total int
	err   error
}

// markedUnits returns the marked subset of the visible list, in list order.
// Marks are stored by unit name so refreshes and filters keep them.
func (m Model) markedUnits() []quadlet.Unit {
	var out []quadlet.Unit
	for _, u := range m.filtered {
		if m.marks[u.UnitName] {
			out = append(out, u)
		}
	}
	return out
}

// toggleMark flips the mark on the cursor row.
func (m *Model) toggleMark() {
	u, ok := m.selected()
	if !ok || u.UnitName == "" {
		return
	}
	if m.marks == nil {
		m.marks = map[string]bool{}
	}
	if m.marks[u.UnitName] {
		delete(m.marks, u.UnitName)
	} else {
		m.marks[u.UnitName] = true
	}
}

// clearMarks drops all marks.
func (m *Model) clearMarks() {
	m.marks = map[string]bool{}
}

// bulkLabel renders the mark count for the status line and legend.
func (m Model) bulkLabel() string {
	if n := len(m.markedUnits()); n > 0 {
		return fmt.Sprintf("%d marked", n)
	}
	return ""
}

// markKeys toggles the mark on the cursor row with space.
func (m Model) markKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if msg.String() != "space" {
		return m, nil, false
	}
	if m.mode != modeList {
		return m, nil, false
	}
	if m.refuseReadonly() {
		return m, nil, true
	}
	m.toggleMark()
	if lbl := m.bulkLabel(); lbl != "" {
		m.setStatus(lbl+" - s/x/r/e/d/R act on all marked", false)
	} else {
		m.setStatus("unmarked", false)
	}
	m.buildRows()
	return m, nil, true
}

// bulkNoun renders "N unit(s)" for confirmation prompts.
func bulkNoun(units []quadlet.Unit) string {
	if len(units) == 1 {
		return "1 unit"
	}
	return fmt.Sprintf("%d units", len(units))
}

// maxBulkErrs bounds how many per-unit failures the bulk error carries;
// the remainder is folded into a "+N more" count.
const maxBulkErrs = 5

// firstLine keeps the head of a multi-line command error for one-line logs.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// bulkCmd runs verb on every marked unit sequentially, reporting each result
// into the status line and the recent-actions log. The list refreshes once
// at the end. Failures carry each unit's own reason, not just its name.
func (m Model) bulkCmd(verb string, units []quadlet.Unit) tea.Cmd {
	sys := m.sys
	names := make([]string, 0, len(units))
	for _, u := range units {
		names = append(names, u.UnitName)
	}
	return func() tea.Msg {
		ctx := context.Background()
		var failed []string
		for _, name := range names {
			if _, err := sys.UnitAction(ctx, verb, name); err != nil {
				failed = append(failed, name+": "+firstLine(err.Error()))
			}
		}
		desc := fmt.Sprintf("bulk %s %d unit(s)", verb, len(names))
		if len(failed) > 0 {
			detail := failed
			more := ""
			if len(failed) > maxBulkErrs {
				detail = failed[:maxBulkErrs]
				more = fmt.Sprintf("; +%d more", len(failed)-maxBulkErrs)
			}
			return bulkMsg{desc: desc, done: len(names) - len(failed), total: len(names),
				err: fmt.Errorf("failed: %s%s", strings.Join(detail, "; "), more)}
		}
		return bulkMsg{desc: desc, done: len(names), total: len(names)}
	}
}
