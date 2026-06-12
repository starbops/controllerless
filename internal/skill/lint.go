package skill

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Lint checks all L1–L8 rules and returns every violation found.
func Lint(s Skill) []LintError {
	var errs []LintError
	errs = append(errs, lintL7(s)...)    // required sections (needed by others)
	errs = append(errs, lintL1(s)...)
	errs = append(errs, lintL2(s)...)
	errs = append(errs, lintL3L6(s)...)  // tool cross-check (L3 + L6 together)
	errs = append(errs, lintL4(s)...)
	errs = append(errs, lintL5(s)...)
	errs = append(errs, lintL8(s)...)
	return errs
}

// procedureSteps extracts numbered steps from the "# Procedure" section.
// Returns nil if the section is absent.
func procedureSteps(body string) []string {
	section := extractSection(body, "# Procedure")
	if section == "" {
		return nil
	}
	var steps []string
	re := regexp.MustCompile(`(?m)^\d+\. `)
	idx := re.FindAllStringIndex(section, -1)
	for i, loc := range idx {
		start := loc[0]
		var end int
		if i+1 < len(idx) {
			end = idx[i+1][0]
		} else {
			end = len(section)
		}
		// Capture the full block for this step (including continuation lines).
		stepText := strings.TrimRight(section[start:end], "\n ")
		// Strip the leading "N. " prefix.
		stepText = re.ReplaceAllString(stepText, "")
		steps = append(steps, stepText)
	}
	return steps
}

// extractSection returns the text under a heading until the next same-level heading.
func extractSection(body, heading string) string {
	lines := strings.Split(body, "\n")
	level := strings.Count(strings.Split(heading, " ")[0], "#")
	prefix := strings.Repeat("#", level) + " "

	var buf []string
	inside := false
	for _, line := range lines {
		if line == heading {
			inside = true
			continue
		}
		if inside {
			// Stop at next heading of same or higher level.
			if strings.HasPrefix(line, prefix) && line != heading {
				break
			}
			buf = append(buf, line)
		}
	}
	return strings.TrimSpace(strings.Join(buf, "\n"))
}

// firstStepWord returns the first significant word of a step line,
// stripping "N. " prefixes and **bold** markers.
func firstStepWord(step string) string {
	// Strip leading bold: **Word phrase.**
	step = regexp.MustCompile(`^\*\*`).ReplaceAllString(step, "")
	fields := strings.Fields(step)
	if len(fields) == 0 {
		return ""
	}
	w := fields[0]
	// Strip trailing ** or punctuation.
	w = strings.TrimRight(w, "*.,:")
	return w
}

// lintL1: Step 1 must be a stability/idempotency check.
func lintL1(s Skill) []LintError {
	steps := procedureSteps(s.Body)
	if len(steps) == 0 {
		return nil // L7 already caught missing section
	}
	step1 := strings.ToLower(steps[0])
	keywords := []string{"stability", "idempotent", "idempotency", "check", "verify", "converge", "done"}
	for _, kw := range keywords {
		if strings.Contains(step1, kw) {
			return nil
		}
	}
	return []LintError{{Rule: "L1", Message: "step 1 must be a stability/idempotency check"}}
}

// lintL2: Each numbered step must start with an uppercase word (imperative verb).
func lintL2(s Skill) []LintError {
	steps := procedureSteps(s.Body)
	var errs []LintError
	for i, step := range steps {
		w := firstStepWord(step)
		if w == "" {
			continue
		}
		runes := []rune(w)
		if !unicode.IsUpper(runes[0]) {
			errs = append(errs, LintError{
				Rule:    "L2",
				Message: fmt.Sprintf("step %d does not start with an uppercase word (got %q)", i+1, w),
			})
		}
	}
	return errs
}

// backtickRE matches `identifier` or `identifier(...)` inside backticks.
var backtickRE = regexp.MustCompile("`([^`]+)`")

