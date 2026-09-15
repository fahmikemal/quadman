package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Editor != "" {
		t.Errorf("missing file should yield zero config, got %+v", c)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := (Config{Editor: "nano"}).Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "quadman", "config.json")); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Editor != "nano" {
		t.Errorf("Editor = %q, want nano", c.Editor)
	}
}

func TestLoadCorrupt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	sub := filepath.Join(dir, "quadman")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "config.json"), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Error("corrupt JSON should return an error")
	}
}
