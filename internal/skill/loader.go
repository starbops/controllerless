package skill

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// Load scans dir for skill subdirectories, each containing a SKILL.md file.
// Skills that fail parsing or lint are logged and skipped.
func Load(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skill: read dir %q: %w", dir, err)
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		s, err := loadFile(skillPath)
		if err != nil {
			slog.Warn("skill load failed, skipping", "path", skillPath, "err", err)
			continue
		}
		lintErrs := Lint(s)
		if len(lintErrs) > 0 {
			for _, le := range lintErrs {
				slog.Warn("skill lint failure, skipping", "path", skillPath, "rule", le.Rule, "msg", le.Message)
			}
			continue
		}
		skills = append(skills, s)
	}
	return skills, nil
}

// loadFile reads and parses a single SKILL.md file.
func loadFile(path string) (Skill, error) {
	data, err := fs.ReadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		return Skill{}, fmt.Errorf("skill: read %q: %w", path, err)
	}
	fm, body, err := Parse(string(data))
	if err != nil {
		return Skill{}, fmt.Errorf("skill: parse %q: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return Skill{Frontmatter: fm, Body: body, Path: abs}, nil
}
