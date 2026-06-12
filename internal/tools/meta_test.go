package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/starbops/controllerless/internal/tools"
)

func newMetaReg(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)
	return reg
}

func TestMeta_Done_ReturnsDoneSignal(t *testing.T) {
	reg := newMetaReg(t)

	raw, _ := json.Marshal(map[string]any{"summary": "all done"})
	result, err := reg.Call(context.Background(), "done", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sig, ok := result.(tools.DoneSignal)
	if !ok {
		t.Fatalf("expected tools.DoneSignal, got %T", result)
	}
	if sig.Summary != "all done" {
		t.Fatalf("expected summary 'all done', got %q", sig.Summary)
	}
}

func TestMeta_RequeueAfter_ReturnsRequeueSignal(t *testing.T) {
	reg := newMetaReg(t)

	raw, _ := json.Marshal(map[string]any{"seconds": 30, "reason": "not yet due"})
	result, err := reg.Call(context.Background(), "requeueAfter", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sig, ok := result.(tools.RequeueSignal)
	if !ok {
		t.Fatalf("expected tools.RequeueSignal, got %T", result)
	}
	if sig.Seconds != 30 {
		t.Fatalf("expected seconds=30, got %d", sig.Seconds)
	}
	if sig.Reason != "not yet due" {
		t.Fatalf("expected reason 'not yet due', got %q", sig.Reason)
	}
}

func TestMeta_DoneSignal_IsDistinguishable(t *testing.T) {
	// The agentic loop detects terminal signals via type assertion, not error unwrapping.
	// Verify both signal types are exported and distinguishable.
	var done tools.DoneSignal
	var requeue tools.RequeueSignal
	var iface any = done
	if _, ok := iface.(tools.DoneSignal); !ok {
		t.Fatal("DoneSignal type assertion failed")
	}
	iface = requeue
	if _, ok := iface.(tools.RequeueSignal); !ok {
		t.Fatal("RequeueSignal type assertion failed")
	}
}

func TestMeta_Done_EmptySummary(t *testing.T) {
	reg := newMetaReg(t)
	raw, _ := json.Marshal(map[string]any{"summary": ""})
	result, err := reg.Call(context.Background(), "done", raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sig, ok := result.(tools.DoneSignal)
	if !ok {
		t.Fatalf("expected DoneSignal, got %T", result)
	}
	if sig.Summary != "" {
		t.Fatalf("expected empty summary, got %q", sig.Summary)
	}
}
