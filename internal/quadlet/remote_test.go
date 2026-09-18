package quadlet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fahmikemal/quadman/internal/remote"
)

func fakeRunner(t *testing.T, script string) remote.Runner {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ssh")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return remote.Runner{Target: "h", SSHBin: bin}
}

func TestDiscoverRemotePodman(t *testing.T) {
	script := "#!/bin/sh\necho \"web.container\\tweb.service\\t/home/u/.config/containers/systemd/web.container\"\n"
	r := fakeRunner(t, script)
	units, err := DiscoverRemote(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("units = %+v, want 1", units)
	}
	u := units[0]
	if u.Name != "web" || u.Kind != KindContainer || u.UnitName != "web.service" {
		t.Errorf("unit = %+v", u)
	}
}

func TestDiscoverRemoteFallbackListUnits(t *testing.T) {
	// First call (podman) fails, second (systemctl) answers.
	script := "#!/bin/sh\nif echo \"$@\" | grep -q podman; then exit 1; fi\necho \"demo-web.service loaded active running Demo\"\n"
	r := fakeRunner(t, script)
	units, err := DiscoverRemote(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].UnitName != "demo-web.service" {
		t.Fatalf("units = %+v", units)
	}
}

func TestParseBytesRoundtrip(t *testing.T) {
	f, err := ParseBytes("web.container", []byte("[Container]\nImage=nginx\n\n[Install]\nWantedBy=default.target\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Image() != "nginx" {
		t.Errorf("image = %q", f.Image())
	}
	if f.BootTarget() != "default.target" {
		t.Errorf("boot = %q", f.BootTarget())
	}
}

func TestParseQuadletListKinds(t *testing.T) {
	units := parseQuadletList([]byte("data.volume\tdata-volume.service\t/p/data.volume\nnet.network\tnet-network.service\t/p/net.network\n"))
	if len(units) != 2 || units[0].Kind != KindVolume || units[1].Kind != KindNetwork {
		t.Fatalf("units = %+v", units)
	}
}
