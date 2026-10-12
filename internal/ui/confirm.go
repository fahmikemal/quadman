package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Confirmation modal (DESIGN.md "Confirmations"): every armed action
// opens a dialog over the screen that says what WILL happen and what it
// KEEPS. Focus starts on Cancel. Bulk stop/restart use a warn border;
// delete uses an err border and stays disabled until the unit name is
// typed. y still confirms the plain dialogs, so the old y/N muscle memory
// keeps working.

// confirmState is the modal's focus and typed-name input.
type confirmUI struct {
	focus   int // 0 = Cancel, 1 = Confirm
	typedIn textinput.Model
}

// arm decorates m.pending with the built-in wording for its verb and
// resets the modal; call sites only set verb and unit.
func (m *Model) armPending() {
	p := m.pending
	if p == nil {
		return
	}
	m.pendingHow = m.trigger
	m.confirm.focus = 0
	if p.title == "" {
		m.describePending(p)
	}
	if p.typed != "" {
		in := textinput.New()
		in.Prompt = ""
		in.Focus()
		m.confirm.typedIn = in
	}
}

// describePending fills title, lines, keeps and danger for built-ins.
func (m Model) describePending(p *pendingAction) {
	u := p.unit
	file := filepath.Base(u.Path)
	switch {
	case strings.HasPrefix(p.verb, "bulk-"):
		verb := strings.TrimPrefix(p.verb, "bulk-")
		marked := m.markedUnits()
		p.title = fmt.Sprintf("%s %d unit%s?", capitalize(verb), len(marked), plural(len(marked)))
		var names []string
		for i, mu := range marked {
			if i == 6 {
				names = append(names, fmt.Sprintf("+%d more", len(marked)-6))
				break
			}
			names = append(names, mu.UnitName)
		}
		p.lines = []string{verb + " " + strings.Join(names, ", ")}
		if verb != "start" {
			p.lines = append(p.lines, "containers are removed and recreated (quadlet runs them with --rm)")
		}
		if deps := m.dependents(marked); len(deps) > 0 {
			p.lines = append(p.lines, "units that depend on them may stop or fail: "+strings.Join(deps, ", "))
		}
		p.keeps = []string{"quadlet files", "volumes and their data", "images"}
	case p.verb == "stop":
		p.title = "Stop " + u.UnitName + "?"
		p.lines = []string{"stop " + u.UnitName}
		if u.Kind == quadlet.KindContainer {
			p.lines = append(p.lines, "remove its container (quadlet runs it with --rm)")
		}
		if deps := m.dependents([]quadlet.Unit{u}); len(deps) > 0 {
			p.lines = append(p.lines, "units that depend on it may stop or fail: "+strings.Join(deps, ", "))
		}
		p.keeps = []string{file, "volumes and their data", "images"}
	case p.verb == "enable":
		p.title = "Enable " + u.UnitName + " at boot?"
		p.lines = []string{"add [Install] WantedBy=" + m.bootTarget() + " to " + file, "reload the generator and start it now"}
		p.keeps = []string{"everything else in " + file}
	case p.verb == "disable":
		p.title = "Disable " + u.UnitName + " at boot?"
		p.lines = []string{"remove the [Install] section from " + file, "reload the generator"}
		p.keeps = []string{"the unit running right now"}
	case p.verb == "install":
		p.title = "Install bundle " + file + "?"
		p.lines = []string{"podman quadlet install " + u.Path, "reload the generator; the new units appear in the list"}
		p.keeps = []string{"existing quadlet files with other names"}
	case p.verb == "prune":
		p.title = "Prune podman storage?"
		p.lines = []string{"remove stopped containers, unused networks, dangling images and build cache"}
		if inactive := m.inactiveQuadletUnits(); len(inactive) > 0 {
			p.lines = append(p.lines, "inactive quadlet units lose their stopped containers: "+strings.Join(inactive, ", "))
		}
		p.keeps = []string{"volumes", "images that are in use or tagged", "quadlet files"}
		p.danger = true
		p.typed = "prune"
	case p.verb == "delete":
		p.title = "Delete " + file + "?"
		p.lines = []string{"stop " + u.UnitName, "delete " + u.Path, "reload the generator"}
		p.keeps = []string{"volumes and their data", "images", "secrets", "drop-in directories"}
		p.danger = true
		p.typed = u.Name
	default:
		p.title = capitalize(p.verb) + "?"
	}
}

