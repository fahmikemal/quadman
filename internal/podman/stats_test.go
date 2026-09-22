package podman

import (
	"context"
	"strings"
	"testing"
)

func TestStats(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
  {
    "id": "215fa3a79d8f",
    "name": "web",
    "cpu_time": "0s",
    "cpu_percent": "0.08%",
    "avg_cpu": "0.01%",
    "mem_usage": "12.58MB / 16.6GB",
    "mem_percent": "0.08%",
    "net_io": "1.2kB / 0B",
    "block_io": "0B / 0B",
    "pids": "12"
  }
]
EOF
`)
	entries, err := Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Name != "web" || e.CPU != "0.08%" || e.Mem != "12.58MB / 16.6GB" || e.MemPerc != "0.08%" || e.Net != "1.2kB / 0B" || e.Block != "0B / 0B" || e.PIDs != "12" {
		t.Errorf("entry = %+v", e)
	}
}

func TestStatsEmpty(t *testing.T) {
	for _, body := range []string{"echo '[]'\n", "echo ''\n", "echo 'null'\n"} {
		fakePodman(t, body)
		entries, err := Stats(context.Background())
		if err != nil {
			t.Fatalf("%q: %v", body, err)
		}
		if len(entries) != 0 {
			t.Errorf("%q: entries = %v, want empty", body, entries)
		}
	}
}

func TestContainerNames(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
  {"Names": ["web", "web-alias"], "Status": "Up 2 hours"},
  {"Names": ["db"], "Status": "Exited (0) 3 days ago"}
]
EOF
`)
	names, err := ContainerNames(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "web" || names[2] != "db" {
		t.Errorf("names = %v", names)
	}
}

func TestResolveExecTargetExactOnly(t *testing.T) {
	names := []string{"web", "web-staging"}
	if !ResolveExecTarget(names, "web") {
		t.Error("exact match should resolve")
	}
	for _, want := range []string{"", "  ", "we", "web-", "staging", "WEB"} {
		if ResolveExecTarget(names, want) {
			t.Errorf("%q must not resolve (exact-only)", want)
		}
	}
}

func TestSystemPrune(t *testing.T) {
	fakePodman(t, `if [ "$1" != "system" ] || [ "$2" != "prune" ] || [ "$3" != "-f" ]; then echo "wrong argv: $@"; exit 9; fi
echo "Total reclaimed space: 1.2GB"`)
	out, err := SystemPrune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1.2GB") {
		t.Errorf("output = %q", out)
	}
}

func TestSystemPruneError(t *testing.T) {
	fakePodman(t, "echo 'boom'; exit 1\n")
	if _, err := SystemPrune(context.Background()); err == nil {
		t.Error("expected error")
	}
}
