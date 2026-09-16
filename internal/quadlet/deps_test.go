package quadlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeps(t *testing.T) {
	dir := t.TempDir()
	content := `[Unit]
Requires=db.service cache-volume.service
After=proxy.service external.service

[Container]
Image=base.image
Network=net0.network
Network=other.container
Volume=data.volume:/var/lib/data
Volume=plainname:/other
Pod=stack.pod
`
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	deps := Deps(Unit{Name: "webapp", Kind: KindContainer, Path: path}, f)

	want := map[string]bool{
		"db": true, "cache": true, "proxy": true, "external": true,
		"base": true, "net0": true, "other": true, "data": true, "stack": true,
	}
	if len(deps) != len(want) {
		t.Fatalf("deps = %v, want %d entries", deps, len(want))
	}
	for _, d := range deps {
		if !want[d] {
			t.Errorf("unexpected dep %q in %v", d, deps)
		}
		delete(want, d)
	}
	if len(want) > 0 {
		t.Errorf("missing deps: %v (got %v)", want, deps)
	}
}

func TestDepsSkipsSelfAndDupes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nNetwork=webapp.container\nNetwork=webapp.container\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := Parse(path)
	if deps := Deps(Unit{Name: "webapp", Kind: KindContainer, Path: path}, f); len(deps) != 0 {
		t.Errorf("self-references must be skipped, got %v", deps)
	}
}

func TestUnitToQuadletName(t *testing.T) {
	cases := map[string]string{
		"webapp.service":        "webapp",
		"stack-pod.service":     "stack",
		"data-volume.service":   "data",
		"net0-network.service":  "net0",
		"base-image.service":    "base",
		"builder-build.service": "builder",
		"pkg-artifact.service":  "pkg",
		"notaservice":           "",
		"foo.timer":             "",
	}
	for in, want := range cases {
		if got := unitToQuadletName(in); got != want {
			t.Errorf("unitToQuadletName(%q) = %q, want %q", in, got, want)
		}
	}
}
