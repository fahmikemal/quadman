package quadlet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBootTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	original := "[Container]\nImage=nginx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureBootTarget(path, "default.target")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("first call must change the file")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "[Install]") || !strings.Contains(string(data), "WantedBy=default.target") {
		t.Errorf("file missing [Install] section:\n%s", data)
	}
	if !strings.Contains(string(data), "Image=nginx") {
		t.Errorf("original content must be preserved:\n%s", data)
	}

	// Second call must be a no-op.
	changed, err = EnsureBootTarget(path, "default.target")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("already-enabled file must not be rewritten")
	}

	// BootTarget sees the declaration.
	f, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.BootTarget() != "default.target" {
		t.Errorf("BootTarget = %q", f.BootTarget())
	}
}

func TestRemoveBootTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	original := `[Unit]
Description=Web

[Container]
Image=nginx

[Install]
WantedBy=default.target
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := RemoveBootTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("file with [Install] must change")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "[Install]") || strings.Contains(string(data), "WantedBy") {
		t.Errorf("[Install] section must be gone:\n%s", data)
	}
	if !strings.Contains(string(data), "Image=nginx") || !strings.Contains(string(data), "Description=Web") {
		t.Errorf("other sections must be preserved:\n%s", data)
	}

	// Idempotent.
	changed, err = RemoveBootTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("second removal must be a no-op")
	}
}

func TestEnsureBootTargetRespectsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	original := "[Container]\nImage=nginx\n\n[Install]\nWantedBy=custom.target\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureBootTarget(path, "default.target")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("existing custom WantedBy must be respected, not overwritten")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Errorf("file must be untouched:\n%s", data)
	}
}
