package quadlet

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	for _, sub := range []string{"common", "1001", "1000"} {
		if err := os.MkdirAll(filepath.Join(users, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dirs := userDirs(base, "1000")
	want := []string{
		users,
		filepath.Join(users, "common"), // non-numeric subdir is shared
		filepath.Join(users, "1000"),   // own UID dir last
	}
	if len(dirs) != len(want) {
		t.Fatalf("userDirs = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("userDirs[%d] = %q, want %q (numeric dirs of other users must be skipped)", i, dirs[i], want[i])
		}
	}
}

func TestUserDirsMissing(t *testing.T) {
	base := t.TempDir()
	dirs := userDirs(base, "1000")
	want := []string{
		filepath.Join(base, "users"),
		filepath.Join(base, "users", "1000"),
	}
	if len(dirs) != 2 || dirs[0] != want[0] || dirs[1] != want[1] {
		t.Errorf("userDirs(missing base) = %v, want %v", dirs, want)
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

func TestStaleUnits(t *testing.T) {
	src := t.TempDir()
	gen := t.TempDir()
	write := func(dir, name string, mtime int64) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, time.Unix(mtime, 0), time.Unix(mtime, 0)); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// fresh: generated unit newer than source; stale: generated older;
	// missing: no generated unit at all.
	write(src, "fresh.container", 1000)
	write(gen, "fresh.service", 2000)
	write(src, "stale.container", 3000)
	write(gen, "stale.service", 1000)
	write(src, "missing.container", 1000)

	units := []Unit{
		{Name: "fresh", Kind: KindContainer, Path: filepath.Join(src, "fresh.container"), UnitName: "fresh.service"},
		{Name: "stale", Kind: KindContainer, Path: filepath.Join(src, "stale.container"), UnitName: "stale.service"},
		{Name: "missing", Kind: KindContainer, Path: filepath.Join(src, "missing.container"), UnitName: "missing.service"},
	}

	stale := StaleUnits(units, gen)
	if len(stale) != 2 {
		t.Fatalf("stale = %+v, want stale+missing", stale)
	}
	for _, u := range stale {
		if u.Name == "fresh" {
			t.Errorf("fresh unit must not be stale: %+v", u)
		}
	}
	if got := StaleUnits(units, ""); got != nil {
		t.Errorf("empty generator dir must report nothing, got %+v", got)
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