// lintL3L6 enforces:
// L3: tools from allowedTools must appear in backticks (not bare).
// L6: backtick-quoted tool calls must be in allowedTools.
func lintL3L6(s Skill) []LintError {
	var errs []LintError
	tools := make(map[string]bool, len(s.Frontmatter.AllowedTools))
	for _, t := range s.Frontmatter.AllowedTools {
		tools[t] = true
	}

	// Extract backtick-quoted identifiers → check against allowedTools (L6).
	quoted := backtickRE.FindAllStringSubmatch(s.Body, -1)
	for _, m := range quoted {
		inner := m[1]
		// Extract the identifier (part before '(' if present).
		name := inner
		if i := strings.Index(inner, "("); i >= 0 {
			name = inner[:i]
		}
		name = strings.TrimSpace(name)
		// Only flag if it looks like a tool (no '/' or spaces, not empty, not a field path
		// with 'spec.', 'metadata.', 'status.' prefixes, etc.).
		if name == "" || isFieldPath(name) {
			continue
		}
		// If it has parens it's definitely a call; otherwise check if it matches an allowedTool.
		hasParen := strings.Contains(inner, "(")
		if hasParen && !tools[name] {
			errs = append(errs, LintError{
				Rule:    "L6",
				Message: fmt.Sprintf("tool %q used in body is not in allowedTools", name),
			})
		} else if !hasParen && tools[name] {
			// It's a plain tool mention, already backtick-quoted — OK.
		} else if !hasParen && !tools[name] {
			// Could be a field path or unknown identifier; skip.
		}
	}

	// L3: for each allowedTool, check it doesn't appear bare (unquoted) in body.
	for tool := range tools {
		// Build a regex that matches the tool name as a word NOT inside backticks.
		// We scan for word-boundary matches of the tool name and check none is inside a backtick span.
		if bareToolInBody(tool, s.Body) {
			errs = append(errs, LintError{
				Rule:    "L3",
				Message: fmt.Sprintf("tool %q referenced without backticks", tool),
			})
		}
	}

	return errs
}

// isFieldPath returns true if name looks like a Kubernetes field path
// (e.g., spec.suspend, metadata.name, status.fires).
func isFieldPath(name string) bool {
	fieldPrefixes := []string{"spec.", "metadata.", "status.", "data.", "stringData."}
	for _, p := range fieldPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// bareToolInBody returns true if tool appears as a bare (unquoted) word in body
// outside backtick spans. The check is case-sensitive because tool names are
// lowercase identifiers; uppercase occurrences (e.g., bold step labels) are prose.
func bareToolInBody(tool, body string) bool {
	// Remove all backtick-quoted spans first.
	stripped := backtickRE.ReplaceAllString(body, " ")
	// Case-sensitive word-boundary match.
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(tool) + `\b`)
	return re.MatchString(stripped)
}

// lintL4: Procedure must have ≤ 8 numbered steps.
func lintL4(s Skill) []LintError {
	steps := procedureSteps(s.Body)
	if len(steps) > 8 {
		return []LintError{{
			Rule:    "L4",
			Message: fmt.Sprintf("procedure has %d steps; maximum is 8", len(steps)),
		}}
	}
	return nil
}

// lintL5: Sub-list nesting depth must be ≤ 1.
func lintL5(s Skill) []LintError {
	section := extractSection(s.Body, "# Procedure")
	if section == "" {
		return nil
	}
	// A line is a sub-list item if it starts with spaces followed by '-' or '*'.
	// Depth is computed by the number of leading spaces / 2 (or 3).
	subListRE := regexp.MustCompile(`^( +)[*\-] `)
	for _, line := range strings.Split(section, "\n") {
		m := subListRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		indent := len(m[1])
		// Depth 1: 3–4 spaces (first sub-level under a numbered step).
		// Depth 2+: 5+ spaces.
		if indent >= 5 {
			return []LintError{{Rule: "L5", Message: "sub-list nesting exceeds depth 1"}}
		}
	}
	return nil
}

// lintL7: Body must have "# What this skill does" and "# Procedure" sections.
func lintL7(s Skill) []LintError {
	var errs []LintError
	if !strings.Contains(s.Body, "# What this skill does") {
		errs = append(errs, LintError{Rule: "L7", Message: "body missing '# What this skill does' section"})
	}
	if !strings.Contains(s.Body, "# Procedure") {
		errs = append(errs, LintError{Rule: "L7", Message: "body missing '# Procedure' section"})
	}
	return errs
}

// lintL8: Frontmatter name must match the parent directory name.
func lintL8(s Skill) []LintError {
	dir := filepath.Base(filepath.Dir(s.Path))
	if dir != s.Frontmatter.Name {
		return []LintError{{
			Rule:    "L8",
			Message: fmt.Sprintf("frontmatter name %q does not match directory %q", s.Frontmatter.Name, dir),
		}}
	}
	return nil
}
