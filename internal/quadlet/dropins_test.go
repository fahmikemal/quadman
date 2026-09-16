package quadlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDropins(t *testing.T) {
	dir := t.TempDir()
	mk := func(rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// unit file itself must exist for a realistic Path
	mk("web-api.container")
	mk("container.d/00-generic.conf")
	mk("container.d/zz-later.conf")
	mk("web-.container.d/10-prefix.conf")
	mk("web-api.container.d/20-second.conf")
	mk("web-api.container.d/10-first.conf")
	mk("web-api.container.d/ignore.txt") // not a .conf, must be skipped
	mk("other.container.d/99-other.conf")

	u := Unit{Name: "web-api", Kind: KindContainer, Path: filepath.Join(dir, "web-api.container")}
	drops := Dropins(u)

	var got []string
	for _, d := range drops {
		got = append(got, d.Dir+"/"+filepath.Base(d.Path))
	}
	want := []string{
		"container.d/00-generic.conf",
		"container.d/zz-later.conf",
		"web-.container.d/10-prefix.conf",
		"web-api.container.d/10-first.conf",
		"web-api.container.d/20-second.conf",
	}
	if len(got) != len(want) {
		t.Fatalf("dropins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dropins[%d] = %q, want %q (order: generic, cascade, own dir, alphabetical)", i, got[i], want[i])
		}
	}
}

func TestDropinsNone(t *testing.T) {
	dir := t.TempDir()
	u := Unit{Name: "plain", Kind: KindContainer, Path: filepath.Join(dir, "plain.container")}
	if got := Dropins(u); len(got) != 0 {
		t.Errorf("no drop-in dirs should yield none, got %v", got)
	}
}

func TestDropinsSimpleName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "web.container.d", "10-a.conf")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := Unit{Name: "web", Kind: KindContainer, Path: filepath.Join(dir, "web.container")}
	got := Dropins(u)
	if len(got) != 1 {
		t.Fatalf("simple name should only read its own dir, got %v", got)
	}
}
