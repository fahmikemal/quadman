package quadlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseQuadlets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stack.quadlets")
	content := `# FileName=webapp
[Container]
Image=docker.io/library/nginx:latest
PublishPort=8080:80
---
# FileName=data
[Volume]
---
# FileName=net0
[Network]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := ParseQuadlets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("docs = %d, want 3: %+v", len(docs), docs)
	}
	if docs[0].Name != "webapp" || docs[0].Kind != KindContainer {
		t.Errorf("doc0 = %+v", docs[0])
	}
	if docs[1].Name != "data" || docs[1].Kind != KindVolume {
		t.Errorf("doc1 = %+v", docs[1])
	}
	if docs[2].Name != "net0" || docs[2].Kind != KindNetwork {
		t.Errorf("doc2 = %+v", docs[2])
	}
	if docs[0].Content == "" || !contains(docs[0].Content, "Image=") {
		t.Errorf("doc0 content must keep the body: %q", docs[0].Content)
	}
}

func TestParseQuadletsSkipsUnnamed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.quadlets")
	content := `# FileName=webapp
[Container]
Image=nginx
---
[Container]
Image=unnamed-should-be-skipped
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := ParseQuadlets(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Name != "webapp" {
		t.Errorf("documents without FileName must be skipped: %+v", docs)
	}
}

func TestDiscoverQuadletsBundle(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("stack.quadlets", "# FileName=webapp\n[Container]\nImage=nginx\n")
	write("webapp.container", "[Container]\nImage=nginx\n")

	units, err := DiscoverDirs([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	var bundle, single *Unit
	for i := range units {
		switch units[i].Kind {
		case KindQuadlets:
			bundle = &units[i]
		case KindContainer:
			single = &units[i]
		}
	}
	if bundle == nil {
		t.Fatal("the .quadlets bundle must be discovered")
	}
	if bundle.UnitName != "" {
		t.Errorf("a bundle maps to no single unit, got %q", bundle.UnitName)
	}
	if single == nil {
		t.Fatal("regular quadlet files must still be discovered")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
