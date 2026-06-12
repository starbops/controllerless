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

// matchingSkill triggers on /v1/Pod Add with no conditions.
func matchingSkill() skill.Skill {
	return skill.Skill{
		Frontmatter: skill.Frontmatter{
			Name: "pod-skill",
			Triggers: []skill.TriggerSpec{
				{GVK: "/v1/Pod", EventTypes: []string{"Add"}},
			},
			AllowedTools: []string{"done", "requeueAfter"},
		},
		Body: "Handle pods.",
	}
}

// nonMatchingSkill triggers on apps/v1/Deployment — will not fire for Pod Add.
func nonMatchingSkill() skill.Skill {
	return skill.Skill{
		Frontmatter: skill.Frontmatter{
			Name: "deploy-skill",
			Triggers: []skill.TriggerSpec{
				{GVK: "apps/v1/Deployment", EventTypes: []string{"Add"}},
			},
			AllowedTools: []string{"done"},
		},
		Body: "Handle deployments.",
	}
}

// celFalseSkill triggers on /v1/Pod but has a CEL condition that is always false.
func celFalseSkill() skill.Skill {
	return skill.Skill{
		Frontmatter: skill.Frontmatter{
			Name: "cel-false-skill",
			Triggers: []skill.TriggerSpec{
				{
					GVK:        "/v1/Pod",
					EventTypes: []string{"Add"},
					Conditions: []string{"false"},
				},
			},
			AllowedTools: []string{"done"},
		},
		Body: "Never runs.",
	}
}

func doneMockScript() []llm.ChatResponse {
	return []llm.ChatResponse{
		{
			Message: llm.Message{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{
					{
						ID:        "tc-1",
						Name:      "done",
						Arguments: json.RawMessage(`{"summary":"done"}`),
					},
				},
			},
			StopReason: llm.StopReasonToolUse,
		},
	}
}

func TestDispatch_MatchingSkillIsReconciled(t *testing.T) {
	prov := mock.New(doneMockScript())
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)

	d := New(Deps{
		Skills:   []skill.Skill{matchingSkill()},
		Tools:    reg,
		Provider: prov,
	})

	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "my-pod", "namespace": "default"},
	}
	if err := d.Dispatch(context.Background(), "/v1/Pod", "Add", "default/my-pod", obj); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
}

func TestDispatch_NonMatchingGVKSkipped(t *testing.T) {
	// Script is empty — if the non-matching skill were reconciled, it would exhaust the mock.
	prov := mock.New([]llm.ChatResponse{})
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)

	d := New(Deps{
		Skills:   []skill.Skill{nonMatchingSkill()},
		Tools:    reg,
		Provider: prov,
	})

	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "my-pod", "namespace": "default"},
	}
	if err := d.Dispatch(context.Background(), "/v1/Pod", "Add", "default/my-pod", obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatch_CELFalseConditionSkipped(t *testing.T) {
	prov := mock.New([]llm.ChatResponse{})
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)

	d := New(Deps{
		Skills:   []skill.Skill{celFalseSkill()},
		Tools:    reg,
		Provider: prov,
	})

	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "my-pod", "namespace": "default"},
	}
	if err := d.Dispatch(context.Background(), "/v1/Pod", "Add", "default/my-pod", obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
