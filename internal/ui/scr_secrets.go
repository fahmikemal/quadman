package ui

import (
	"bufio"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/podman"
	"github.com/fahmikemal/quadman/internal/quadlet"
)

// Secrets screen (K): which unit uses which secret and how (env or file).
// Values are never rendered: they are typed into a masked input, travel
// to podman on stdin, and a copy to the clipboard is cleared after 30s.

// secretUse is one Secret= reference in a quadlet file.
type secretUse struct {
	unit   string // generated unit name
	name   string // quadlet base name
	as     string // "env DB_PASS", "file /run/secrets/jwt", "file"
	file   string // source file base name
	line   int
	secret string
}

type secretsData struct {
	list     []podman.SecretEntry
	uses     map[string][]secretUse // by secret name
	loadedAt time.Time
}

type secretsMsg struct {
	data secretsData
	err  error
}

func (s secretsMsg) screenMode() mode        { return modeSecrets }
func (s secretsMsg) screenErrPrefix() string { return "secrets" }
func (s secretsMsg) screenErr() error        { return s.err }
func (s secretsMsg) apply(m *Model)          { m.secrets = s.data }

// secretClipboardTTL is how long a copied secret value stays on the
// clipboard before quadman overwrites it.
const secretClipboardTTL = 30 * time.Second

type clipboardClearMsg struct{ secret string }

func (m Model) secretsCmd() tea.Cmd {
	units := slices.Clone(m.units)
	read := m.readUnitFile
	return func() tea.Msg {
		list, err := podman.SecretList(context.Background())
		if err != nil {
			return secretsMsg{err: err}
		}
		return secretsMsg{data: secretsData{list: list, uses: secretUses(units, read), loadedAt: time.Now()}}
	}
}

// secretUses scans every container and kube file for Secret= lines,
// keeping the line number so a missing secret points at its reference.
func secretUses(units []quadlet.Unit, read func(quadlet.Unit) ([]byte, error)) map[string][]secretUse {
	out := map[string][]secretUse{}
	for _, u := range units {
		if u.Kind != quadlet.KindContainer {
			continue
		}
		data, err := read(u)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(strings.NewReader(string(data)))
		section := ""
		for n := 1; sc.Scan(); n++ {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
				section = strings.ToLower(strings.Trim(line, "[]"))
				continue
			}
			val, ok := strings.CutPrefix(line, "Secret=")
			if !ok || section != "container" {
				continue
			}
			parts := strings.Split(val, ",")
			use := secretUse{unit: u.UnitName, name: u.Name, file: baseName(u.Path), line: n, secret: strings.TrimSpace(parts[0])}
			typ, target := "mount", ""
			for _, p := range parts[1:] {
				k, v, _ := strings.Cut(p, "=")
				switch strings.ToLower(strings.TrimSpace(k)) {
				case "type":
					typ = strings.TrimSpace(v)
				case "target":
					target = strings.TrimSpace(v)
				}
			}
			switch typ {
			case "env":
				if target == "" {
					target = use.secret
				}
				use.as = "env " + target
			default:
				use.as = "file"
				if target != "" {
					if !strings.HasPrefix(target, "/") {
						target = "/run/secrets/" + target
					}
					use.as += " " + target
				}
			}
			out[use.secret] = append(out[use.secret], use)
		}
	}
	return out
}

