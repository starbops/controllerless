package skill

import (
	"io"
	"log/slog"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// watcher wraps an fsnotify.Watcher and implements io.Closer.
type watcher struct {
	fw *fsnotify.Watcher
}

func (w *watcher) Close() error {
	return w.fw.Close()
}

// Watch monitors dir for changes to SKILL.md files. On any create/write/remove
// event it re-runs Load; if the new set is non-empty it calls onChange.
// Lint or parse failures are logged and the previous state is preserved
// (onChange is not called).
//
// Returns an io.Closer that stops the watcher.
func Watch(dir string, onChange func([]Skill)) (io.Closer, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Walk and add the root dir and all skill subdirs.
	if err := watchRecursive(fw, dir); err != nil {
		_ = fw.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-fw.Events:
				if !ok {
					return
				}
				if !isSkillFile(event.Name) {
					continue
				}
				if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Remove) {
					// On new subdirectories, ensure we watch them.
					if event.Has(fsnotify.Create) {
						_ = fw.Add(filepath.Dir(event.Name))
					}
					skills, err := Load(dir)
					if err != nil {
						slog.Warn("skill reload failed", "err", err)
						continue
					}
					if len(skills) > 0 {
						onChange(skills)
					}
				}
			case err, ok := <-fw.Errors:
				if !ok {
					return
				}
				slog.Warn("skill watcher error", "err", err)
			}
		}
	}()

	return &watcher{fw: fw}, nil
}

// watchRecursive adds dir and all immediate subdirectories to the watcher.
func watchRecursive(fw *fsnotify.Watcher, dir string) error {
	if err := fw.Add(dir); err != nil {
		return err
	}
	// Add each skill subdirectory.
	entries, _ := filepath.Glob(filepath.Join(dir, "*"))
	for _, e := range entries {
		_ = fw.Add(e)
	}
	return nil
}

// isSkillFile returns true if path is a SKILL.md file.
func isSkillFile(path string) bool {
	return filepath.Base(path) == "SKILL.md"
}
