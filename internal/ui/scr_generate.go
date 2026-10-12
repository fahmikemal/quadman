package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/fahmikemal/quadman/internal/podlet"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Generate screen (n): podlet's output as one tab per file, a one-line
// check row (generator dry-run, port and name clashes), boot/start
// toggles, and the target directory. Nothing is written until y.

// genFile is one generated quadlet file.
type genFile struct {
	name    string // base name with extension, e.g. web.container
	content string
}

// splitGenerated cuts podlet output into files: podlet separates files
// with a line holding only "---" and names each with "# FileName=".
func splitGenerated(out string) ([]genFile, error) {
	var files []genFile
	var cur []string
	flush := func() error {
		text := strings.TrimSpace(strings.Join(cur, "\n"))
		cur = nil
		if text == "" {
			return nil
		}
		name, err := generatedFileName(podlet.FileNameOf(text))
		if err != nil {
			return err
		}
		files = append(files, genFile{name: name, content: text + "\n"})
		return nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "---" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		cur = append(cur, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("podlet produced no quadlet file")
	}
	return files, nil
}

// quadletTargetDir is where generated files are written: the first
// writable directory of the generator's own search path for the session's
// mode — ~/.config/containers/systemd for rootless users ($XDG_CONFIG_HOME
// aware), /etc/containers/systemd for system units. Never an extra dir:
// the generator would not see the files there.
func quadletTargetDir(system bool) (string, error) {
	if system {
		return "/etc/containers/systemd", nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "containers", "systemd"), nil
}

// genCheck is one item of the check row.
type genCheck struct {
	g    glyphKind
	text string
}

type generateMsg struct {
	input   string
	content string
	files   []genFile
	checks  []genCheck
	err     error
}

// generateCmd runs podlet, splits its output, and checks the files with
// the generator itself (local sessions only: the generator runs locally).
func (m Model) generateCmd(input string) tea.Cmd {
	system := m.system
	searchDirs := m.searchDirs()
	isolated := m.ssh.Isolated()
	pl := m.pl
	return func() tea.Msg {
		ctx := context.Background()
		out, err := generatePodlet(ctx, pl, strings.TrimSpace(input))
		if err != nil {
			return generateMsg{err: err}
		}
		files, err := splitGenerated(out)
		if err != nil {
			return generateMsg{err: err}
		}
		msg := generateMsg{input: input, content: out, files: files}
		if !isolated {
			msg.checks = append(msg.checks, dryRunCheck(ctx, files, searchDirs, system))
		}
		return msg
	}
}

// dryRunCheck validates the generated files next to the existing search
// path, so references to existing .volume/.network units resolve.
func dryRunCheck(ctx context.Context, files []genFile, searchDirs []string, system bool) genCheck {
	if quadlet.GeneratorBinary() == "" {
		return genCheck{gNotFound, "dry-run skipped (no generator)"}
	}
	tmp, err := os.MkdirTemp("", "quadman-gen-")
	if err != nil {
		return genCheck{gWarn, "dry-run skipped: " + err.Error()}
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	names := map[string]bool{}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmp, f.name), []byte(f.content), 0o600); err != nil {
			return genCheck{gWarn, "dry-run skipped: " + err.Error()}
		}
		names[f.name] = true
	}
	issues, err := quadlet.ValidateMode(ctx, append([]string{tmp}, searchDirs...), system)
	if err != nil {
		return genCheck{gWarn, "dry-run failed: " + err.Error()}
	}
	for _, is := range issues {
		if names[is.File] && is.Severity == quadlet.SeverityError {
			return genCheck{gFailed, is.File + ": " + is.Message}
		}
	}
	return genCheck{gDone, "dry-run ok"}
}

// hostPorts extracts host ports from PublishPort= values: "8080:80",
// "127.0.0.1:8080:80/tcp" → 8080; container-only "80" publishes nothing.
func hostPorts(values []string) []string {
	var out []string
	for _, v := range values {
		v, _, _ = strings.Cut(v, "/")
		parts := strings.Split(v, ":")
		if len(parts) >= 2 && parts[len(parts)-2] != "" {
			out = append(out, parts[len(parts)-2])
		}
	}
	return out
}

var publishPortRe = regexp.MustCompile(`(?m)^\s*PublishPort\s*=\s*(\S+)`)

