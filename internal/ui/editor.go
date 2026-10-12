package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/shellwords"
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
	if _, ok := m.editPath(); !ok {
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
		m.editTarget = editTarget{}
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

// editorArgv builds the argv for opening path in editor. The editor value
// may carry its own arguments (`code --wait`), split quote-aware without a
// shell; the file path is always the final argument.
func editorArgv(editor, path string) ([]string, error) {
	return editorArgvAt(editor, path, 0)
}

// editorArgvAt is editorArgv that also jumps to line (> 0) for editors
// whose line syntax is known; any other editor just opens the file.
func editorArgvAt(editor, path string, line int) ([]string, error) {
	argv, err := shellwords.Split(editor)
	if err != nil {
		return nil, fmt.Errorf("bad $EDITOR %q: %w", editor, err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty $EDITOR")
	}
	if line <= 0 {
		return append(argv, path), nil
	}
	switch filepath.Base(argv[0]) {
	case "vi", "vim", "nvim", "nano", "emacs", "micro", "kak", "mg", "joe", "ne":
		return append(argv, fmt.Sprintf("+%d", line), path), nil
	case "hx", "helix":
		return append(argv, fmt.Sprintf("%s:%d", path, line)), nil
	case "code", "codium", "code-insiders":
		return append(argv, "--goto", fmt.Sprintf("%s:%d", path, line)), nil
	}
	return append(argv, path), nil
}

// editTarget is a file (and optional line) to edit instead of the
// selected unit's own file: timer files, a problem's exact line.
type editTarget struct {
	path string
	line int
}

// editPath resolves what the editor opens: the explicit target when set,
// else the selected unit's file.
func (m Model) editPath() (editTarget, bool) {
	if m.editTarget.path != "" {
		return m.editTarget, true
	}
	if u, ok := m.selected(); ok {
		return editTarget{path: u.Path}, true
	}
	return editTarget{}, false
}

// editPathAt opens path at line with the same gates as E on the list.
func (m Model) editPathAt(path string, line int) (tea.Model, tea.Cmd) {
	if m.refuse(capTargetFiles | capTerminal) {
		return m, nil
	}
	m.editTarget = editTarget{path: path, line: line}
	return m.editSelected()
}

// editWith opens the file in the given editor via tea.ExecProcess.
func (m Model) editWith(editor string) (tea.Model, tea.Cmd) {
	target, ok := m.editPath()
	if !ok {
		return m, nil
	}
	m.editTarget = editTarget{}
	argv, err := editorArgvAt(editor, target.path, target.line)
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	before := fileMtime(target.path)
	cmd := exec.Command(argv[0], argv[1:]...) // #nosec G702 G204 -- the editor binary is the user's own $EDITOR or their saved picker choice; argv slice, no shell
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{changed: fileMtime(target.path) != before, err: err}
	})
}
