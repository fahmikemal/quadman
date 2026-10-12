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

// TestDiscoverRemoteFallbackGenerated: without podman, discovery keeps
// only units generated from a Quadlet source (SourcePath=), never other
// generators' output or ordinary services.
func TestDiscoverRemoteFallbackGenerated(t *testing.T) {
	script := `#!/bin/sh
for last; do :; done
case "$last" in
  podman*) exit 127 ;;
  *list-unit-files*)
    printf 'demo-web.service generated -\napp-autostart.service generated -\ncache-volume.service generated -\n' ;;
  *show*)
    printf 'Id=demo-web.service\nSourcePath=/home/u/.config/containers/systemd/apps/demo-web.container\n\n'
    printf 'Id=app-autostart.service\nSourcePath=/etc/xdg/autostart/app.desktop\n\n'
    printf 'Id=cache-volume.service\nSourcePath=/home/u/.config/containers/systemd/cache.volume\n' ;;
esac
`
	r := fakeRunner(t, script)
	units, err := DiscoverRemote(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatalf("units = %+v, want the two quadlet-generated units only", units)
	}
	if u := units[1]; u.Name != "demo-web" || u.Kind != KindContainer || u.UnitName != "demo-web.service" || u.Path != "/home/u/.config/containers/systemd/apps/demo-web.container" {
		t.Errorf("container unit = %+v", u)
	}
	if u := units[0]; u.Name != "cache" || u.Kind != KindVolume || u.UnitName != "cache-volume.service" {
		t.Errorf("volume unit = %+v", u)
	}
}

func TestDiscoverRemoteFallbackNoGenerated(t *testing.T) {
	script := "#!/bin/sh\nfor last; do :; done\ncase \"$last\" in podman*) exit 127 ;; esac\n"
	units, err := DiscoverRemote(context.Background(), fakeRunner(t, script))
	if err != nil || len(units) != 0 {
		t.Fatalf("units = %+v, err = %v", units, err)
	}
}

// TestDiscoverRemoteFormatSurvivesSSH: the tab-separated --format must reach
// podman as one argument even though OpenSSH re-parses the command line.
func TestDiscoverRemoteFormatSurvivesSSH(t *testing.T) {
	script := `#!/bin/sh
for last; do :; done
eval "set -- $last"
[ "$#" -eq 5 ] || exit 3
printf 'web.container\tweb.service\t/p/web.container\n'
`
	units, err := DiscoverRemote(context.Background(), fakeRunner(t, script))
	if err != nil || len(units) != 1 {
		t.Fatalf("units = %+v, err = %v (format argument split by the remote shell?)", units, err)
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
	units := parseQuadletList([]byte("data.volume\tdata-volume.service\t/p/data.volume\nnet.network\tnet-network.service\t/p/net.network\nstack.quadlets\t\t/p/stack.quadlets\n"))
	if len(units) != 3 || units[0].Kind != KindVolume || units[1].Kind != KindNetwork {
		t.Fatalf("units = %+v", units)
	}
	if units[2].Kind != KindQuadlets || units[2].UnitName != "" || units[2].Name != "stack" {
		t.Errorf("bundle = %+v", units[2])
	}
}

func TestInspectRemoteFile(t *testing.T) {
	script := "#!/bin/sh\nprintf '[Container]\\nImage=nginx\\nContainerName=web-main\\n'\n"
	info, f := InspectRemoteFile(context.Background(), fakeRunner(t, script), Unit{Name: "web", Kind: KindContainer, Path: "/p/web.container"})
	if f == nil || info.Image != "nginx" || info.ContainerName != "web-main" || info.UnitName != "web.service" {
		t.Errorf("info = %+v, file = %v", info, f)
	}
}
