package dispatch

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/starbops/controllerless/internal/llm"
	"github.com/starbops/controllerless/internal/skill"
	"github.com/starbops/controllerless/internal/tools"
	"github.com/starbops/controllerless/internal/trace"
)

// reconcile runs the agentic tool-call loop for a single (skill, event) pair.
//
// If tracesDir is non-empty, a per-reconcile JSONL trace file is written under
// <tracesDir>/<date>/<namespace>__<gvk-slug>__<name>__<unix-ms>.jsonl.
//
// It returns the terminal signal (DoneSignal or RequeueSignal) if the LLM
// signals completion, or nil if the max-turns guard fires.
func reconcile(
	ctx context.Context,
	s skill.Skill,
	eventYAML string,
	prov llm.Provider,
	reg *tools.Registry,
	maxTurns int,
	tracesDir, namespace, gvk, name string,
) (any, error) {
	reconcileID := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	start := time.Now()
	slog.Info("reconcile: start", "skill", s.Frontmatter.Name, "reconcileId", reconcileID)

	// Open trace writer if tracesDir is configured (best-effort: failure does not abort).
	var tw *trace.Writer
	if tracesDir != "" {
		var err error
		tw, err = trace.NewWriter(tracesDir, namespace, gvk, name, reconcileID)
		if err != nil {
			slog.Warn("reconcile: trace writer unavailable", "err", err)
		} else {
			defer tw.Close()
			traceWrite(tw, "skill_start", map[string]any{"skill": s.Frontmatter.Name})
		}
	}

	prompt := buildPrompt(s, eventYAML)
	toolDefs := reg.Subset(s.Frontmatter.AllowedTools)
	llmTools := toProviderTools(toolDefs)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}

	var totalToolCalls int

	for turn := 0; turn < maxTurns; turn++ {
		traceWrite(tw, "llm_request", map[string]any{
			"turn":     turn,
			"messages": len(messages),
			"tools":    len(llmTools),
		})

		resp, err := prov.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    llmTools,
		})
		if err != nil {
			return nil, fmt.Errorf("reconcile: chat turn %d: %w", turn, err)
		}

		traceWrite(tw, "llm_response", map[string]any{
			"stopReason": resp.StopReason,
			"toolCalls":  len(resp.Message.ToolCalls),
			"usage": map[string]any{
				"promptTokens":     resp.Usage.PromptTokens,
				"completionTokens": resp.Usage.CompletionTokens,
				"totalMs":          resp.Usage.TotalMs,
			},
		})

		messages = append(messages, resp.Message)

		if resp.StopReason != llm.StopReasonToolUse || len(resp.Message.ToolCalls) == 0 {
			continue
		}

		for _, tc := range resp.Message.ToolCalls {
			callStart := time.Now()
			result, callErr := reg.Call(ctx, tc.Name, tc.Arguments)
			durationMs := time.Since(callStart).Milliseconds()
			totalToolCalls++

			// Check for terminal signals before handling errors.
			switch sig := result.(type) {
			case tools.DoneSignal:
				traceWrite(tw, "tool_call", map[string]any{
					"id": tc.ID, "name": tc.Name, "args": tc.Arguments,
					"result": sig.Summary, "durationMs": durationMs,
				})
				traceWrite(tw, "skill_complete", map[string]any{
					"outcome":         "done",
					"summary":         sig.Summary,
					"totalDurationMs": time.Since(start).Milliseconds(),
					"llmTurns":        turn + 1,
					"toolCalls":       totalToolCalls,
				})
				return sig, nil
			case tools.RequeueSignal:
				traceWrite(tw, "tool_call", map[string]any{
					"id":   tc.ID,
					"name": tc.Name,
					"args": tc.Arguments,
					"result": fmt.Sprintf("requeue after %ds: %s", sig.Seconds, sig.Reason),
					"durationMs": durationMs,
				})
				traceWrite(tw, "requeue", map[string]any{
					"afterSeconds": sig.Seconds,
					"reason":       sig.Reason,
				})
				return sig, nil
			}

			var toolContent string
			if callErr != nil {
				toolContent = fmt.Sprintf("error: %s", callErr.Error())
			} else {
				toolContent = fmt.Sprintf("%v", result)
			}

			traceWrite(tw, "tool_call", map[string]any{
				"id":         tc.ID,
				"name":       tc.Name,
				"args":       tc.Arguments,
				"result":     toolContent,
				"durationMs": durationMs,
			})

			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				ToolCallID: tc.ID,
				Name:       tc.Name,
				Content:    toolContent,
			})
		}
	}

	// Max-turns guard fired — return nil signal (caller logs/handles).
	return nil, nil
}

// traceWrite is a best-effort helper; failures are logged but do not abort reconciliation.
func traceWrite(w *trace.Writer, phase string, payload any) {
	if w == nil {
		return
	}
	if err := w.Write(phase, payload); err != nil {
		slog.Warn("trace: write failed", "phase", phase, "err", err)
	}
}

// toProviderTools converts tools.ToolDef → llm.ToolDef.
func toProviderTools(defs []tools.ToolDef) []llm.ToolDef {
	out := make([]llm.ToolDef, len(defs))
	for i, d := range defs {
		out[i] = llm.ToolDef{
			Name:        d.Name,
			Description: d.Description,
			Schema:      d.Schema,
		}
	}
	return out
}
