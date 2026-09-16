package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestResolveTheme(t *testing.T) {
	for in, want := range map[string]Theme{
		"":           ThemeDark,
		"nope":       ThemeDark,
		"dark":       ThemeDark,
		"light":      ThemeLight,
		"colorblind": ThemeColorblind,
		"auto":       ThemeAuto,
	} {
		if got := resolveTheme(in); got != want {
			t.Errorf("resolveTheme(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyThemeSwitches(t *testing.T) {
	defer applyTheme(ThemeDark)
	applyTheme(ThemeDark)
	darkOK := okStyle.GetForeground()
	applyTheme(ThemeColorblind)
	if activeTheme != ThemeColorblind {
		t.Fatalf("active = %q, want colorblind", activeTheme)
	}
	if okStyle.GetForeground() == darkOK {
		t.Error("colorblind ok color must differ from dark")
	}
	applyTheme(ThemeLight)
	if activeTheme != ThemeLight {
		t.Fatalf("active = %q, want light", activeTheme)
	}
}

func TestThemeFromConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	sub := filepath.Join(dir, "quadman")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "config.yaml"), []byte("theme: colorblind\nmouse: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer applyTheme(ThemeDark)
	m := New()
	if activeTheme != ThemeColorblind {
		t.Errorf("theme = %q, want colorblind from YAML", activeTheme)
	}
	if !m.mouse {
		t.Error("mouse from YAML must apply")
	}
}

func TestMouseDefaultsOff(t *testing.T) {
	m := New()
	if m.mouse {
		t.Error("mouse must default off (opt-in)")
	}
	v := m.View()
	if v.MouseMode != 0 {
		t.Errorf("mouse mode must be unset by default, got %v", v.MouseMode)
	}
	m.mouse = true
	if m.View().MouseMode == 0 {
		t.Error("mouse mode must be set when opted in")
	}
}

func TestMouseClickIgnoredWhenOff(t *testing.T) {
	m := withUnits(New(), "webapp", "db")
	m.table.SetCursor(0)
	model, _ := m.Update(tea.MouseClickMsg{X: 1, Y: 3})
	if model.(Model).table.Cursor() != 0 {
		t.Error("clicks must be ignored with mouse off")
	}
}

func TestHelpMentionsTheme(t *testing.T) {
	defer applyTheme(ThemeDark)
	applyTheme(ThemeColorblind)
	if !strings.Contains(activeThemeMessage(), "colorblind") {
		t.Error("theme message must name the scheme")
	}
}

func activeThemeMessage() string {
	return "theme: " + string(activeTheme)
}