// dependents lists quadlet units whose files reference any of us.
func (m Model) dependents(us []quadlet.Unit) []string {
	names := map[string]bool{}
	for _, u := range us {
		names[u.Name] = true
	}
	var out []string
	for _, other := range m.units {
		if names[other.Name] {
			continue
		}
		f, err := m.parseUnitFile(other)
		if err != nil {
			continue
		}
		for _, d := range quadlet.Deps(other, f) {
			if names[d] {
				out = append(out, other.Name)
				break
			}
		}
	}
	return out
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// confirmReady reports whether Confirm is enabled (typed name matches).
func (m Model) confirmReady() bool {
	p := m.pending
	return p != nil && (p.typed == "" || strings.TrimSpace(m.confirm.typedIn.Value()) == p.typed)
}

// confirmKeys owns the keyboard while a confirmation is armed.
func (m Model) confirmKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.pending
	cancel := func() (tea.Model, tea.Cmd) {
		m.pending = nil
		m.recordCancelled(p.verb+" "+pendingTarget(p), m.pendingHow)
		m.setStatus(p.verb+" cancelled", false)
		return m, nil
	}
	run := func() (tea.Model, tea.Cmd) {
		m.pending = nil
		m.trigger = strings.TrimSpace(m.pendingHow + " · confirmed")
		return m.runPending(p)
	}
	k := msg.String()
	switch k {
	case "esc", "ctrl+c":
		return cancel()
	case "tab", "shift+tab", "left", "right":
		m.confirm.focus = 1 - m.confirm.focus
		return m, nil
	case "enter":
		if m.confirm.focus == 1 || p.typed != "" {
			if !m.confirmReady() {
				m.setStatus("type "+p.typed+" to enable "+confirmLabel(p), true)
				return m, nil
			}
			return run()
		}
		return cancel()
	}
	if p.typed != "" {
		var cmd tea.Cmd
		m.confirm.typedIn, cmd = m.confirm.typedIn.Update(msg)
		if m.confirmReady() {
			m.confirm.focus = 1
		}
		return m, cmd
	}
	switch k {
	case "y", "Y":
		return run()
	case "h":
		m.confirm.focus = 0
		return m, nil
	case "l":
		m.confirm.focus = 1
		return m, nil
	}
	// Any other key declines, like the old y/N prompt.
	return cancel()
}

func pendingTarget(p *pendingAction) string {
	if p.unit.UnitName != "" {
		return p.unit.UnitName
	}
	return strings.TrimSuffix(strings.TrimPrefix(p.title, capitalize(p.verb)+" "), "?")
}

// confirmView renders the modal box.
func (m Model) confirmView(w int) string {
	p := m.pending
	boxW := min(72, w-4)
	inner := boxW - 4
	border := activePalette.warn
	titleStyle := warnStyle.Bold(true)
	if p.danger {
		border = activePalette.err
		titleStyle = errStyle.Bold(true)
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(ansi.Truncate(p.title, inner, "…")))
	b.WriteString("\n")
	if len(p.lines) > 0 {
		b.WriteString("\n" + dimStyle.Bold(true).Render("THIS WILL") + "\n")
		for _, l := range p.lines {
			b.WriteString(wrapBullet("•", l, inner, fgStyle))
		}
	}
	if len(p.keeps) > 0 {
		b.WriteString("\n" + dimStyle.Bold(true).Render("KEEPS") + "\n")
		for _, l := range p.keeps {
			b.WriteString(wrapBullet("•", l, inner, mutedStyle))
		}
	}
	if p.typed != "" {
		b.WriteString("\n" + mutedStyle.Render("type ") + brightStyle.Render(p.typed) + mutedStyle.Render(" to confirm") + "\n")
		b.WriteString(accentStyle.Render("› ") + m.confirm.typedIn.View() + "\n")
	}
	b.WriteString("\n")
	cancelBtn, okBtn := " Cancel ", " "+confirmLabel(p)+" "
	btn := func(label string, focused, enabled bool) string {
		switch {
		case !enabled:
			return dimStyle.Render("[" + label + "]")
		case focused && label == okBtn && p.danger:
			return lipgloss.NewStyle().Foreground(lipgloss.Color(activePalette.onAccent)).Background(lipgloss.Color(activePalette.err)).Bold(true).Render(label)
		case focused:
			return tabActiveStyle.Render(label)
		}
		return fgStyle.Render("[" + strings.TrimSpace(label) + "]")
	}
	buttons := btn(cancelBtn, m.confirm.focus == 0, true) + "  " + btn(okBtn, m.confirm.focus == 1, m.confirmReady())
	hint := "y/N  ←/→ choose  ↵ select"
	if p.typed != "" {
		hint = "↵ confirm when the name matches · esc cancel"
	}
	b.WriteString(buttons + "   " + dimStyle.Render(hint))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		Padding(0, 1).
		Width(boxW).
		Render(b.String())
}

func confirmLabel(p *pendingAction) string {
	if p.verb == "delete" {
		return "Delete"
	}
	if strings.HasPrefix(p.verb, "bulk-") {
		return capitalize(strings.TrimPrefix(p.verb, "bulk-"))
	}
	return capitalize(strings.Fields(p.verb)[0])
}

// wrapBullet word-wraps "• text" to width with a hanging indent.
func wrapBullet(bullet, text string, width int, st lipgloss.Style) string {
	words := strings.Fields(text)
	var b strings.Builder
	line := bullet
	indent := strings.Repeat(" ", ansi.StringWidth(bullet))
	for _, w := range words {
		if ansi.StringWidth(line)+1+ansi.StringWidth(w) > width && line != bullet {
			b.WriteString(st.Render(line) + "\n")
			line = indent
		}
		line += " " + w
	}
	b.WriteString(st.Render(line) + "\n")
	return b.String()
}

// overlay composites box over frame, centered in a w×h screen.
func overlay(frame, box string, w, h int) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x := max((w-bw)/2, 0)
	y := max((h-bh)/2, 1)
	base := lipgloss.NewLayer(lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(frame))
	return lipgloss.NewCompositor(base, lipgloss.NewLayer(box).X(x).Y(y).Z(1)).Render()
}
