package quadlet

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectCacheHit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := Unit{Name: "webapp", Kind: KindContainer, Path: path}

	var c InspectCache
	first := c.Inspect(u)
	if first.Image != "nginx:1" {
		t.Fatalf("image = %q, want nginx:1", first.Image)
	}

	// Rewrite with the same content: the cache must still hold one entry.
	second := c.Inspect(u)
	if second.Image != "nginx:1" {
		t.Fatalf("cached image = %q, want nginx:1", second.Image)
	}
	if len(c.entries) != 1 {
		t.Fatalf("entries = %d, want 1 (cache hit, no re-parse growth)", len(c.entries))
	}
}

func TestInspectCacheInvalidatesOnChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	u := Unit{Name: "webapp", Kind: KindContainer, Path: path}

	var c InspectCache
	if got := c.Inspect(u).Image; got != "nginx:1" {
		t.Fatalf("image = %q, want nginx:1", got)
	}

	// Bump the mtime so the change is visible even on coarse filesystems.
	past := time.Now().Add(-2 * time.Second)
	_ = os.Chtimes(path, past, past)
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx:2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := c.Inspect(u).Image; got != "nginx:2" {
		t.Fatalf("after edit image = %q, want nginx:2", got)
	}
}

func TestInspectAllKeepsOrder(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) Unit {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		base := name[:len(name)-len(".container")]
		return Unit{Name: base, Kind: KindContainer, Path: p, UnitName: base + ".service"}
	}
	units := []Unit{
		write("aaa.container", "[Container]\nImage=img-a\n"),
		write("bbb.container", "[Container]\nImage=img-b\n"),
	}
	var c InspectCache
	infos, images := c.InspectAll(units)
	if len(infos) != 2 || len(images) != 2 {
		t.Fatalf("lens = %d/%d, want 2/2", len(infos), len(images))
	}
	if images[0] != "img-a" || images[1] != "img-b" {
		t.Errorf("images = %v, want [img-a img-b]", images)
	}
	if units[0].UnitName != "aaa.service" || units[1].UnitName != "bbb.service" {
		t.Errorf("units not updated in place: %+v", units)
	}
}
