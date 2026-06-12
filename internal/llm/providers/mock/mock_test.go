package mock_test

import (
	"context"
	"testing"

	"github.com/starbops/controllerless/internal/llm"
	"github.com/starbops/controllerless/internal/llm/providers/mock"
)

func TestMock_ScriptedResponses(t *testing.T) {
	script := []llm.ChatResponse{
		{
			Message:    llm.Message{Role: llm.RoleAssistant, Content: "first"},
			StopReason: llm.StopReasonStop,
		},
		{
			Message:    llm.Message{Role: llm.RoleAssistant, Content: "second"},
			StopReason: llm.StopReasonToolUse,
		},
	}

	p := mock.New(script)

	req := llm.ChatRequest{Model: "any", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}}}

	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if resp.Message.Content != "first" {
		t.Errorf("expected first, got %q", resp.Message.Content)
	}
	if resp.StopReason != llm.StopReasonStop {
		t.Errorf("expected StopReasonStop, got %q", resp.StopReason)
	}

	resp, err = p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if resp.Message.Content != "second" {
		t.Errorf("expected second, got %q", resp.Message.Content)
	}
	if resp.StopReason != llm.StopReasonToolUse {
		t.Errorf("expected StopReasonToolUse, got %q", resp.StopReason)
	}
}

func TestMock_ErrorAfterScriptExhausted(t *testing.T) {
	p := mock.New([]llm.ChatResponse{
		{Message: llm.Message{Content: "only"}, StopReason: llm.StopReasonStop},
	})

	req := llm.ChatRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}}

	if _, err := p.Chat(context.Background(), req); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if _, err := p.Chat(context.Background(), req); err == nil {
		t.Fatal("expected error after script exhausted, got nil")
	}
}

func TestMock_Name(t *testing.T) {
	p := mock.New(nil)
	if p.Name() != "mock" {
		t.Errorf("expected mock, got %q", p.Name())
	}
}
