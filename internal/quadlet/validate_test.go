package quadlet

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	if GeneratorBinary() == "" {
		t.Skip("no podman-system-generator on this host")
	}
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("good.container", "[Container]\nImage=docker.io/library/busybox:latest\nExec=sleep 10\n")
	write("bad.container", "[Container]\nImage=docker.io/library/busybox:latest\nBadKey=123\n")

	issues, err := Validate(context.Background(), []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	var badErr, goodClean bool
	for _, is := range issues {
		if is.File == "bad.container" && is.Severity == SeverityError {
			badErr = true
			if is.Message == "" {
				t.Error("error issue should carry the generator message")
			}
			if is.Path == "" {
				t.Error("error issue should carry the file path")
			}
		}
		if is.File == "good.container" && is.Severity == SeverityError {
			t.Errorf("good file must not produce errors, got %+v", is)
		}
	}
	if !badErr {
		t.Errorf("bad.container must produce an error issue, got %+v", issues)
	}
	_ = goodClean
}

func TestValidateNoDirs(t *testing.T) {
	issues, err := Validate(context.Background(), nil)
	if err != nil || len(issues) != 0 {
		t.Errorf("no dirs = nil, no issues, got %v %v", issues, err)
	}
}

func TestStripGeneratorPrefix(t *testing.T) {
	if got := stripGeneratorPrefix("quadlet-generator[123]: Warning: x y"); got != "Warning: x y" {
		t.Errorf("strip = %q", got)
	}
	if got := stripGeneratorPrefix("plain line"); got != "plain line" {
		t.Errorf("plain = %q", got)
	}
}

func TestValidateModeSystem(t *testing.T) {
	if GeneratorBinary() == "" {
		t.Skip("no podman-system-generator on this host")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.container")
	if err := os.WriteFile(p, []byte("[Container]\nImage=alpine\nBadKey=xyz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues, err := ValidateMode(context.Background(), []string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, is := range issues {
		if is.File == "bad.container" && is.Severity == SeverityError {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error issue for bad.container in system mode, got: %+v", issues)
	}
}
