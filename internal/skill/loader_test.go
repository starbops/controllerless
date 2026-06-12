package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSkillFile writes content to <dir>/<skillName>/SKILL.md.
func writeSkillFile(t *testing.T, dir, skillName, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, skillName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

func makeValidSkillContent(name string) string {
	return `---
name: ` + name + `
description: A valid skill.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added, Modified]
allowedTools:
  - get
  - done
---

# What this skill does

Does something useful.

# Procedure

1. **Stability check.** If converged, call ` + "`done()`" + ` and return.
2. **Get resource.** Call ` + "`get()`" + ` to fetch.

# Errors

Handle errors.
`
}

func TestLoad_ValidSkills(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "skill-alpha", makeValidSkillContent("skill-alpha"))
	writeSkillFile(t, dir, "skill-beta", makeValidSkillContent("skill-beta"))

	skills, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestLoad_InvalidSkillSkipped(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill", makeValidSkillContent("good-skill"))
	// bad-skill: name does not match directory (L8 violation)
	badContent := `---
name: wrong-name
description: Bad skill.
triggers:
  - gvk: agentic.io/v1alpha1/ScheduledTask
    eventTypes: [Added]
allowedTools:
  - done
---

# What this skill does

Does something.

# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.

# Errors

Handle errors.
`
	writeSkillFile(t, dir, "bad-skill", badContent)

	skills, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("expected 1 valid skill, got %d", len(skills))
	}
	if skills[0].Frontmatter.Name != "good-skill" {
		t.Errorf("expected good-skill, got %q", skills[0].Frontmatter.Name)
	}
}

func TestLoad_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	skills, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestLoad_NonexistentDir(t *testing.T) {
	_, err := Load("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent dir, got nil")
	}
}
