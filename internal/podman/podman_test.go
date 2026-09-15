package podman

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakePodman puts a fake podman binary first on PATH.
func fakePodman(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "podman")
	script := "#!/bin/sh\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestQuadletList(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
  {
    "Name": "webapp.container",
    "UnitName": "webapp.service",
    "Path": "/home/alice/.config/containers/systemd/webapp.container",
    "Status": "Not loaded",
    "App": "web",
    "Pod": "stack-pod.service"
  }
]
EOF
`)
	entries, err := QuadletList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.UnitName != "webapp.service" || e.Status != "Not loaded" || e.App != "web" || e.Pod != "stack-pod.service" {
		t.Errorf("entry = %+v", e)
	}

	byUnit := ByUnit(entries)
	if byUnit["webapp.service"].App != "web" {
		t.Errorf("ByUnit lookup failed: %+v", byUnit)
	}
}

func TestQuadletListEmpty(t *testing.T) {
	fakePodman(t, "echo '[]'\n")
	entries, err := QuadletList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
}

func TestQuadletListCommandError(t *testing.T) {
	fakePodman(t, "echo 'unsupported' >&2\nexit 125\n")
	if _, err := QuadletList(context.Background()); err == nil {
		t.Fatal("QuadletList should fail when podman exits non-zero")
	}
}

func TestQuadletListBadJSON(t *testing.T) {
	fakePodman(t, "echo 'not json'\n")
	if _, err := QuadletList(context.Background()); err == nil {
		t.Fatal("QuadletList should fail on non-JSON output")
	}
}

func TestPsHealth(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
  {"Names": ["systemd-webapp"], "Status": "Up 2 minutes (healthy)"},
  {"Names": ["systemd-db"], "Status": "Up 5 minutes (unhealthy)"},
  {"Names": ["plain"], "Status": "Up 1 hour"}
]
EOF
`)
	m, err := PsHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m["systemd-webapp"] != "healthy" {
		t.Errorf("webapp health = %q", m["systemd-webapp"])
	}
	if m["systemd-db"] != "unhealthy" {
		t.Errorf("db health = %q", m["systemd-db"])
	}
	if _, ok := m["plain"]; ok {
		t.Error("container without healthcheck must be absent")
	}
}

func TestHealthcheckRun(t *testing.T) {
	fakePodman(t, "exit 0\n")
	ok, err := HealthcheckRun(context.Background(), "systemd-webapp")
	if err != nil || !ok {
		t.Errorf("exit 0 = passed: ok=%v err=%v", ok, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "podman")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nif [ \"$1\" = healthcheck ]; then exit 1; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ok, err = HealthcheckRun(context.Background(), "systemd-webapp")
	if err != nil || ok {
		t.Errorf("exit 1 = failed healthcheck: ok=%v err=%v", ok, err)
	}
}

func TestAutoUpdateDryRun(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[{"Container":"abc123","Image":"quay.io/x:latest","Policy":"registry","Unit":"webapp.service","Updated":"pending"}]
EOF
`)
	entries, err := AutoUpdateDryRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Unit != "webapp.service" || entries[0].Updated != "pending" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestAutoUpdateDryRunEmpty(t *testing.T) {
	fakePodman(t, "exit 0\n") // podman prints nothing when no labeled containers
	entries, err := AutoUpdateDryRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}

func TestAvailable(t *testing.T) {
	fakePodman(t, "true\n")
	if !Available() {
		t.Error("Available should find the fake podman on PATH")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if Available() {
		t.Error("Available should be false with no podman on PATH")
	}
}
