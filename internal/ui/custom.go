package ui

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
	"github.com/fahmikemal/quadman/internal/quadlet"
	"github.com/fahmikemal/quadman/internal/remote"
	"github.com/fahmikemal/quadman/internal/shellwords"
)

// customTimeout bounds a custom command run.
const customTimeout = 30 * time.Second

type customMsg struct {
	desc string
	out  string
	err  error
}

// customData is the template data for custom command args.
type customData struct {
	Name     string
	UnitName string
	Kind     string
	Image    string
}

func customDataOf(u quadlet.Unit, images map[string]string) customData {
	return customData{
		Name:     u.Name,
		UnitName: u.UnitName,
		Kind:     string(u.Kind),
		Image:    images[u.UnitName],
	}
}

// builtinKeys are the single-char list-mode keys owned by quadman's own
// handlers. A custom command bound to one of them never fires (built-ins
// dispatch first), so New() warns instead of letting the config rot
// silently. TestBuiltinKeysCoverDispatch derives the set from the real
// dispatch, so it cannot go stale.
var builtinKeys = func() map[string]bool {
	keys := map[string]bool{" ": true}
	for _, k := range strings.Fields("/ ? A d D e E g h i I l L n o P q r R s S t T K u v w x X y Y z") {
		keys[k] = true
	}
	return keys
}()

// tableKeys are the single-character keys the unit table handles itself
// (bubbles' default table keymap: j/k, b/f, u/d, g/G). Custom commands
// dispatch before the table, so a custom command on one of them works but
// takes that navigation key away; New() says so.
var tableKeys = func() map[string]bool {
	km := table.DefaultKeyMap()
	keys := map[string]bool{}
	for _, b := range []key.Binding{km.LineUp, km.LineDown, km.PageUp, km.PageDown, km.HalfPageUp, km.HalfPageDown, km.GotoTop, km.GotoBottom} {
		for _, k := range b.Keys() {
			if len([]rune(k)) == 1 {
				keys[k] = true
			}
		}
	}
	return keys
}()

// customNavOverrides returns "name (key)" labels for custom commands that
// take a table navigation key away (they fire; the navigation does not).
func customNavOverrides(custom []config.CustomCommand) []string {
	var out []string
	for _, cc := range custom {
		if tableKeys[cc.Key] && !builtinKeys[cc.Key] {
			out = append(out, fmt.Sprintf("%s (%s)", cc.Name, cc.Key))
		}
	}
	return out
}

// customKeyConflicts returns "name (key)" labels for custom commands whose
// key is shadowed by a built-in binding.
func customKeyConflicts(custom []config.CustomCommand) []string {
	var out []string
	for _, cc := range custom {
		if builtinKeys[cc.Key] {
			out = append(out, fmt.Sprintf("%s (%s)", cc.Name, cc.Key))
		}
	}
	return out
}

// customCmd splits cc.Run quote-aware into argv, expands each word as a Go
// template over the selected unit, and runs it without a shell through
// the session runner — so a custom command acts on the session's host and
// user (SSH target, compartment) exactly like the built-in actions.
func customCmd(runner remote.Runner, cc config.CustomCommand, u quadlet.Unit, images map[string]string) tea.Cmd {
	return func() tea.Msg {
		desc := "custom " + cc.Name
		argv, err := expandCustom(cc.Run, customDataOf(u, images))
		if err != nil {
			return customMsg{desc: desc, err: err}
		}
		if len(argv) == 0 {
			return customMsg{desc: desc, err: fmt.Errorf("empty command")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), customTimeout)
		defer cancel()
		cmd := runner.Command(ctx, argv[0], argv[1:]...) // #nosec G204 -- argv from the user's own config file, no shell
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			out := strings.TrimSpace(stdout.String())
			if se := strings.TrimSpace(stderr.String()); se != "" {
				out = strings.TrimSpace(out + "\n" + se)
			}
			return customMsg{desc: desc, out: out, err: fmt.Errorf("%s: %w", strings.Join(argv, " "), err)}
		}
		return customMsg{desc: desc, out: strings.TrimSpace(stdout.String())}
	}
}

// templateAction matches one {{ … }} action, spaces inside included.
var templateAction = regexp.MustCompile(`\{\{.*?\}\}`)

// expandCustom splits run quote-aware into words first, then renders each
// word as its own Go template. Splitting before expanding means a value
// from a unit file (an Image= with spaces or quotes) always stays inside
// the one argument it was written in and can never add arguments. Template
// actions are shielded from the split, so `{{ .UnitName }}` with spaces
// stays one action.
func expandCustom(run string, data customData) ([]string, error) {
	var actions []string
	masked := templateAction.ReplaceAllStringFunc(run, func(a string) string {
		actions = append(actions, a)
		return fmt.Sprintf("\x00%d\x00", len(actions)-1)
	})
	words, err := shellwords.Split(masked)
	if err != nil {
		return nil, err
	}
	argv := make([]string, 0, len(words))
	for _, w := range words {
		for i, a := range actions {
			w = strings.ReplaceAll(w, fmt.Sprintf("\x00%d\x00", i), a)
		}
		tmpl, err := template.New("custom").Parse(w)
		if err != nil {
			return nil, fmt.Errorf("bad template: %w", err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("template failed: %w", err)
		}
		argv = append(argv, buf.String())
	}
	return argv, nil
}

// runCustom looks up the custom command for key and runs it on the selected
// unit. It returns handled=false when key is not a configured custom key.
func (m Model) runCustom(key string) (tea.Model, tea.Cmd, bool) {
	var cc *config.CustomCommand
	for i := range m.custom {
		if m.custom[i].Key == key {
			cc = &m.custom[i]
			break
		}
	}
	if cc == nil {
		return m, nil, false
	}
	u, ok := m.selected()
	if !ok {
		return m, nil, true
	}
	if m.refuse(capMutate) {
		return m, nil, true
	}
	images := m.images
	return m, tea.Batch(m.setBusy("custom "+cc.Name), customCmd(m.ssh, *cc, u, images)), true
}
