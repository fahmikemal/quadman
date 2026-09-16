package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePodletRejectsGarbage(t *testing.T) {
	ctx := context.Background()
	for _, in := range []string{
		"",
		"   ",
		"nginx:latest",
		"docker ps",
		"podman ps",
		`"unclosed quote`,
		"compose ",
	} {
		if _, err := generatePodlet(ctx, in); err == nil {
			t.Errorf("generatePodlet(%q) should fail validation", in)
		}
	}
}

func TestGeneratePodletRunShorthand(t *testing.T) {
	// `run ...` is shorthand for `podman run ...`: prove the routing by
	// pointing podlet at a fake binary that echoes its argv.
	dir := t.TempDir()
	fake := filepath.Join(dir, "podlet")
	script := "#!/bin/sh\necho \"FAKE-ARGS:$@\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	out, err := generatePodlet(context.Background(), "run --name web nginx")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "FAKE-ARGS:podman run --name web nginx") {
		t.Errorf("shorthand must prepend podman, got %q", out)
	}

	out, err = generatePodlet(context.Background(), `podman run --name "my web" nginx`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "FAKE-ARGS:podman run --name my web nginx") {
		t.Errorf("quoted value must survive as one word, got %q", out)
	}
}

func TestGeneratePodletComposeFile(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "podlet")
	script := "#!/bin/sh\necho \"FAKE-COMPOSE:$@\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	compose := filepath.Join(dir, "stack.yml")
	if err := os.WriteFile(compose, []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := generatePodlet(context.Background(), compose)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "FAKE-COMPOSE:compose -f") {
		t.Errorf("compose file must route to `podlet compose -f`, got %q", out)
	}
}