func (m Model) secretsSpec() screenSpec {
	d := m.secrets
	known := map[string]podman.SecretEntry{}
	drivers := map[string]bool{}
	for _, s := range d.list {
		known[s.Name] = s
		drivers[s.Driver] = true
	}
	var missing, unused, inUse []string
	for name := range d.uses {
		if _, ok := known[name]; !ok {
			missing = append(missing, name)
		}
	}
	unitSet := map[string]bool{}
	for _, s := range d.list {
		if uses := d.uses[s.Name]; len(uses) > 0 {
			inUse = append(inUse, s.Name)
			for _, u := range uses {
				unitSet[u.unit] = true
			}
		} else {
			unused = append(unused, s.Name)
		}
	}
	slices.Sort(missing)
	slices.Sort(unused)
	slices.Sort(inUse)
	driverNames := make([]string, 0, len(drivers))
	for k := range drivers {
		driverNames = append(driverNames, k)
	}
	slices.Sort(driverNames)

	tiles := []tile{
		{label: "SECRETS", value: fmt.Sprintf("%d", len(d.list)), note: "driver " + orDash(strings.Join(driverNames, ", ")), valueStyle: brightStyle},
		{label: "IN USE", value: fmt.Sprintf("%s %d", glyph(gActive), len(inUse)), note: fmt.Sprintf("%d unit%s", len(unitSet), plural(len(unitSet))), valueStyle: okStyle},
		{label: "UNUSED", value: fmt.Sprintf("%s %d", glyph(gWarn), len(unused)), note: firstOr(unused, "none"), valueStyle: warnStyle},
		{label: "MISSING", value: fmt.Sprintf("%s %d", glyph(gFailed), len(missing)), note: "referenced, not found", valueStyle: errStyle},
	}
	if len(unused) == 0 {
		tiles[2].valueStyle = mutedStyle
	} else {
		tiles[2].tint = activePalette.warn
	}
	if len(missing) == 0 {
		tiles[3].valueStyle = mutedStyle
		tiles[3].note = "every reference resolves"
	} else {
		tiles[3].tint = activePalette.err
	}

	s := screenSpec{
		tiles: tiles,
		cols: []scrCol{
			{title: "NAME", min: 14, max: 30},
			{title: "DRIVER", w: 7},
			{title: "CREATED", w: 13},
			{title: "UPDATED", w: 13},
			{title: "USED BY · AS", min: 16, hide: breakpointMid},
		},
		empty:  "no podman secrets — n creates one",
		detail: m.secretDetail,
		info:   "podman secret ls · values are never rendered",
	}
	if m.secretIn.active {
		s.toolbar = m.secretPromptLine()
	}
	for _, name := range missing {
		u := d.uses[name][0]
		s.rows = append(s.rows, scrRow{
			id: "missing:" + name, unit: u.unit,
			cells: []scrCell{
				cell(name, errStyle), cell("—", dimStyle), cell("—", dimStyle), cell("—", dimStyle),
				cell(fmt.Sprintf("%s missing · %s:%d Secret=%s", glyph(gFailed), u.file, u.line, name), errStyle),
			},
		})
	}
	for _, name := range unused {
		e := known[name]
		s.rows = append(s.rows, scrRow{
			id: "secret:" + name,
			cells: []scrCell{
				cell(name, fgStyle), cell(e.Driver, mutedStyle), cell(agoRFC3339(e.CreatedAt), mutedStyle),
				cell(agoRFC3339(e.UpdatedAt), mutedStyle), cell(glyph(gWarn)+" not referenced by any unit", warnStyle),
			},
		})
	}
	for _, name := range inUse {
		e := known[name]
		uses := d.uses[name]
		var parts []string
		for _, u := range uses {
			parts = append(parts, m.unitGlyph(u.unit)+" "+u.as)
		}
		s.rows = append(s.rows, scrRow{
			id: "secret:" + name, unit: uses[0].unit,
			cells: []scrCell{
				cell(name, fgStyle), cell(e.Driver, mutedStyle), cell(agoRFC3339(e.CreatedAt), mutedStyle),
				cell(agoRFC3339(e.UpdatedAt), mutedStyle), cell(strings.Join(parts, " · "), fgStyle),
			},
		})
	}
	return s
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	if len(xs) == 1 {
		return xs[0]
	}
	return fmt.Sprintf("%s +%d", xs[0], len(xs)-1)
}

