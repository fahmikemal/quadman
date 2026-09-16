package ui

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kemal-labs/quadman/internal/config"
	"github.com/kemal-labs/quadman/internal/quadlet"
	"github.com/kemal-labs/quadman/internal/shellwords"
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

// customCmd expands cc.Run as a Go template over the selected unit, splits
// it quote-aware into argv, and runs it without a shell.
func customCmd(cc config.CustomCommand, u quadlet.Unit, images map[string]string) tea.Cmd {
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
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- argv from the user's own config file, no shell
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

// expandCustom renders run as a Go template with data, then splits the
// result quote-aware into argv.
func expandCustom(run string, data customData) ([]string, error) {
	tmpl, err := template.New("custom").Parse(run)
	if err != nil {
		return nil, fmt.Errorf("bad template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template failed: %w", err)
	}
	return shellwords.Split(buf.String())
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
	if m.refuseReadonly() {
		return m, nil, true
	}
	images := m.images
	return m, tea.Batch(m.setBusy("custom "+cc.Name), customCmd(*cc, u, images)), true
}
