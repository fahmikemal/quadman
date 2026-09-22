package podlet

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePodlet puts a recording fake podlet binary first on PATH.
func fakePodlet(t *testing.T, record *string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "podlet")
	script := "#!/bin/sh\necho \"$@\" > " + filepath.Join(dir, "argv") + "\ncat\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	*record = dir
}

func readArgv(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "argv"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestGenerateObject(t *testing.T) {
	var dir string
	fakePodlet(t, &dir)
	out, err := GenerateObject(context.Background(), "container", "web")
	if err != nil {
		t.Fatal(err)
	}
	_ = out
	if got := readArgv(t, dir); got != "generate container web" {
		t.Errorf("argv = %q", got)
	}
}

func TestGenerateObjectRejects(t *testing.T) {
	for _, tc := range [][2]string{
		{"service", "web"},
		{"container", ""},
		{"container", "a b"},
		{"container", "web;rm"},
		{"container", "$(web)"},
		{"container", "`web`"},
	} {
		if _, err := GenerateObject(context.Background(), tc[0], tc[1]); err == nil {
			t.Errorf("GenerateObject(%q, %q) should fail", tc[0], tc[1])
		}
	}
}

func TestIsObjectKind(t *testing.T) {
	for _, k := range []string{"container", "pod", "network", "volume", "image"} {
		if !IsObjectKind(k) {
			t.Errorf("%q should be convertible", k)
		}
	}
	if IsObjectKind("service") || IsObjectKind("") {
		t.Error("unknown kinds must be rejected")
	}
}