func (m Model) secretDetail(r scrRow, _ int) []string {
	kind, name, _ := strings.Cut(r.id, ":")
	uses := m.secrets.uses[name]
	lines := []string{brightStyle.Render(name)}
	if kind == "missing" {
		lines = append(lines, errStyle.Render(glyph(gFailed)+" referenced but not in the secret store — those units fail to start"))
		for _, u := range uses {
			lines = append(lines, detailField("Referenced", fmt.Sprintf("%s:%d (%s)", u.file, u.line, u.as)))
		}
		lines = append(lines, keyHint("n", "create it")+"   "+keyHint("↵", "open unit"))
		return lines
	}
	var e podman.SecretEntry
	for _, s := range m.secrets.list {
		if s.Name == name {
			e = s
		}
	}
	lines = append(lines,
		detailField("Value", "••••••••••••••••"+mutedStyle.Render("  never shown · c copy to clipboard (clears in 30s)")),
		detailField("Rotate", fmt.Sprintf("r new value → restart %d dependent unit%s (asks first)", len(uses), plural(len(uses)))),
		detailField("ID", shortHex(e.ID)+" · driver "+e.Driver+" · updated "+agoRFC3339(e.UpdatedAt)),
	)
	for _, u := range uses {
		lines = append(lines, detailField("Used by", m.unitGlyph(u.unit)+" "+mutedStyle.Render(u.as+fmt.Sprintf(" · %s:%d", u.file, u.line))))
	}
	return lines
}

// secretInput is the two-step create / rotate prompt.
type secretInput struct {
	active bool
	rotate bool
	stage  int // 0 = name, 1 = value
	name   string
	in     textinput.Model
}

func (m Model) secretPromptLine() string {
	if m.secretIn.stage == 0 {
		return filterStyle.Render("new secret name: " + m.secretIn.in.View() + "  (enter / esc)")
	}
	verb := "value for " + m.secretIn.name
	if m.secretIn.rotate {
		verb = "new value for " + m.secretIn.name
	}
	return filterStyle.Render(verb+": "+m.secretIn.in.View()) + mutedStyle.Render("  hidden · sent on stdin · enter / esc")
}

func (m *Model) openSecretPrompt(rotate bool, name string) tea.Cmd {
	in := textinput.New()
	in.Prompt = ""
	m.secretIn = secretInput{active: true, rotate: rotate, name: name, in: in}
	if rotate {
		m.secretIn.stage = 1
		m.secretIn.in.EchoMode = textinput.EchoPassword
		m.secretIn.in.EchoCharacter = '•'
	}
	m.secretIn.in.Focus()
	m.resize()
	return textinput.Blink
}

// secretInputKeys owns the keyboard while the secret prompt is open.
func (m Model) secretInputKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	if !m.secretIn.active {
		return m, nil, false
	}
	switch msg.String() {
	case "esc":
		m.secretIn = secretInput{}
		m.setStatus("secret entry cancelled", false)
		m.resize()
		return m, nil, true
	case "enter":
		val := m.secretIn.in.Value()
		if m.secretIn.stage == 0 {
			name := strings.TrimSpace(val)
			if name == "" || strings.ContainsAny(name, " /\t") {
				m.setStatus("secret names cannot be empty or contain spaces or /", true)
				return m, nil, true
			}
			m.secretIn.name = name
			m.secretIn.stage = 1
			m.secretIn.in.SetValue("")
			m.secretIn.in.EchoMode = textinput.EchoPassword
			m.secretIn.in.EchoCharacter = '•'
			return m, nil, true
		}
		if val == "" {
			m.setStatus("empty value — type the secret, or esc to cancel", true)
			return m, nil, true
		}
		in := m.secretIn
		m.secretIn = secretInput{}
		m.resize()
		value := []byte(val)
		if !in.rotate {
			return m, tea.Batch(m.setBusy("create secret "+in.name), actionCmdHint("create secret "+in.name,
				"reference it with Secret="+in.name+" in a .container file",
				func(ctx context.Context) (string, error) {
					return "", podman.SecretCreate(ctx, in.name, value, false)
				}), m.secretsCmd()), true
		}
		return m.confirmRotate(in.name, value), nil, true
	}
	var cmd tea.Cmd
	m.secretIn.in, cmd = m.secretIn.in.Update(msg)
	return m, cmd, true
}

