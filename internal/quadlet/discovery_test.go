package quadlet

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDiscoverDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(dirA, "webapp.container")
	write(dirA, "db.container") // wins over dirB in lookup order
	if err := os.Mkdir(filepath.Join(dirA, "db.container.d"), 0o755); err != nil {
		t.Fatal(err) // drop-in directory must be skipped
	}
	write(dirA, "notes.txt") // unrelated file must be skipped
	write(dirB, "db.container")
	write(dirB, "cache.volume")
	write(dirB, "stack.pod")
	write(dirB, "net0.network")
	write(dirB, "base.image")
	write(dirB, "builder.build")
	write(dirB, "stack.kube")
	write(dirB, "web@.container") // template unit

	units, err := DiscoverDirs([]string{dirA, dirB})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]Unit{}
	for _, u := range units {
		got[u.Name] = u
	}
	if len(units) != 9 {
		t.Fatalf("got %d units, want 9: %+v", len(units), units)
	}
	if u := got["webapp"]; u.UnitName != "webapp.service" || u.Kind != KindContainer {
		t.Errorf("webapp = %+v", u)
	}
	if u := got["db"]; u.Path != filepath.Join(dirA, "db.container") {
		t.Errorf("db should resolve from dirA (lookup order), got %+v", u)
	}
	if u := got["stack"]; u.UnitName != "stack-pod.service" {
		t.Errorf("stack (pod) = %+v", u)
	}
	if u := got["net0"]; u.UnitName != "net0-network.service" {
		t.Errorf("net0 = %+v, want net0-network.service", u)
	}
	if u := got["cache"]; u.UnitName != "cache-volume.service" {
		t.Errorf("cache = %+v, want cache-volume.service", u)
	}
	if u := got["base"]; u.UnitName != "base-image.service" {
		t.Errorf("base = %+v, want base-image.service", u)
	}
	if u := got["builder"]; u.UnitName != "builder-build.service" {
		t.Errorf("builder = %+v, want builder-build.service", u)
	}
	kubeCount := 0
	for _, u := range units {
		if u.Kind == KindKube {
			kubeCount++
			if u.UnitName != "stack.service" {
				t.Errorf("stack (kube) = %+v, want stack.service", u)
			}
		}
	}
	if kubeCount != 1 {
		t.Errorf("kube units = %d, want 1", kubeCount)
	}
	if u := got["web@"]; u.UnitName != "web@.service" {
		t.Errorf("web@ = %+v, want web@.service", u)
	}
}

func TestUserDirs(t *testing.T) {
	base := t.TempDir()
	users := filepath.Join(base, "users")
	dirs := userDirs(base, "1000")
	if len(dirs) != 2 || dirs[0] != users || dirs[1] != filepath.Join(users, "1000") {
		t.Errorf("userDirs = %v, want shared users dir then the own UID dir", dirs)
	}
}

// TestDiscoverRecursive mirrors the generator: subdirectories are searched
// recursively, files of a dir come before its subdirs, drop-in dirs are
// skipped, and other users' numeric dirs below users/ are never read.
func TestDiscoverRecursive(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "config")
	users := filepath.Join(base, "containers", "systemd", "users")
	mk := func(rel, content string) {
		p := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("config/apps/web/web.container", "[Container]\nImage=a\n")
	mk("config/apps/db.container", "[Container]\nImage=db\n")
	mk("config/zz.container", "[Container]\nImage=top\n")
	mk("config/a/zz.container", "[Container]\nImage=nested\n")       // shadowed by the top-level file
	mk("config/web.container.d/override.container", "[Container]\n") // inside a drop-in dir: never a unit
	mk("containers/systemd/users/shared/common.container", "[Container]\n")
	mk("containers/systemd/users/1001/other.container", "[Container]\n") // someone else's
	mk("containers/systemd/users/1000/mine.container", "[Container]\n")

	units, err := DiscoverDirs([]string{cfg, users, filepath.Join(users, "1000")})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, u := range units {
		got[u.Name] = u.Path
	}
	want := map[string]string{
		"web":    filepath.Join(cfg, "apps", "web", "web.container"),
		"db":     filepath.Join(cfg, "apps", "db.container"),
		"zz":     filepath.Join(cfg, "zz.container"),
		"common": filepath.Join(users, "shared", "common.container"),
		"mine":   filepath.Join(users, "1000", "mine.container"),
	}
	if len(got) != len(want) {
		t.Fatalf("units = %v, want %v", got, want)
	}
	for name, p := range want {
		if got[name] != p {
			t.Errorf("%s = %q, want %q", name, got[name], p)
		}
	}
}

func TestDiscoverExtraDirSkipsHidden(t *testing.T) {
	old := ExtraDirs
	t.Cleanup(func() { ExtraDirs = old })
	repo := t.TempDir()
	ExtraDirs = []string{repo}
	for _, rel := range []string{"stack/web.container", ".git/objects/x.container"} {
		p := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("[Container]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	units, err := DiscoverDirs([]string{repo})
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Name != "web" {
		t.Errorf("extra dirs must skip hidden subtrees: %+v", units)
	}
}

func TestDiscoverSymlinks(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "sub", "web.container"), []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "linked.container"), []byte("[Container]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "linked.container"), filepath.Join(real, "linked.container")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "systemd")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	units, err := DiscoverDirs([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, u := range units {
		got[u.Name] = u.Path
	}
	if got["web"] != filepath.Join(link, "sub", "web.container") {
		t.Errorf("a symlinked search dir must be walked under its configured path, got %v", got)
	}
	if got["linked"] != filepath.Join(link, "linked.container") {
		t.Errorf("symlinked unit files must be discovered, got %v", got)
	}
}

