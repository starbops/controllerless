// Package trace handles structured logging (slog) and per-reconcile JSONL trace files.
package trace

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// Writer writes per-reconcile JSONL trace entries to a single file.
type Writer struct {
	reconcileID ulid.ULID
	file        *os.File
	bw          *bufio.Writer
}

// NewWriter creates a JSONL trace writer under tracesDir.
//
// The file is placed at:
//
//	<tracesDir>/<YYYY-MM-DD>/<namespace>__<gvk-slug>__<name>__<unix-ms>.jsonl
//
// where gvk-slug replaces every "/" with "-" (e.g. "apps/v1/Deployment" → "apps-v1-Deployment").
func NewWriter(tracesDir, namespace, gvk, name string, reconcileID ulid.ULID) (*Writer, error) {
	now := time.Now().UTC()
	dir := filepath.Join(tracesDir, now.Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("trace: mkdir %s: %w", dir, err)
	}

	gvkSlug := strings.ReplaceAll(gvk, "/", "-")
	filename := fmt.Sprintf("%s__%s__%s__%d.jsonl", namespace, gvkSlug, name, now.UnixMilli())
	path := filepath.Join(dir, filename)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("trace: open %s: %w", path, err)
	}
	return &Writer{
		reconcileID: reconcileID,
		file:        f,
		bw:          bufio.NewWriter(f),
	}, nil
}

// Write appends one JSON line to the trace file.
// Fields from payload (marshaled as a JSON object) are merged with the
// envelope fields t, reconcileId, and phase.
func (w *Writer) Write(phase string, payload any) error {
	env := map[string]any{
		"t":           time.Now().UTC().Format(time.RFC3339Nano),
		"reconcileId": w.reconcileID.String(),
		"phase":       phase,
	}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("trace: marshal payload: %w", err)
		}
		var fields map[string]any
		if err := json.Unmarshal(b, &fields); err != nil {
			return fmt.Errorf("trace: unmarshal payload fields: %w", err)
		}
		for k, v := range fields {
			env[k] = v
		}
	}

	line, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("trace: marshal entry: %w", err)
	}
	if _, err := fmt.Fprintf(w.bw, "%s\n", line); err != nil {
		return fmt.Errorf("trace: write line: %w", err)
	}
	return nil
}

// Close flushes the buffer and closes the underlying trace file.
func (w *Writer) Close() error {
	if err := w.bw.Flush(); err != nil {
		return fmt.Errorf("trace: flush: %w", err)
	}
	return w.file.Close()
}
