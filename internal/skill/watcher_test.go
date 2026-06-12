package skill

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_CallsOnChangeOnFileWrite(t *testing.T) {
	dir := t.TempDir()
	skillName := "watch-skill"
	writeSkillFile(t, dir, skillName, makeValidSkillContent(skillName))

	// Load initial skills to establish baseline.
	initial, err := Load(dir)
	if err != nil || len(initial) != 1 {
		t.Fatalf("initial load failed: err=%v, count=%d", err, len(initial))
	}

	changed := make(chan []Skill, 1)
	closer, err := Watch(dir, func(skills []Skill) {
		changed <- skills
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	defer func() { _ = closer.(io.Closer).Close() }()

	// Rewrite the skill file (same valid content triggers onChange).
	skillPath := filepath.Join(dir, skillName, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(makeValidSkillContent(skillName)), 0o644); err != nil {
		t.Fatalf("rewrite skill file: %v", err)
	}

	select {
	case skills := <-changed:
		if len(skills) != 1 {
			t.Errorf("expected 1 skill in onChange, got %d", len(skills))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("onChange was not called within 3 seconds")
	}
}

func TestWatch_InvalidSkillKeepsPrevious(t *testing.T) {
	dir := t.TempDir()
	skillName := "stable-skill"
	writeSkillFile(t, dir, skillName, makeValidSkillContent(skillName))

	called := make(chan struct{}, 1)
	closer, err := Watch(dir, func(_ []Skill) {
		called <- struct{}{}
	})
	if err != nil {
		t.Fatalf("Watch returned error: %v", err)
	}
	defer func() { _ = closer.(io.Closer).Close() }()

	// Overwrite with invalid content (no frontmatter).
	skillPath := filepath.Join(dir, skillName, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# No frontmatter here\n"), 0o644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}

	// onChange should NOT be called for a lint/parse failure.
	select {
	case <-called:
		t.Error("onChange should not be called when new load fails")
	case <-time.After(2 * time.Second):
		// expected: no callback for bad skill
	}
}