func TestSearchDirs(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "xdg-config")
	rt := filepath.Join(home, "xdg-runtime")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_RUNTIME_DIR", rt)

	uid := strconv.Itoa(os.Getuid())
	dirs := SearchDirs()
	if len(dirs) < 4 {
		t.Fatalf("SearchDirs = %v, want at least 4 entries", dirs)
	}
	if dirs[0] != filepath.Join(rt, "containers", "systemd") {
		t.Errorf("dirs[0] = %q, want runtime dir first", dirs[0])
	}
	if dirs[1] != filepath.Join(cfg, "containers", "systemd") {
		t.Errorf("dirs[1] = %q, want config dir second", dirs[1])
	}
	var foundEtc, foundUID bool
	for _, d := range dirs {
		if d == "/etc/containers/systemd/users" {
			foundEtc = true
		}
		if d == filepath.Join("/etc/containers/systemd/users", uid) {
			foundUID = true
		}
	}
	if !foundEtc {
		t.Errorf("SearchDirs missing /etc/containers/systemd/users: %v", dirs)
	}
	if !foundUID {
		t.Errorf("SearchDirs missing /etc/containers/systemd/users/%s: %v", uid, dirs)
	}
	for _, d := range dirs {
		if d == "/etc/containers/systemd" {
			t.Errorf("SearchDirs must not include the system-only dir /etc/containers/systemd: %v", dirs)
		}
	}
}

func TestSearchDirsNoRuntimeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", "") // unset → runtime dir must be skipped
	t.Setenv("XDG_CONFIG_HOME", "")

	dirs := SearchDirs()
	want := filepath.Join(home, ".config", "containers", "systemd")
	if len(dirs) == 0 || dirs[0] != want {
		t.Errorf("dirs[0] = %q, want %q (config fallback when XDG vars are unset)", dirs, want)
	}
}

func TestSystemSearchDirs(t *testing.T) {
	dirs := SystemSearchDirs()
	want := []string{
		"/run/containers/systemd",
		"/etc/containers/systemd",
		"/usr/share/containers/systemd",
	}
	if len(dirs) < 3 {
		t.Fatalf("SystemSearchDirs = %v, want at least 3 entries", dirs)
	}
	for i, w := range want {
		if dirs[i] != w {
			t.Errorf("dirs[%d] = %q, want %q", i, dirs[i], w)
		}
	}
	// Verify that user-specific dirs are not in system dirs
	for _, d := range dirs {
		if strings.Contains(d, "/users") {
			t.Errorf("SystemSearchDirs must not include user-specific dirs: %s", d)
		}
	}
}

func TestDiscoverMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sysweb.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldExtra := ExtraDirs
	ExtraDirs = []string{dir}
	defer func() { ExtraDirs = oldExtra }()

	units, err := DiscoverMode(true)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, u := range units {
		if u.Name == "sysweb" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DiscoverMode(true) did not find sysweb unit in extra dirs: %v", units)
	}
}

func TestDiscoverDirsPermissionDenied(t *testing.T) {
	parent := t.TempDir()
	unreadable := filepath.Join(parent, "unreadable")
	if err := os.Mkdir(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(unreadable, 0o755) }()

	goodDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(goodDir, "good.container"), []byte("[Container]\nImage=alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should not return an error even if one dir is unreadable
	units, err := DiscoverDirs([]string{unreadable, goodDir})
	if err != nil {
		t.Fatalf("DiscoverDirs returned unexpected error on unreadable dir: %v", err)
	}
	if len(units) != 1 || units[0].Name != "good" {
		t.Errorf("expected 1 unit 'good', got %v", units)
	}
}

func TestSearchDirsFor(t *testing.T) {
	dirs := SearchDirsFor("/home/svc", "1001", "/run/user/1001")
	if len(dirs) < 2 {
		t.Fatalf("SearchDirsFor = %v", dirs)
	}
	if dirs[0] != "/run/user/1001/containers/systemd" {
		t.Errorf("dirs[0] = %q, want runtime dir first", dirs[0])
	}
	if dirs[1] != "/home/svc/.config/containers/systemd" {
		t.Errorf("dirs[1] = %q, want config dir second", dirs[1])
	}
	found := false
	for _, d := range dirs {
		if d == "/etc/containers/systemd/users/1001" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing UID dir: %v", dirs)
	}
}

func TestSearchDirsForNoRuntime(t *testing.T) {
	dirs := SearchDirsFor("/home/svc", "1001", "")
	if len(dirs) == 0 || dirs[0] != "/home/svc/.config/containers/systemd" {
		t.Errorf("without runtime, config dir first: %v", dirs)
	}
}