// localChecks compares the files with what is already installed: a host
// port another unit publishes, or a file name that already exists.
func (m Model) localChecks(files []genFile) []genCheck {
	used := map[string]string{}
	for _, u := range m.units {
		if u.Kind != quadlet.KindContainer {
			continue
		}
		f, err := m.parseUnitFile(u)
		if err != nil {
			continue
		}
		for _, p := range hostPorts(f.Section("Container").GetAll("PublishPort")) {
			used[p] = u.Name
		}
	}
	var checks []genCheck
	dir, _ := quadletTargetDir(m.system)
	for _, f := range files {
		var vals []string
		for _, mt := range publishPortRe.FindAllStringSubmatch(f.content, -1) {
			vals = append(vals, mt[1])
		}
		for _, p := range hostPorts(vals) {
			if owner, ok := used[p]; ok && owner+".container" != f.name {
				checks = append(checks, genCheck{gWarn, "port " + p + " used by " + owner})
			}
		}
		if dir != "" {
			if _, err := os.Stat(filepath.Join(dir, f.name)); err == nil {
				checks = append(checks, genCheck{gWarn, f.name + " exists — y replaces it"})
			}
		}
	}
	return checks
}

// applyGenerate opens the preview.
func (m Model) applyGenerate(msg generateMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.setStatus("generate: "+msg.err.Error(), true)
		return m, nil
	}
	m.genContent = msg.content
	m.genFiles = msg.files
	m.genInput = msg.input
	m.genTab = 0
	m.genChecks = append(msg.checks, m.localChecks(msg.files)...)
	m.genBoot = strings.Contains(msg.content, "[Install]")
	m.genStart = false
	m.mode = modeGenerate
	m.showGenFile()
	m.resize()
	return m, nil
}

func (m *Model) showGenFile() {
	if m.genTab < len(m.genFiles) {
		m.viewport.SetContent(m.genFiles[m.genTab].content)
		m.viewport.GotoTop()
	}
}

// generateView renders the preview body around the viewport.
func (m Model) generateView(w int) string {
	var b strings.Builder
	b.WriteString(accentStyle.Render("› ") + mutedStyle.Render(ansi.Truncate(m.genInput, w-4, "…")))
	b.WriteString("\n")
	for i, f := range m.genFiles {
		label := " + " + f.name + " "
		if i == m.genTab {
			b.WriteString(tabActiveStyle.Render(label))
		} else {
			b.WriteString(fgStyle.Render(label))
		}
	}
	b.WriteString("  " + dimStyle.Render(fmt.Sprintf("%d file%s", len(m.genFiles), plural(len(m.genFiles)))))
	if len(m.genFiles) > 1 {
		b.WriteString("  " + keyHint("tab", "next"))
	}
	b.WriteString("\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	var checks []string
	for _, c := range m.genChecks {
		checks = append(checks, glyphStyle(c.g).Render(glyph(c.g)+" "+c.text))
	}
	box := func(on bool, label string) string {
		if on {
			return accentStyle.Render("☑ " + label)
		}
		return mutedStyle.Render("☐ " + label)
	}
	checks = append(checks, box(m.genBoot, "boot")+" "+box(m.genStart, "start"))
	b.WriteString(ansi.Truncate(" "+strings.Join(checks, "   "), w, "…"))
	b.WriteString("\n")
	dir, _ := quadletTargetDir(m.system)
	b.WriteString(ansi.Truncate(" "+mutedStyle.Render("→ ")+fgStyle.Render(dir+"/")+"   "+dimStyle.Render("nothing written yet"), w, "…"))
	return b.String()
}

// withBoot adds [Install] WantedBy=<target> when the file has no
// [Install] section of its own.
func withBoot(content, target string) string {
	if strings.Contains(content, "[Install]") {
		return content
	}
	return strings.TrimRight(content, "\n") + "\n\n[Install]\nWantedBy=" + target + "\n"
}

// generatedUnitName is the unit the generator makes from a generated file,
// honoring a ServiceName= the file declares.
func generatedUnitName(f genFile) (string, quadlet.Kind) {
	ext := filepath.Ext(f.name)
	u := quadlet.Unit{Name: strings.TrimSuffix(f.name, ext), Kind: quadlet.Kind(strings.TrimPrefix(ext, "."))}
	parsed, _ := quadlet.ParseBytes(f.name, []byte(f.content))
	return quadlet.InfoOf(u, parsed).UnitName, u.Kind
}

// installGenerated writes every generated file into the session's quadlet
// directory (atomically: a crash never leaves half a unit file), reloads
// the generator, and optionally starts the container units.
func (m Model) installGenerated() (tea.Model, tea.Cmd) {
	if len(m.genFiles) == 0 {
		m.setStatus("nothing to write", true)
		return m, nil
	}
	dir, err := quadletTargetDir(m.system)
	if err != nil {
		m.setStatus("generate: "+err.Error(), true)
		return m, nil
	}
	files := make([]genFile, len(m.genFiles))
	copy(files, m.genFiles)
	if m.genBoot {
		target := m.bootTarget()
		for i := range files {
			if ext := filepath.Ext(files[i].name); ext == ".container" || ext == ".pod" || ext == ".kube" {
				files[i].content = withBoot(files[i].content, target)
			}
		}
	}
	start := m.genStart
	sys := m.sys
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.name)
	}
	desc := "generate " + strings.Join(names, ", ")
	m.mode = modeList
	m.resize()
	return m, tea.Batch(m.setBusy("write "+strings.Join(names, ", ")),
		m.actionCmdHint(desc, "written to "+dir+" and reloaded", func(ctx context.Context) (string, error) {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return "", err
			}
			for _, f := range files {
				if err := quadlet.WriteFileAtomic(filepath.Join(dir, f.name), []byte(f.content), 0o644); err != nil { // #nosec G306 -- quadlet files are user config, not secrets
					return "", err
				}
			}
			if _, err := sys.DaemonReload(ctx); err != nil {
				return "", err
			}
			if start {
				for _, f := range files {
					unit, kind := generatedUnitName(f)
					if kind != quadlet.KindContainer {
						continue
					}
					if _, err := sys.UnitAction(ctx, "start", unit); err != nil {
						return "", fmt.Errorf("written and reloaded, but start %s failed: %w", unit, err)
					}
				}
			}
			return dir, nil
		}))
}

