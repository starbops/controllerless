package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// skillAt returns a Skill placed at <tmpDir>/<name>/SKILL.md for L8 testing.
func skillAt(name, body string, tools []string, dir string) Skill {
	return Skill{
		Frontmatter: Frontmatter{
			Name:         name,
			Description:  "test skill",
			Triggers:     []TriggerSpec{{GVK: "agentic.io/v1alpha1/ScheduledTask", EventTypes: []string{"Added"}}},
			AllowedTools: tools,
		},
		Body: body,
		Path: filepath.Join(dir, name, "SKILL.md"),
	}
}

const goodBody = `# What this skill does

Does something useful.

# Procedure

1. **Stability check.** If already converged, call ` + "`done()`" + ` and return.
2. **Get the resource.** Call ` + "`get()`" + ` to fetch current state.

# Errors

Handle errors gracefully.
`

func TestLint_L1_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L1" {
			t.Errorf("unexpected L1 violation: %s", e.Message)
		}
	}
}

func TestLint_L1_Fail(t *testing.T) {
	body := `# What this skill does

Does something.

# Procedure

1. **Create the resource.** Call ` + "`create()`" + ` immediately.
2. **Patch status.** Call ` + "`patch()`" + ` to update.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"create", "patch"}, t.TempDir())
	errs := Lint(s)
	hasL1 := false
	for _, e := range errs {
		if e.Rule == "L1" {
			hasL1 = true
		}
	}
	if !hasL1 {
		t.Error("expected L1 violation, got none")
	}
}

func TestLint_L2_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L2" {
			t.Errorf("unexpected L2 violation: %s", e.Message)
		}
	}
}

func TestLint_L2_Fail(t *testing.T) {
	body := `# What this skill does

Does something.

# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.
2. the resource should be retrieved using ` + "`get()`" + `.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	hasL2 := false
	for _, e := range errs {
		if e.Rule == "L2" {
			hasL2 = true
		}
	}
	if !hasL2 {
		t.Error("expected L2 violation, got none")
	}
}

func TestLint_L3_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L3" {
			t.Errorf("unexpected L3 violation: %s", e.Message)
		}
	}
}

func TestLint_L3_Fail(t *testing.T) {
	// "done" appears without backticks
	body := `# What this skill does

Does something.

# Procedure

1. **Stability check.** If converged, call done and return.
2. **Get resource.** Call ` + "`get()`" + ` to fetch.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	hasL3 := false
	for _, e := range errs {
		if e.Rule == "L3" {
			hasL3 = true
		}
	}
	if !hasL3 {
		t.Error("expected L3 violation, got none")
	}
}

func TestLint_L4_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L4" {
			t.Errorf("unexpected L4 violation: %s", e.Message)
		}
	}
}

func TestLint_L4_Fail(t *testing.T) {
	steps := make([]string, 9)
	for i := range steps {
		steps[i] = strings.Repeat(" ", 0) + strings.Replace(
			"N. **Step N.** Call `get()` to fetch.", "N", string(rune('1'+i)), -1)
	}
	// Step 1 must be stability check for L1 not to interfere
	steps[0] = "1. **Stability check.** Call `done()` and return."
	body := "# What this skill does\n\nDoes something.\n\n# Procedure\n\n" +
		strings.Join(steps, "\n") + "\n\n# Errors\n\nHandle errors.\n"
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	hasL4 := false
	for _, e := range errs {
		if e.Rule == "L4" {
			hasL4 = true
		}
	}
	if !hasL4 {
		t.Error("expected L4 violation, got none")
	}
}

func TestLint_L5_Pass(t *testing.T) {
	body := `# What this skill does

Does something.

# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.
2. **Get resource.**
   - Call ` + "`get()`" + ` to fetch.
   - Inspect the result.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L5" {
			t.Errorf("unexpected L5 violation: %s", e.Message)
		}
	}
}

func TestLint_L5_Fail(t *testing.T) {
	body := `# What this skill does

Does something.

# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.
2. **Get resource.**
   - Call ` + "`get()`" + ` to fetch.
     - Check the result deeply.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	hasL5 := false
	for _, e := range errs {
		if e.Rule == "L5" {
			hasL5 = true
		}
	}
	if !hasL5 {
		t.Error("expected L5 violation, got none")
	}
}

func TestLint_L6_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L6" {
			t.Errorf("unexpected L6 violation: %s", e.Message)
		}
	}
}

func TestLint_L6_Fail(t *testing.T) {
	// body references `delete()` which is not in allowedTools
	body := `# What this skill does

Does something.

# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.
2. **Remove resource.** Call ` + "`delete()`" + ` to remove.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done"}, t.TempDir())
	errs := Lint(s)
	hasL6 := false
	for _, e := range errs {
		if e.Rule == "L6" {
			hasL6 = true
		}
	}
	if !hasL6 {
		t.Error("expected L6 violation, got none")
	}
}

func TestLint_L7_Pass(t *testing.T) {
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L7" {
			t.Errorf("unexpected L7 violation: %s", e.Message)
		}
	}
}

func TestLint_L7_Fail_MissingSection(t *testing.T) {
	body := `# Procedure

1. **Stability check.** Call ` + "`done()`" + ` and return.

# Errors

Handle errors.
`
	s := skillAt("my-skill", body, []string{"done"}, t.TempDir())
	errs := Lint(s)
	hasL7 := false
	for _, e := range errs {
		if e.Rule == "L7" {
			hasL7 = true
		}
	}
	if !hasL7 {
		t.Error("expected L7 violation (missing '# What this skill does'), got none")
	}
}

func TestLint_L8_Pass(t *testing.T) {
	dir := t.TempDir()
	s := skillAt("my-skill", goodBody, []string{"done", "get"}, dir)
	errs := Lint(s)
	for _, e := range errs {
		if e.Rule == "L8" {
			t.Errorf("unexpected L8 violation: %s", e.Message)
		}
	}
}

func TestLint_L8_Fail(t *testing.T) {
	dir := t.TempDir()
	s := Skill{
		Frontmatter: Frontmatter{
			Name:         "my-skill",
			Description:  "test",
			Triggers:     []TriggerSpec{{GVK: "agentic.io/v1alpha1/ScheduledTask", EventTypes: []string{"Added"}}},
			AllowedTools: []string{"done", "get"},
		},
		Body: goodBody,
		// Path directory name is "wrong-dir", not "my-skill"
		Path: filepath.Join(dir, "wrong-dir", "SKILL.md"),
	}
	errs := Lint(s)
	hasL8 := false
	for _, e := range errs {
		if e.Rule == "L8" {
			hasL8 = true
		}
	}
	if !hasL8 {
		t.Error("expected L8 violation, got none")
	}
}

func TestLint_AllViolationsReturned(t *testing.T) {
	// A body that violates L4 (>8 steps) and L7 (missing sections)
	steps := make([]string, 9)
	steps[0] = "1. **Stability check.** Call `done()` and return."
	for i := 1; i < 9; i++ {
		steps[i] = strings.Replace(
			"N. **Step.** Call `get()` to fetch.", "N", string(rune('1'+i)), -1)
	}
	body := "# Procedure\n\n" + strings.Join(steps, "\n") + "\n\n# Errors\n\nHandle errors.\n"
	s := skillAt("my-skill", body, []string{"done", "get"}, t.TempDir())
	errs := Lint(s)
	rules := map[string]bool{}
	for _, e := range errs {
		rules[e.Rule] = true
	}
	if !rules["L4"] {
		t.Error("expected L4 in violations")
	}
	if !rules["L7"] {
		t.Error("expected L7 in violations (missing '# What this skill does')")
	}
}
