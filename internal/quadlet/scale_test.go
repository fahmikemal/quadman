package quadlet

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writeMany creates n container units in dir for scale benchmarks.
func writeMany(b *testing.B, dir string, n int) []string {
	b.Helper()
	dirs := []string{dir}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("svc-%04d.container", i)
		content := fmt.Sprintf("[Container]\nImage=registry.example/app:%d\nExec=/app --id %d\n", i, i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	return dirs
}

// BenchmarkDiscover500 measures scanning 500 unit files.
func BenchmarkDiscover500(b *testing.B) {
	dir := b.TempDir()
	writeMany(b, dir, 500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		units, err := DiscoverDirs([]string{dir})
		if err != nil {
			b.Fatal(err)
		}
		if len(units) != 500 {
			b.Fatalf("units = %d, want 500", len(units))
		}
	}
}

// BenchmarkInspectCold500 measures first-parse resolution of 500 units.
func BenchmarkInspectCold500(b *testing.B) {
	dir := b.TempDir()
	dirs := writeMany(b, dir, 500)
	units, err := DiscoverDirs(dirs)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var c InspectCache
		infos, images := c.InspectAll(units)
		if len(infos) != 500 || len(images) != 500 {
			b.Fatal("short result")
		}
	}
}

// BenchmarkInspectWarm500 measures the cached path: files unchanged, so no
// re-parse happens. This is the steady-state cost of every 2.5s poll.
func BenchmarkInspectWarm500(b *testing.B) {
	dir := b.TempDir()
	dirs := writeMany(b, dir, 500)
	units, err := DiscoverDirs(dirs)
	if err != nil {
		b.Fatal(err)
	}
	var c InspectCache
	c.InspectAll(units)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		infos, images := c.InspectAll(units)
		if len(infos) != 500 || len(images) != 500 {
			b.Fatal("short result")
		}
	}
}
