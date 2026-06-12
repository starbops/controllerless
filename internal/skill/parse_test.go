package skill

import (
	"testing"
)

const validSkillRaw = `---
name: my-skill
description: A test skill.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added, Modified]
    conditions:
      - "spec.suspend != true"
allowedTools:
  - get
  - done
---

# What this skill does

Does something useful.

# Procedure

1. **Stability check.** If already converged, call ` + "`done()`" + ` and return.
2. **Get the resource.** Call ` + "`get()`" + ` to fetch current state.
`

func TestParse_ValidFrontmatter(t *testing.T) {
	fm, body, err := Parse(validSkillRaw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "my-skill" {
		t.Errorf("expected name %q, got %q", "my-skill", fm.Name)
	}
	if fm.Description != "A test skill." {
		t.Errorf("expected description %q, got %q", "A test skill.", fm.Description)
	}
	if len(fm.Triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(fm.Triggers))
	}
	if fm.Triggers[0].GVK != "agentic.io/v1alpha1/ScheduledTask" {
		t.Errorf("unexpected GVK: %q", fm.Triggers[0].GVK)
	}
	if len(fm.Triggers[0].EventTypes) != 2 {
		t.Errorf("expected 2 eventTypes, got %d", len(fm.Triggers[0].EventTypes))
	}
	if len(fm.Triggers[0].Conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(fm.Triggers[0].Conditions))
	}
	if len(fm.AllowedTools) != 2 {
		t.Errorf("expected 2 allowedTools, got %d", len(fm.AllowedTools))
	}
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestParse_MissingFrontmatter(t *testing.T) {
	content := "# What this skill does\n\nNo frontmatter here.\n"
	_, _, err := Parse(content)
	if err == nil {
		t.Fatal("expected error for missing frontmatter, got nil")
	}
}

func TestParse_EmptyBody(t *testing.T) {
	content := "---\nname: empty-body\ndescription: test\ntriggers: []\nallowedTools: []\n---\n"
	fm, body, err := Parse(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "empty-body" {
		t.Errorf("expected name %q, got %q", "empty-body", fm.Name)
	}
	if body != "" {
		t.Errorf("expected empty body, got %q", body)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	content := "---\nname: [\nbadyaml\n---\nbody\n"
	_, _, err := Parse(content)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
