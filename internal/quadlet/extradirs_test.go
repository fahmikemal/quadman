package quadlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtraDirsAppended(t *testing.T) {
	old := ExtraDirs
	ExtraDirs = nil
	t.Cleanup(func() { ExtraDirs = old })

	extra := t.TempDir()
	if err := os.WriteFile(filepath.Join(extra, "mine.container"), []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ExtraDirs = []string{extra}

	dirs := SearchDirs()
	if dirs[len(dirs)-1] != extra {
		t.Errorf("extra dir must be last, got %v", dirs)
	}
	units, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range units {
		if u.Name == "mine" && u.Path == filepath.Join(extra, "mine.container") {
			found = true
		}
	}
	if !found {
		t.Errorf("Discover must include the extra dir unit, got %+v", units)
	}
}

func TestExtraDirsShadowedByStandard(t *testing.T) {
	old := ExtraDirs
	ExtraDirs = nil
	t.Cleanup(func() { ExtraDirs = old })

	std := t.TempDir()
	extra := t.TempDir()
	content := "[Container]\nImage=nginx\n"
	if err := os.WriteFile(filepath.Join(std, "dup.container"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "dup.container"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	ExtraDirs = []string{extra}

	units, err := DiscoverDirs(append([]string{std}, SearchDirs()...))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, u := range units {
		if u.Name == "dup" {
			count++
			if u.Path != filepath.Join(std, "dup.container") {
				t.Errorf("standard path must win, got %s", u.Path)
			}
		}
	}
	if count != 1 {
		t.Errorf("dup must appear once, got %d", count)
	}
}
