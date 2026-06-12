package dispatch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/starbops/controllerless/internal/llm"
	"github.com/starbops/controllerless/internal/llm/providers/mock"
	"github.com/starbops/controllerless/internal/skill"
	"github.com/starbops/controllerless/internal/tools"
)

func makeToolReg() *tools.Registry {
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)
	return reg
}

func makeSkill(name string) skill.Skill {
	return skill.Skill{
		Frontmatter: skill.Frontmatter{
			Name:         name,
			AllowedTools: []string{"done", "requeueAfter"},
		},
		Body: "Do the thing.",
	}
}

// scriptedDone returns a mock script: one tool_use for done(), then done response.
func scriptedDone() []llm.ChatResponse {
	return []llm.ChatResponse{
		{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        "tc-1",
						Name:      "done",
						Arguments: json.RawMessage(`{"summary":"all good"}`),
					},
				},
			},
			StopReason: llm.StopReasonToolUse,
		},
	}
}

// scriptedRequeue returns a mock script: one tool_use for requeueAfter().
func scriptedRequeue() []llm.ChatResponse {
	return []llm.ChatResponse{
		{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        "tc-1",
						Name:      "requeueAfter",
						Arguments: json.RawMessage(`{"seconds":30,"reason":"not ready"}`),
					},
				},
			},
			StopReason: llm.StopReasonToolUse,
		},
	}
}

// scriptedMaxTurns returns more responses than maxTurns allows.
func scriptedMaxTurns(n int) []llm.ChatResponse {
	resp := llm.ChatResponse{
		Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "thinking...",
		},
		StopReason: llm.StopReasonStop,
	}
	script := make([]llm.ChatResponse, n+2)
	for i := range script {
		script[i] = resp
	}
	return script
}

func TestReconcile_DoneTerminatesLoop(t *testing.T) {
	prov := mock.New(scriptedDone())
	reg := makeToolReg()
	s := makeSkill("my-skill")

	sig, err := reconcile(context.Background(), s, "kind: Pod\n", prov, reg, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	done, ok := sig.(tools.DoneSignal)
	if !ok {
		t.Fatalf("expected DoneSignal, got %T", sig)
	}
	if done.Summary == "" {
		t.Error("DoneSignal.Summary should not be empty")
	}
}

func TestReconcile_RequeueTerminatesLoop(t *testing.T) {
	prov := mock.New(scriptedRequeue())
	reg := makeToolReg()
	s := makeSkill("my-skill")

	sig, err := reconcile(context.Background(), s, "kind: Pod\n", prov, reg, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req, ok := sig.(tools.RequeueSignal)
	if !ok {
		t.Fatalf("expected RequeueSignal, got %T", sig)
	}
	if req.Seconds != 30 {
		t.Errorf("expected 30s, got %d", req.Seconds)
	}
}

func TestReconcile_MaxTurnsGuard(t *testing.T) {
	const maxTurns = 3
	prov := mock.New(scriptedMaxTurns(maxTurns))
	reg := makeToolReg()
	s := makeSkill("my-skill")

	sig, err := reconcile(context.Background(), s, "kind: Pod\n", prov, reg, maxTurns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sig != nil {
		t.Errorf("expected nil signal on max-turns exit, got %T", sig)
	}
}
