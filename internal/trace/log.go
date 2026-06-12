package trace

import (
	"log/slog"
	"os"
)

// Init configures the default slog handler to JSON output.
// Level is read from LOG_LEVEL env var; defaults to INFO.
func Init() {
	level := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		var l slog.Level
		if err := l.UnmarshalText([]byte(v)); err == nil {
			level = l
		}
	}
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
