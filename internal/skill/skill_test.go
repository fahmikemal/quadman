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
	if !strings.Contains(md, "quadman --skill") || !strings.Contains(md, "quadman --system list") {
		t.Errorf("expected skill export and pre-command system list in markdown, got: %s", md)
	}
	if strings.Contains(md, "quadman list --system") {
		t.Errorf("post-command list --system is rejected by the CLI, must not be documented: %s", md)
	}

	dv := Get("v0.5.1")
	if !strings.Contains(dv.Markdown(), "# Agent Skill: quadman (v0.5.1)") {
		t.Errorf("v-prefixed version must render single v prefix, got: %s", dv.Markdown())
	}

	js, err := d.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, `"name": "quadman"`) {
		t.Errorf("expected json format, got: %s", js)
	}
}
