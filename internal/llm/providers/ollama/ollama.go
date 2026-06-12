// Package ollama implements the llm.Provider interface using the native Ollama client.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	ollamaapi "github.com/ollama/ollama/api"
	"github.com/starbops/controllerless/internal/llm"
)

// Config holds Ollama provider configuration.
type Config struct {
	// BaseURL is the Ollama server URL (e.g. "http://localhost:11434").
	BaseURL string
	// Model is the default model to use when ChatRequest.Model is empty.
	Model string
	// Temperature overrides ChatRequest.Temperature when non-zero.
	Temperature float32
	// MaxTokens overrides ChatRequest.MaxTokens when non-zero.
	MaxTokens int
	// Timeout is the per-request HTTP timeout (0 = no timeout).
	Timeout time.Duration
	// NumCtx is the context window size passed as an option.
	NumCtx int
}

// Provider is an llm.Provider backed by a local Ollama instance.
type Provider struct {
	client *ollamaapi.Client
	cfg    Config
}

// New constructs a Provider from Config.
func New(cfg Config) *Provider {
	base := cfg.BaseURL
	if base == "" {
		base = "http://localhost:11434"
	}
	u, err := url.Parse(base)
	if err != nil {
		// Fallback — will fail at request time with a clear error.
		u, _ = url.Parse("http://localhost:11434")
	}
	httpClient := &http.Client{}
	if cfg.Timeout > 0 {
		httpClient.Timeout = cfg.Timeout
	}
	return &Provider{
		client: ollamaapi.NewClient(u, httpClient),
		cfg:    cfg,
	}
}

// Name implements llm.Provider.
func (p *Provider) Name() string { return "ollama" }

// Chat implements llm.Provider using the native /api/chat endpoint.
func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	msgs, err := toOllamaMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal messages: %w", err)
	}

	tools, err := toOllamaTools(req.Tools)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal tools: %w", err)
	}

	opts := buildOptions(p.cfg, req)

	stream := false
	apiReq := &ollamaapi.ChatRequest{
		Model:    model,
		Messages: msgs,
		Tools:    tools,
		Stream:   &stream,
		Options:  opts,
	}

	start := time.Now()
	var result *llm.ChatResponse

	err = p.client.Chat(ctx, apiReq, func(resp ollamaapi.ChatResponse) error {
		if !resp.Done {
			return nil
		}
		result = fromOllamaResponse(resp, time.Since(start))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: chat: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("ollama: no response received")
	}
	return result, nil
}

func toOllamaMessages(msgs []llm.Message) ([]ollamaapi.Message, error) {
	out := make([]ollamaapi.Message, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaapi.Message{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			args := ollamaapi.NewToolCallFunctionArguments()
			var raw map[string]any
			if len(tc.Arguments) > 0 {
				if err := json.Unmarshal(tc.Arguments, &raw); err != nil {
					return nil, err
				}
				for k, v := range raw {
					args.Set(k, v)
				}
			}
			om.ToolCalls = append(om.ToolCalls, ollamaapi.ToolCall{
				Function: ollamaapi.ToolCallFunction{
					Name:      tc.Name,
					Arguments: args,
				},
			})
		}
		out = append(out, om)
	}
	return out, nil
}

func toOllamaTools(tools []llm.ToolDef) (ollamaapi.Tools, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make(ollamaapi.Tools, 0, len(tools))
	for _, td := range tools {
		var params ollamaapi.ToolFunctionParameters
		if len(td.Schema) > 0 {
			if err := json.Unmarshal(td.Schema, &params); err != nil {
				return nil, fmt.Errorf("tool %q schema: %w", td.Name, err)
			}
		}
		out = append(out, ollamaapi.Tool{
			Type: "function",
			Function: ollamaapi.ToolFunction{
				Name:        td.Name,
				Description: td.Description,
				Parameters:  params,
			},
		})
	}
	return out, nil
}

func buildOptions(cfg Config, req llm.ChatRequest) map[string]any {
	opts := map[string]any{}

	temp := req.Temperature
	if temp == 0 && cfg.Temperature != 0 {
		temp = cfg.Temperature
	}
	if temp != 0 {
		opts["temperature"] = float64(temp)
	}

	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = cfg.MaxTokens
	}
	if maxTok != 0 {
		opts["num_predict"] = maxTok
	}

	if cfg.NumCtx != 0 {
		opts["num_ctx"] = cfg.NumCtx
	}

	if len(opts) == 0 {
		return nil
	}
	return opts
}

func fromOllamaResponse(resp ollamaapi.ChatResponse, elapsed time.Duration) *llm.ChatResponse {
	msg := llm.Message{
		Role:    llm.Role(resp.Message.Role),
		Content: resp.Message.Content,
	}
	for _, tc := range resp.Message.ToolCalls {
		raw, _ := json.Marshal(tc.Function.Arguments)
		msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(raw),
		})
	}

	stop := mapStopReason(resp.DoneReason)

	totalMs := resp.TotalDuration.Milliseconds()
	if totalMs == 0 {
		totalMs = elapsed.Milliseconds()
	}

	return &llm.ChatResponse{
		Message:    msg,
		StopReason: stop,
		Usage: llm.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalMs:          totalMs,
		},
	}
}

func mapStopReason(reason string) llm.StopReason {
	switch reason {
	case "tool_calls":
		return llm.StopReasonToolUse
	case "length":
		return llm.StopReasonMaxTokens
	case "error":
		return llm.StopReasonError
	default:
		return llm.StopReasonStop
	}
}
