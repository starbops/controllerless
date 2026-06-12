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
)

// reconcile runs the agentic tool-call loop for a single (skill, event) pair.
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
) (any, error) {
	reconcileID := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	slog.Info("reconcile: start", "skill", s.Frontmatter.Name, "reconcileId", reconcileID)

	prompt := buildPrompt(s, eventYAML)

	toolDefs := reg.Subset(s.Frontmatter.AllowedTools)
	llmTools := toProviderTools(toolDefs)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := prov.Chat(ctx, llm.ChatRequest{
			Messages: messages,
			Tools:    llmTools,
		})
		if err != nil {
			return nil, fmt.Errorf("reconcile: chat turn %d: %w", turn, err)
		}

		messages = append(messages, resp.Message)

		if resp.StopReason != llm.StopReasonToolUse || len(resp.Message.ToolCalls) == 0 {
			// Model stopped without calling a tool — treat as a turn with no action.
			continue
		}

		for _, tc := range resp.Message.ToolCalls {
			result, callErr := reg.Call(ctx, tc.Name, tc.Arguments)

			// Check for terminal signals before handling errors.
			switch sig := result.(type) {
			case tools.DoneSignal:
				return sig, nil
			case tools.RequeueSignal:
				return sig, nil
			}

			var toolContent string
			if callErr != nil {
				toolContent = fmt.Sprintf("error: %s", callErr.Error())
			} else {
				toolContent = fmt.Sprintf("%v", result)
			}

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
