package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

func keyMsg(s string) tea.KeyPressMsg {
	var code rune
	if len(s) == 1 {
		code = rune(s[0])
	}
	return tea.KeyPressMsg{Code: code, Text: s}
}

func TestApplyExtraDirsDedups(t *testing.T) {
	old := quadlet.ExtraDirs
	quadlet.ExtraDirs = nil
	t.Cleanup(func() { quadlet.ExtraDirs = old })

	applyExtraDirs([]string{"/a", "", "/b", "/a"})
	applyExtraDirs([]string{"/b", "/c"})
	if len(quadlet.ExtraDirs) != 3 {
		t.Fatalf("dirs = %v, want [/a /b /c]", quadlet.ExtraDirs)
	}
}

func TestExternalUnitMarked(t *testing.T) {
	old := quadlet.ExtraDirs
	extra := t.TempDir()
	quadlet.ExtraDirs = []string{extra}
	t.Cleanup(func() { quadlet.ExtraDirs = old })

	if err := os.WriteFile(filepath.Join(extra, "mine.container"), []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isExternalDir(extra) {
		t.Fatal("extra dir must count as external")
	}
	cfg := t.TempDir()
	if !isExternalDir(filepath.Join(cfg, "other")) == false {
		_ = cfg
	}
	// A standard path is not external.
	std := quadlet.SearchDirs()[0]
	if isExternalDir(std) {
		t.Errorf("%q must not be external", std)
	}
}

func TestExternalMarkerInRows(t *testing.T) {
	old := quadlet.ExtraDirs
	extra := t.TempDir()
	quadlet.ExtraDirs = []string{extra}
	t.Cleanup(func() { quadlet.ExtraDirs = old })

	path := filepath.Join(extra, "mine.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	units := []quadlet.Unit{{Name: "mine", Kind: quadlet.KindContainer, Path: path, UnitName: "mine.service"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{"nginx"}, lingerOK: true})
	rows := model.(Model).table.Rows()
	if len(rows) != 1 || !strings.Contains(rows[0][0], "mine~") {
		t.Fatalf("external rows must carry the ~ marker, got %v", rows)
	}
}

func TestExternalStartWarns(t *testing.T) {
	old := quadlet.ExtraDirs
	extra := t.TempDir()
	quadlet.ExtraDirs = []string{extra}
	t.Cleanup(func() { quadlet.ExtraDirs = old })

	path := filepath.Join(extra, "mine.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New()
	units := []quadlet.Unit{{Name: "mine", Kind: quadlet.KindContainer, Path: path, UnitName: "mine.service"}}
	model, _ := m.Update(refreshMsg{units: units, images: []string{"nginx"}, lingerOK: true})
	mm := model.(Model)
	mm.table.SetCursor(0)
	model, _ = mm.Update(keyMsg("s"))
	mm = model.(Model)
	if mm.busy {
		t.Fatal("start on an external unit must not run (generator cannot see it)")
	}
	if !strings.Contains(mm.statusLine, "external") {
		t.Fatalf("must explain external, got %q", mm.statusLine)
	}
}