// modeGenerateKeys handles the preview.
func (m Model) modeGenerateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if m.mode != modeGenerate {
		return m, nil, false
	}
	switch msg.String() {
	case "esc", "q":
		m.mode = modeList
		m.setStatus("generate cancelled — nothing written", false)
		m.resize()
		return m, nil, true
	case "y":
		if m.refuse(capMutate | capTargetFiles) {
			return m, nil, true
		}
		mm, cmd := m.installGenerated()
		return mm, cmd, true
	case "b":
		m.genBoot = !m.genBoot
		return m, nil, true
	case "space":
		m.genStart = !m.genStart
		return m, nil, true
	case "tab", "]":
		if len(m.genFiles) > 0 {
			m.genTab = (m.genTab + 1) % len(m.genFiles)
			m.showGenFile()
		}
		return m, nil, true
	case "shift+tab", "[":
		if len(m.genFiles) > 0 {
			m.genTab = (m.genTab + len(m.genFiles) - 1) % len(m.genFiles)
			m.showGenFile()
		}
		return m, nil, true
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd, true
}

// normalizeRunArgs rewrites podman's -d / --detach in the leading option
// run to podlet's --detach=true (podlet 0.3 requires the value; services
// always run detached anyway). Arguments after the first non-option word
// belong to the image and its command and are never touched.
func normalizeRunArgs(args []string) []string {
	out := make([]string, 0, len(args)+1)
	i := 0
	for ; i < len(args) && (args[i] == "podman" || args[i] == "docker" || args[i] == "run"); i++ {
		out = append(out, args[i])
	}
	for ; i < len(args) && strings.HasPrefix(args[i], "-"); i++ {
		a := args[i]
		switch {
		case a == "-d" || a == "--detach":
			out = append(out, "--detach=true")
		case len(a) > 2 && a[1] != '-' && !strings.Contains(a, "=") && strings.Contains(a, "d") && regexp.MustCompile(`^-[a-zA-Z]+$`).MatchString(a):
			out = append(out, "--detach=true")
			if rest := strings.ReplaceAll(a[1:], "d", ""); rest != "" {
				out = append(out, "-"+rest)
			}
		default:
			out = append(out, a)
		}
	}
	return append(out, args[i:]...)
}
