package skill

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse splits a SKILL.md file into its Frontmatter and Markdown body.
// The expected format is:
//
//	---
//	<YAML>
//	---
//	<body>
func Parse(content string) (Frontmatter, string, error) {
	const delim = "---"

	// Must start with "---\n"
	if !strings.HasPrefix(content, delim+"\n") {
		return Frontmatter{}, "", fmt.Errorf("skill: missing frontmatter delimiter")
	}

	// Find closing "---"
	rest := content[len(delim)+1:] // skip opening "---\n"
	end := strings.Index(rest, "\n"+delim)
	if end < 0 {
		return Frontmatter{}, "", fmt.Errorf("skill: frontmatter closing delimiter not found")
	}

	yamlPart := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n"+delim):], "\n")

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return Frontmatter{}, "", fmt.Errorf("skill: invalid YAML frontmatter: %w", err)
	}

	return fm, body, nil
}
