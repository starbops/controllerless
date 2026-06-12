package dispatch

import (
	"strings"
	"testing"

	"github.com/starbops/controllerless/internal/skill"
)

func TestBuildPrompt_ContainsHarnessText(t *testing.T) {
	s := skill.Skill{
		Frontmatter: skill.Frontmatter{Name: "test-skill"},
		Body:        "## Instructions\nDo the thing.",
	}
	got := buildPrompt(s, "kind: Pod\nname: my-pod\n")
	if !strings.Contains(got, "autonomous Kubernetes reconciler") {
		t.Error("prompt missing harness system text")
	}
}

func TestBuildPrompt_ContainsSkillBody(t *testing.T) {
	s := skill.Skill{
		Frontmatter: skill.Frontmatter{Name: "test-skill"},
		Body:        "## Instructions\nDo the thing.",
	}
	got := buildPrompt(s, "kind: Pod\n")
	if !strings.Contains(got, "Do the thing.") {
		t.Error("prompt missing skill body")
	}
}

func TestBuildPrompt_ContainsEventYAML(t *testing.T) {
	s := skill.Skill{
		Frontmatter: skill.Frontmatter{Name: "test-skill"},
		Body:        "## Instructions\nDo the thing.",
	}
	eventYAML := "kind: Pod\nname: unique-event-marker\n"
	got := buildPrompt(s, eventYAML)
	if !strings.Contains(got, "unique-event-marker") {
		t.Error("prompt missing event YAML")
	}
}

func TestBuildPrompt_SectionsOrdered(t *testing.T) {
	s := skill.Skill{
		Frontmatter: skill.Frontmatter{Name: "test-skill"},
		Body:        "SKILL_BODY_MARKER",
	}
	eventYAML := "EVENT_YAML_MARKER"
	got := buildPrompt(s, eventYAML)

	harnessIdx := strings.Index(got, "autonomous Kubernetes reconciler")
	skillIdx := strings.Index(got, "SKILL_BODY_MARKER")
	eventIdx := strings.Index(got, "EVENT_YAML_MARKER")

	if harnessIdx < 0 || skillIdx < 0 || eventIdx < 0 {
		t.Fatal("missing one or more expected sections")
	}
	if !(harnessIdx < skillIdx && skillIdx < eventIdx) {
		t.Error("expected harness < skill body < event YAML in prompt")
	}
}
