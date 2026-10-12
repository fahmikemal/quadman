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

func TestWriteAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	payloads := []string{"[Container]\nImage=a\n", "[Container]\nImage=bb\n", "[Container]\nImage=ccc\n"}
	done := make(chan error, len(payloads)*4)
	for range 4 {
		for _, p := range payloads {
			go func() { done <- writeAtomic(path, []byte(p)) }()
		}
	}
	for range len(payloads) * 4 {
		if err := <-done; err != nil {
			t.Fatalf("writeAtomic: %v", err)
		}
	}
	data, _ := os.ReadFile(path)
	complete := false
	for _, p := range payloads {
		if string(data) == p {
			complete = true
		}
	}
	if !complete {
		t.Errorf("file holds a torn write: %q", data)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestEnsureBootTargetExistingInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\n\n[Install]\nAlias=www.service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := EnsureBootTarget(path, "multi-user.target"); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "[Install]") != 1 {
		t.Errorf("the existing [Install] section must gain the key, not get a twin:\n%s", data)
	}
	f, _ := Parse(path)
	if f.BootTarget() != "multi-user.target" || f.Section("Install").Get("Alias") != "www.service" {
		t.Errorf("install = %+v\n%s", f.Section("Install"), data)
	}
}

func TestEnsureBootTargetRespectsRequiredBy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.container")
	original := "[Container]\nImage=nginx\n\n[Install]\nRequiredBy=app.target\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, _ := EnsureBootTarget(path, "default.target"); changed {
		t.Error("RequiredBy= already starts the unit at boot")
	}
}

func TestRemoveBootTargetKeepsOtherInstallKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.container")
	content := "[Container]\nImage=nginx\n\n[Install]\nWantedBy=default.target \\\n  multi-user.target\nAlias=www.service\nRequiredBy=app.target\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := RemoveBootTarget(path); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if strings.Contains(got, "WantedBy") || strings.Contains(got, "RequiredBy") || strings.Contains(got, "multi-user.target") {
		t.Errorf("boot keys (and their continuation lines) must go:\n%s", got)
	}
	if !strings.Contains(got, "[Install]\nAlias=www.service") {
		t.Errorf("[Install] with Alias= must stay:\n%s", got)
	}
}

func TestBootSourceDropin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := Unit{Name: "web", Kind: KindContainer, Path: path}
	if got := BootSource(u); got != "" {
		t.Errorf("not enabled: BootSource = %q", got)
	}
	dropDir := filepath.Join(dir, "web.container.d")
	if err := os.Mkdir(dropDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dropDir, "boot.conf")
	if err := os.WriteFile(conf, []byte("[Install]\nWantedBy=default.target\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BootSource(u); got != conf {
		t.Errorf("BootSource = %q, want the drop-in %q", got, conf)
	}
}

// TestWriteAtomicFollowsSymlink: editing a symlinked quadlet (e.g. into a
// git checkout) must update the link's target and keep the link.
func TestWriteAtomicFollowsSymlink(t *testing.T) {
	repo := t.TempDir()
	real := filepath.Join(repo, "web.container")
	if err := os.WriteFile(real, []byte("[Container]\nImage=nginx\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "web.container")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if changed, err := EnsureBootTarget(link, "default.target"); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink must survive the write: %v %v", fi, err)
	}
	data, _ := os.ReadFile(real)
	if !strings.Contains(string(data), "WantedBy=default.target") {
		t.Errorf("the link target must hold the edit:\n%s", data)
	}
	if st, _ := os.Stat(real); st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 kept", st.Mode().Perm())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(repo, ".tmp-*")); len(leftovers) != 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}

func TestWriteFileAtomicNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.container")
	if err := WriteFileAtomic(path, []byte("[Container]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("new file mode = %v, %v", st, err)
	}
}
