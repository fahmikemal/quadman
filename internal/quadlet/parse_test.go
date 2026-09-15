package quadlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webapp.container")
	content := `# a comment
[Unit]
Description=Web App
; another comment

[Container]
Image=docker.io/library/nginx:latest
PublishPort=8080:80
PublishPort=8443:443
Volume=webdata:/usr/share/nginx/html:Z
Environment=FOO=1 BAR=2 \
	BAZ=3
stray line without section

[Service]
Restart=always
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	c := f.Section("Container")
	if got := c.Get("image"); got != "docker.io/library/nginx:latest" {
		t.Errorf("Image = %q, want nginx image", got)
	}
	if got := c.GetAll("PublishPort"); len(got) != 2 {
		t.Errorf("PublishPort count = %d, want 2 (%v)", len(got), got)
	}
	if got := c.Get("Environment"); got != "FOO=1 BAR=2 BAZ=3" {
		t.Errorf("Environment = %q, want backslash-continued value", got)
	}
	if got := f.Section("unit").Get("Description"); got != "Web App" {
		t.Errorf("Description = %q, want %q", got, "Web App")
	}
	if got := f.Section("Service").Get("Restart"); got != "always" {
		t.Errorf("Restart = %q, want %q", got, "always")
	}
	if got := f.Image(); got != "docker.io/library/nginx:latest" {
		t.Errorf("Image() = %q, want nginx image", got)
	}
	if f.Section("Missing") != nil {
		t.Error("Section(Missing) should be nil")
	}
}

func TestUnitFileName(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		want string
	}{
		{"webapp", KindContainer, "webapp.service"},
		{"stack", KindPod, "stack-pod.service"},
		{"app", KindKube, "app.service"},
		{"cache", KindVolume, "cache-volume.service"},
		{"net0", KindNetwork, "net0-network.service"},
		{"base", KindImage, "base-image.service"},
		{"builder", KindBuild, "builder-build.service"},
		{"pkg", KindArtifact, "pkg-artifact.service"},
		{"web@", KindContainer, "web@.service"},
		{"web@", KindVolume, "web-volume@.service"},
	}
	for _, tc := range cases {
		if got := UnitFileName(tc.name, tc.kind); got != tc.want {
			t.Errorf("UnitFileName(%q, %q) = %q, want %q", tc.name, tc.kind, got, tc.want)
		}
	}
}

func TestInspect(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		desc     string
		unit     Unit
		wantName string
		wantImg  string
	}{
		{
			desc: "container default name and image",
			unit: Unit{Name: "webapp", Kind: KindContainer, Path: write("webapp.container",
				"[Container]\nImage=docker.io/library/nginx:latest\n")},
			wantName: "webapp.service",
			wantImg:  "docker.io/library/nginx:latest",
		},
		{
			desc: "container ServiceName override",
			unit: Unit{Name: "webapp", Kind: KindContainer, Path: write("override.container",
				"[Container]\nImage=quay.io/foo\nServiceName=web-custom\nAutoUpdate=registry\nImageVolume=tmpfs\n")},
			wantName: "web-custom.service",
			wantImg:  "quay.io/foo",
		},
		{
			desc: "volume default name, no image",
			unit: Unit{Name: "cache", Kind: KindVolume, Path: write("cache.volume",
				"[Volume]\nVolumeName=data\n")},
			wantName: "cache-volume.service",
			wantImg:  "",
		},
		{
			desc: "network default name",
			unit: Unit{Name: "net0", Kind: KindNetwork, Path: write("net0.network",
				"[Network]\n")},
			wantName: "net0-network.service",
			wantImg:  "",
		},
		{
			desc: "image file shows its Image key",
			unit: Unit{Name: "base", Kind: KindImage, Path: write("base.image",
				"[Image]\nImage=quay.io/base\n")},
			wantName: "base-image.service",
			wantImg:  "quay.io/base",
		},
		{
			desc: "build file shows ImageTag",
			unit: Unit{Name: "builder", Kind: KindBuild, Path: write("builder.build",
				"[Build]\nImageTag=localhost/builder:dev\n")},
			wantName: "builder-build.service",
			wantImg:  "localhost/builder:dev",
		},
		{
			desc:     "unreadable file falls back to default name",
			unit:     Unit{Name: "gone", Kind: KindContainer, Path: filepath.Join(dir, "missing.container")},
			wantName: "gone.service",
			wantImg:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := Inspect(tc.unit)
			if got.UnitName != tc.wantName {
				t.Errorf("UnitName = %q, want %q", got.UnitName, tc.wantName)
			}
			if got.Image != tc.wantImg {
				t.Errorf("Image = %q, want %q", got.Image, tc.wantImg)
			}
		})
	}
}

func TestInspectAutoUpdateAndImageVolume(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webapp.container")
	if err := os.WriteFile(path, []byte("[Container]\nImage=nginx\nAutoUpdate=registry\nImageVolume=tmpfs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Inspect(Unit{Name: "webapp", Kind: KindContainer, Path: path})
	if got.AutoUpdate != "registry" {
		t.Errorf("AutoUpdate = %q, want registry", got.AutoUpdate)
	}
	if got.ImageVolume != "tmpfs" {
		t.Errorf("ImageVolume = %q, want tmpfs", got.ImageVolume)
	}
}