// confirmRotate arms the rotate confirmation: replace the value, then
// restart every running unit that uses it so they pick it up.
func (m Model) confirmRotate(name string, value []byte) Model {
	var restart []string
	for _, u := range m.secrets.uses[name] {
		if st := m.status[u.unit]; st.ActiveState == "active" && !slices.Contains(restart, u.unit) {
			restart = append(restart, u.unit)
		}
	}
	lines := []string{"replace the value of " + name}
	if len(restart) > 0 {
		lines = append(lines, "restart "+strings.Join(restart, ", ")+" so they read it")
	} else {
		lines = append(lines, "no running unit uses it: nothing restarts")
	}
	sys := m.sys
	m.pending = &pendingAction{
		verb:  "rotate",
		title: "Rotate secret " + name + "?",
		lines: lines,
		keeps: []string{"the secret's name and every Secret= reference"},
		run: func(m Model) (tea.Model, tea.Cmd) {
			return m, tea.Batch(m.setBusy("rotate secret "+name), actionCmdHint("rotate secret "+name, "dependent units restarted",
				func(ctx context.Context) (string, error) {
					if err := podman.SecretCreate(ctx, name, value, true); err != nil {
						return "", err
					}
					for _, u := range restart {
						if _, err := sys.UnitAction(ctx, "restart", u); err != nil {
							return "", fmt.Errorf("value replaced, but restart %s failed: %w", u, err)
						}
					}
					return "", nil
				}), m.secretsCmd())
		},
	}
	return m
}

// secretsKeysIn handles the secrets screen's own keys.
func (m Model) secretsKeysIn(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	k := msg.String()
	switch k {
	case "ctrl+r":
		return m, tea.Batch(m.setBusy("podman secrets"), m.secretsCmd()), true
	case "n":
		if m.refuseReadonly() {
			return m, nil, true
		}
		if r, ok := m.selectedRow(); ok && strings.HasPrefix(r.id, "missing:") {
			cmd := m.openSecretPrompt(false, "")
			m.secretIn.name = strings.TrimPrefix(r.id, "missing:")
			m.secretIn.stage = 1
			m.secretIn.in.EchoMode = textinput.EchoPassword
			m.secretIn.in.EchoCharacter = '•'
			return m, cmd, true
		}
		return m, m.openSecretPrompt(false, ""), true
	}
	r, ok := m.selectedRow()
	if !ok {
		return m, nil, false
	}
	kind, name, _ := strings.Cut(r.id, ":")
	switch k {
	case "r":
		if kind != "secret" || m.refuseReadonly() {
			return m, nil, true
		}
		return m, m.openSecretPrompt(true, name), true
	case "c":
		if kind != "secret" {
			return m, nil, true
		}
		if m.clientInfo != "" || m.readonly {
			m.setStatus("copying secret values is disabled in readonly and served sessions", true)
			return m, nil, true
		}
		return m, tea.Batch(m.setBusy("read "+name), func() tea.Msg {
			v, err := podman.SecretValue(context.Background(), name)
			return secretValueMsg{name: name, value: v, err: err}
		}), true
	case "D":
		if kind != "secret" || m.refuseReadonly() {
			return m, nil, true
		}
		uses := m.secrets.uses[name]
		lines := []string{"remove " + name + " from the secret store"}
		if len(uses) > 0 {
			var names []string
			for _, u := range uses {
				names = append(names, u.name)
			}
			lines = append(lines, "break "+strings.Join(names, ", ")+": they fail to start until it exists again")
		}
		m.pending = &pendingAction{
			verb:   "remove secret",
			title:  "Remove secret " + name + "?",
			lines:  lines,
			keeps:  []string{"running containers keep the value they already read", "the quadlet files"},
			danger: true,
			typed:  name,
			run: func(m Model) (tea.Model, tea.Cmd) {
				return m, tea.Batch(m.setBusy("remove secret "+name), actionCmd("remove secret "+name,
					func(ctx context.Context) (string, error) { return "", podman.SecretRemove(ctx, name) }), m.secretsCmd())
			},
		}
		return m, nil, true
	}
	return m, nil, false
}

type secretValueMsg struct {
	name  string
	value []byte
	err   error
}

// applySecretValue puts the value on the clipboard and schedules the wipe.
func (m Model) applySecretValue(msg secretValueMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.setStatus("read secret: "+msg.err.Error(), true)
		return m, nil
	}
	m.setStatus(fmt.Sprintf("copied the value of %s — the clipboard clears in %s", msg.name, secretClipboardTTL), false)
	name := msg.name
	return m, tea.Batch(tea.SetClipboard(string(msg.value)), tea.Tick(secretClipboardTTL, func(time.Time) tea.Msg {
		return clipboardClearMsg{secret: name}
	}))
}
