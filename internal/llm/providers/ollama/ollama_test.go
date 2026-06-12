package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/starbops/controllerless/internal/llm"
	"github.com/starbops/controllerless/internal/llm/providers/ollama"
)

func TestOllama_Name(t *testing.T) {
	p := ollama.New(ollama.Config{BaseURL: "http://localhost:11434", Model: "gemma4:12b-mxfp8"})
	if p.Name() != "ollama" {
		t.Errorf("expected ollama, got %q", p.Name())
	}
}

func TestOllama_Chat_RequestShape(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"model": "gemma4:12b-mxfp8",
			"message": map[string]any{
				"role":    "assistant",
				"content": "hello",
			},
			"done_reason": "stop",
			"done":        true,
			"prompt_eval_count": 10,
			"eval_count":        5,
			"total_duration":    1000000000,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := ollama.New(ollama.Config{BaseURL: srv.URL, Model: "gemma4:12b-mxfp8"})

	req := llm.ChatRequest{
		Model: "gemma4:12b-mxfp8",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "hello"},
		},
		Temperature: 0.7,
		MaxTokens:   512,
	}
	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify response mapping
	if resp.Message.Content != "hello" {
		t.Errorf("expected hello, got %q", resp.Message.Content)
	}
	if resp.StopReason != llm.StopReasonStop {
		t.Errorf("expected StopReasonStop, got %q", resp.StopReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected PromptTokens=10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected CompletionTokens=5, got %d", resp.Usage.CompletionTokens)
	}

	// verify request shape
	if captured["model"] != "gemma4:12b-mxfp8" {
		t.Errorf("expected model gemma4:12b-mxfp8 in request, got %v", captured["model"])
	}
	if captured["stream"] != false {
		t.Errorf("expected stream=false, got %v", captured["stream"])
	}
	msgs, ok := captured["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Errorf("expected 1 message in request, got %v", captured["messages"])
	}
}

func TestOllama_Chat_ToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"model": "gemma4:12b-mxfp8",
			"message": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []any{
					map[string]any{
						"function": map[string]any{
							"name":      "get_pod",
							"arguments": map[string]any{"name": "my-pod", "namespace": "default"},
						},
					},
				},
			},
			"done_reason": "stop",
			"done":        true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := ollama.New(ollama.Config{BaseURL: srv.URL, Model: "gemma4:12b-mxfp8"})
	resp, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "get pod"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	if resp.Message.ToolCalls[0].Name != "get_pod" {
		t.Errorf("expected get_pod, got %q", resp.Message.ToolCalls[0].Name)
	}
}

func TestOllama_Chat_InvalidURL(t *testing.T) {
	p := ollama.New(ollama.Config{BaseURL: "http://127.0.0.1:1", Model: "test"})
	_, err := p.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

// Ensure the provider satisfies the llm.Provider interface at compile time.
func TestOllama_ImplementsProvider(t *testing.T) {
	_ = url.URL{}
	var _ llm.Provider = ollama.New(ollama.Config{})
}
