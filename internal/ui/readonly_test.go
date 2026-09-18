package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
)

func readonlyModel() Model {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.readonly = true
	return m
}

func TestReadonlyRefusesWrites(t *testing.T) {
	for _, key := range []string{"s", "r", "x", "e", "d", "E", "h", "i", "D", "R", "L", "n", "A"} {
		m := readonlyModel()
		// A key is not in the model for these cases.
		if key == "A" {
			model, _ := m.Update(tea.KeyPressMsg{Code: 'A', Text: "A"})
			mm := model.(Model)
			if mm.mode == modeRecent {
				// A only opens a read screen; allowed even in readonly.
				continue
			}
		}
		model, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		mm := model.(Model)
		if mm.busy || mm.pending != nil || mm.generating || mm.instancing {
			t.Errorf("key %q must not start work in readonly mode", key)
		}
		if !strings.Contains(mm.statusLine, "readonly") {
			t.Errorf("key %q must explain readonly mode, got %q", key, mm.statusLine)
		}
	}
}

func TestReadonlyAllowsReads(t *testing.T) {
	m := readonlyModel()
	for _, key := range []string{"/", "?", "v", "t", "g", "w", "y"} {
		model, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		mm := model.(Model)
		if strings.Contains(mm.statusLine, "readonly mode") {
			t.Errorf("read key %q must not be refused: %q", key, mm.statusLine)
		}
	}
}

func TestReadonlyBannerInLegend(t *testing.T) {
	m := readonlyModel()
	if !strings.Contains(m.helpBar(), "readonly") {
		t.Errorf("legend must show readonly state: %q", m.helpBar())
	}
}

func TestExpandCustom(t *testing.T) {
	argv, err := expandCustom("echo {{.UnitName}} {{.Image}}", customData{Name: "web", UnitName: "web.service", Kind: "container", Image: "nginx"})
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 3 || argv[0] != "echo" || argv[1] != "web.service" || argv[2] != "nginx" {
		t.Errorf("argv = %q", argv)
	}
	if _, err := expandCustom("echo {{.Nope}", customData{}); err == nil {
		t.Error("bad template must fail")
	}
	if _, err := expandCustom(`echo "unclosed`, customData{}); err == nil {
		t.Error("unclosed quote must fail")
	}
}

func TestRunCustomDispatch(t *testing.T) {
	m := withUnits(New(), "webapp")
	m.table.SetCursor(0)
	m.custom = []config.CustomCommand{{Name: "demo", Key: "C", Run: "echo {{.UnitName}}"}}

	model, cmd := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	mm := model.(Model)
	if !mm.busy {
		t.Fatal("custom key must start the busy action")
	}
	if cmd == nil {
		t.Fatal("custom key must return a command")
	}

	// Unknown keys fall through to the table, not to custom.
	model, _ = m.Update(tea.KeyPressMsg{Code: 'Z', Text: "Z"})
	if model.(Model).busy {
		t.Error("unconfigured key must not start a custom action")
	}
}

func TestRunCustomReadonly(t *testing.T) {
	m := readonlyModel()
	m.custom = []config.CustomCommand{{Name: "demo", Key: "C", Run: "echo hi"}}
	model, _ := m.Update(tea.KeyPressMsg{Code: 'C', Text: "C"})
	mm := model.(Model)
	if mm.busy {
		t.Error("custom commands must not run in readonly mode")
	}
	if !strings.Contains(mm.statusLine, "readonly") {
		t.Errorf("must explain readonly, got %q", mm.statusLine)
	}
}

func TestCustomMsgRecorded(t *testing.T) {
	m := New()
	model, _ := m.Update(customMsg{desc: "custom demo", out: "hi"})
	mm := model.(Model)
	if len(mm.actions) != 1 || !mm.actions[0].ok {
		t.Fatalf("custom result must be logged: %+v", mm.actions)
	}
	if !strings.Contains(mm.statusLine, "custom demo ok") {
		t.Errorf("status = %q", mm.statusLine)
	}
}
