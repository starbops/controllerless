package trace_test

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/starbops/controllerless/internal/trace"
)

func newTestID(t *testing.T) ulid.ULID {
	t.Helper()
	return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
}

func readLines(t *testing.T, tracesDir string) []string {
	t.Helper()
	dateDir := time.Now().UTC().Format("2006-01-02")
	entries, err := os.ReadDir(filepath.Join(tracesDir, dateDir))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 trace file, got %d", len(entries))
	}
	f, err := os.Open(filepath.Join(tracesDir, dateDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestNewWriter_createsDirectoryStructure(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "default", "apps/v1/Deployment", "my-deploy", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	dateDir := time.Now().UTC().Format("2006-01-02")
	entries, err := os.ReadDir(filepath.Join(dir, dateDir))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasSuffix(name, ".jsonl") {
		t.Errorf("expected .jsonl suffix, got %q", name)
	}
	if !strings.Contains(name, "apps-v1-Deployment") {
		t.Errorf("expected gvk slug in filename, got %q", name)
	}
	if !strings.HasPrefix(name, "default__") {
		t.Errorf("expected namespace prefix in filename, got %q", name)
	}
}

func TestNewWriter_coreGroupGVK(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "kube-system", "/v1/Pod", "test-pod", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	defer w.Close()

	dateDir := time.Now().UTC().Format("2006-01-02")
	entries, _ := os.ReadDir(filepath.Join(dir, dateDir))
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.Contains(name, "-v1-Pod") {
		t.Errorf("expected core gvk slug (-v1-Pod) in filename, got %q", name)
	}
}

func TestWriter_Write_validJSONLine(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "default", "/v1/Pod", "my-pod", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Write("skill_start", map[string]any{"skill": "test-skill"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["reconcileId"] != id.String() {
		t.Errorf("reconcileId: got %v, want %v", entry["reconcileId"], id.String())
	}
	if entry["phase"] != "skill_start" {
		t.Errorf("phase: got %v", entry["phase"])
	}
	if entry["skill"] != "test-skill" {
		t.Errorf("payload field 'skill' missing or wrong: %v", entry)
	}
	if _, ok := entry["t"]; !ok {
		t.Error("missing 't' field")
	}
}

func TestWriter_Write_multipleLines(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "default", "/v1/Pod", "my-pod", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := range 3 {
		if err := w.Write("tool_call", map[string]any{"n": i}); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, dir)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestWriter_Write_nilPayload(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "default", "/v1/Pod", "my-pod", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Write("skill_complete", nil); err != nil {
		t.Fatalf("Write nil payload: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := readLines(t, dir)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if entry["phase"] != "skill_complete" {
		t.Errorf("phase: got %v", entry["phase"])
	}
}

func TestWriter_Close_noError(t *testing.T) {
	dir := t.TempDir()
	id := newTestID(t)

	w, err := trace.NewWriter(dir, "ns", "/v1/Pod", "pod", id)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
