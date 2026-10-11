package podman

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixtures below are trimmed copies of real podman 6.1.1 output.

func TestSystemDfSummary(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
    {"Type":"Images","Total":517,"Active":21,"RawSize":38876881697,"RawReclaimable":27371309784,"TotalCount":517,"Size":"38.88GB","Reclaimable":"27.37GB (70%)"},
    {"Type":"Local Volumes","Total":44,"Active":26,"RawSize":10312140674,"RawReclaimable":841533471,"TotalCount":44,"Size":"10.31GB","Reclaimable":"841.5MB (8%)"}
]
EOF
`)
	rows, err := SystemDfSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Type != "Images" || rows[0].Total != 517 || rows[0].RawReclaimable != 27371309784 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestImagesDangling(t *testing.T) {
	fakePodman(t, `cat <<'EOF'
[
  {"Id":"db0e114aab45","Names":["localhost/superset-local:6.1.0"],"Size":901634126,"Created":1791475300,"Containers":2},
  {"Id":"0f2c77d1e9b3","Names":null,"Size":1400000000,"Created":1791475300,"Containers":0}
]
EOF
`)
	imgs, err := Images(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 || imgs[0].Dangling() || !imgs[1].Dangling() {
		t.Fatalf("images = %+v", imgs)
	}
	if imgs[1].Ref() != "<none>:<none>" || imgs[0].Ref() != "localhost/superset-local:6.1.0" {
		t.Errorf("refs = %q %q", imgs[0].Ref(), imgs[1].Ref())
	}
}

func TestContainersLabelsAndSize(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakePodman(t, `echo "$@" > `+argsFile+`
cat <<'EOF'
[{"Names":["systemd-web"],"Image":"nginx","ImageID":"93aa","State":"running","Status":"Up 2 hours","Labels":{"PODMAN_SYSTEMD_UNIT":"web.service"},"Created":1786531448,"Id":"215fa3a79d8f35a8","Size":{"rootFsSize":299969757,"rwSize":35362}}]
EOF
`)
	cs, err := Containers(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].SystemdUnit() != "web.service" || cs[0].Name() != "systemd-web" || cs[0].Size.RW != 35362 {
		t.Errorf("containers = %+v", cs)
	}
	args, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(args), "--size") {
		t.Errorf("withSize must pass --size: %q", args)
	}
}

func TestParseVolumeSizes(t *testing.T) {
	text := `Images space usage:

REPOSITORY  TAG  IMAGE ID  CREATED  SIZE  SHARED SIZE  UNIQUE SIZE  CONTAINERS

Containers space usage:

CONTAINER ID  IMAGE  COMMAND  LOCAL VOLUMES  SIZE  CREATED  STATUS  NAMES

Local Volumes space usage:

VOLUME NAME                       LINKS       SIZE
aegis-local-auth                  0           1.686kB
fulltime90merch-postgres-data     1           89.44MB
`
	got := ParseVolumeSizes(text)
	if got["aegis-local-auth"] != "1.686kB" || got["fulltime90merch-postgres-data"] != "89.44MB" || len(got) != 2 {
		t.Errorf("sizes = %v", got)
	}
}

func TestParseEvent(t *testing.T) {
	line := `{"ID":"ddd293e1","Image":"docker.io/library/busybox:latest","Name":"systemd-demo-web","Status":"health_status","time":1789478343,"timeNano":1789478343718938337,"Type":"container","health_status":"unhealthy","Attributes":{"PODMAN_SYSTEMD_UNIT":"demo-web.service"}}`
	e, ok := ParseEvent(line)
	if !ok || e.HealthStatus != "unhealthy" || e.SystemdUnit() != "demo-web.service" || e.Type != "container" {
		t.Fatalf("event = %+v ok=%v", e, ok)
	}
	if !e.At().Equal(time.Unix(0, 1789478343718938337)) {
		t.Errorf("At = %v", e.At())
	}
	died := `{"ContainerExitCode":137,"ID":"61d6","Name":"x","Status":"died","time":1789477405,"Type":"container","Attributes":null}`
	if e, ok := ParseEvent(died); !ok || e.ContainerExit == nil || *e.ContainerExit != 137 {
		t.Errorf("died event = %+v", e)
	}
	if _, ok := ParseEvent("not json"); ok {
		t.Error("garbage must not parse")
	}
}

func TestSecretCreateUsesStdin(t *testing.T) {
	dir := t.TempDir()
	fakePodman(t, `echo "$@" > `+dir+`/args
cat > `+dir+`/stdin
`)
	if err := SecretCreate(context.Background(), "db-pass", []byte("s3cret"), true); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args"))
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if strings.Contains(string(args), "s3cret") {
		t.Fatalf("the secret value must never be in argv: %q", args)
	}
	if string(stdin) != "s3cret" || !strings.Contains(string(args), "--replace") || !strings.Contains(string(args), "-- db-pass -") {
		t.Errorf("args=%q stdin=%q", args, stdin)
	}
}

func TestHumanAndParseBytes(t *testing.T) {
	for n, want := range map[int64]string{0: "0B", 999: "999B", 1420000000: "1.42GB", 388000000: "388MB", 12580000: "12.6MB"} {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
	for s, want := range map[string]int64{"12.58MB": 12580000, "1.2kB": 1200, "0B": 0, "1GiB": 1 << 30, "junk": 0} {
		if got := ParseBytes(s); got != want {
			t.Errorf("ParseBytes(%q) = %d, want %d", s, got, want)
		}
	}
}
