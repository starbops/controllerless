// Package skill handles loading, parsing, linting, and hot-reloading skill documents.
package skill

// TriggerSpec is one entry in the frontmatter triggers list.
type TriggerSpec struct {
	// GVK is the raw string "group/version/Kind".
	GVK        string   `yaml:"gvk"`
	EventTypes []string `yaml:"eventTypes"`
	// Conditions are optional CEL expressions evaluated at dispatch time.
	Conditions []string `yaml:"conditions,omitempty"`
}

// Frontmatter is the YAML header of a SKILL.md file.
type Frontmatter struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Triggers     []TriggerSpec `yaml:"triggers"`
	AllowedTools []string      `yaml:"allowedTools"`
}

// Skill is a parsed skill document, ready for dispatch.
type Skill struct {
	Frontmatter Frontmatter
	Body        string // raw Markdown body (after frontmatter)
	Path        string // absolute path to the SKILL.md file
}

// LintError is a single lint rule violation.
type LintError struct {
	Rule    string
	Message string
}

func (e LintError) Error() string {
	return e.Rule + ": " + e.Message
}
