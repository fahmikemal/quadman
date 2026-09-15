package ui

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editorChoice is one entry of the first-use editor picker.
type editorChoice struct {
	key string
	bin string
}

// editorCandidates are offered on first use, friendliest first.
var editorCandidates = []editorChoice{
	{key: "n", bin: "nano"},
	{key: "v", bin: "vim"},
	{key: "i", bin: "vi"},
}

// availableEditors filters the candidates to binaries present on PATH.
func availableEditors() []editorChoice {
	var out []editorChoice
	for _, c := range editorCandidates {
		if _, err := exec.LookPath(c.bin); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// editSelected opens the unit file in an editor. Resolution order: $EDITOR
// environment variable → the saved preference → a one-time picker whose
// answer is persisted to the config file.
func (m Model) editSelected() (tea.Model, tea.Cmd) {
	if _, ok := m.selected(); !ok {
		return m, nil
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return m.editWith(e)
	}
	if m.cfg.Editor != "" {
		return m.editWith(m.cfg.Editor)
	}
	choices := availableEditors()
	if len(choices) == 0 {
		return m.editWith("vi") // POSIX last resort
	}
	m.editorChoices = choices
	m.pickingEditor = true
	return m, nil
}

// pickEditor handles keys while the first-use picker is open.
func (m Model) pickEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.pickingEditor = false
		m.setStatus("edit cancelled", false)
		return m, nil
	}
	for _, c := range m.editorChoices {
		if msg.String() == c.key {
			m.pickingEditor = false
			m.cfg.Editor = c.bin
			if err := m.cfg.Save(); err != nil {
				m.setStatus("could not save editor preference ("+err.Error()+"), using "+c.bin+" this time", true)
			}
			return m.editWith(c.bin)
		}
	}
	return m, nil // ignore anything else while picking
}

// pickerLine renders the first-use picker prompt from the available editors.
func (m Model) pickerLine() string {
	var parts []string
	for _, c := range m.editorChoices {
		parts = append(parts, "["+c.key+"] "+c.bin)
	}
	return "Choose an editor: " + strings.Join(parts, "  ") + " - saved for next time (esc to cancel)"
}

// editWith opens the file in the given editor via tea.ExecProcess.
func (m Model) editWith(editor string) (tea.Model, tea.Cmd) {
	u, ok := m.selected()
	if !ok {
		return m, nil
	}
	before := fileMtime(u.Path)
	cmd := exec.Command(editor, u.Path) // #nosec G702 G204 -- the editor binary is the user's own $EDITOR or their saved picker choice; argv slice, no shell
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{changed: fileMtime(u.Path) != before, err: err}
	})
}
