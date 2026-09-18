package skill

import (
	"strings"
	"testing"
)

func TestGetSkill(t *testing.T) {
	d := Get("0.5.0")
	if d.Name != "quadman" || d.Version != "0.5.0" {
		t.Errorf("unexpected skill metadata: %+v", d)
	}

	md := d.Markdown()
	if !strings.Contains(md, "# Agent Skill: quadman (v0.5.0)") {
		t.Errorf("expected header in markdown, got: %s", md)
	}
	if !strings.Contains(md, ".container") || !strings.Contains(md, "quadman list") {
		t.Errorf("expected quadlet specs in markdown, got: %s", md)
	}

	js, err := d.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"name": "quadman"`) {
		t.Errorf("expected json format, got: %s", js)
	}
}
