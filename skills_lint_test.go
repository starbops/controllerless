package main

import (
	"fmt"
	"testing"

	"github.com/starbops/controllerless/internal/skill"
)

func TestSkillsLint(t *testing.T) {
	skills, err := skill.Load("skills")
	if err != nil {
		t.Fatalf("skill.Load failed: %v", err)
	}
	if len(skills) != 4 {
		t.Errorf("expected 4 skills, got %d", len(skills))
	}
	for _, s := range skills {
		errs := skill.Lint(s)
		if len(errs) > 0 {
			for _, e := range errs {
				fmt.Printf("SKILL %s: %s\n", s.Frontmatter.Name, e.Error())
			}
			t.Errorf("skill %q has %d lint error(s)", s.Frontmatter.Name, len(errs))
		}
	}
}
