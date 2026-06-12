package dispatch

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/starbops/controllerless/internal/kube"
	"github.com/starbops/controllerless/internal/llm/providers/mock"
	"github.com/starbops/controllerless/internal/skill"
	"github.com/starbops/controllerless/internal/tools"
)

// newFakeMapper returns a DefaultRESTMapper pre-populated with the given mappings.
func newFakeMapper(entries []schema.GroupVersionKind) meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper(nil)
	for _, gvk := range entries {
		plural, _ := meta.UnsafeGuessKindToResource(gvk)
		mapper.AddSpecific(gvk, plural, schema.GroupVersionResource{
			Group:    gvk.Group,
			Version:  gvk.Version,
			Resource: gvk.Kind + "s",
		}, meta.RESTScopeRoot)
	}
	return mapper
}

// scheduledTaskGVK is the GVK for the ScheduledTask CRD used in skills.
var scheduledTaskGVK = schema.GroupVersionKind{
	Group:   "agentic.io",
	Version: "v1alpha1",
	Kind:    "ScheduledTask",
}

// scheduledTaskSkill returns a skill that triggers on ScheduledTask Add events.
func scheduledTaskSkill() skill.Skill {
	return skill.Skill{
		Frontmatter: skill.Frontmatter{
			Name: "st-skill",
			Triggers: []skill.TriggerSpec{
				{GVK: "agentic.io/v1alpha1/ScheduledTask", EventTypes: []string{"Add"}},
			},
			AllowedTools: []string{"done"},
		},
		Body: "Handle ScheduledTask.",
	}
}

// TestUpdateSkills_ReplacesSkillsAtomically verifies that UpdateSkills swaps
// the skills slice and that subsequent Dispatch calls use the new set.
func TestUpdateSkills_ReplacesSkillsAtomically(t *testing.T) {
	prov := mock.New(doneMockScript())
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)

	d := New(Deps{
		Skills:   []skill.Skill{matchingSkill()},
		Tools:    reg,
		Provider: prov,
	})

	// Replace with a non-matching skill — subsequent Dispatch for Pod should be a no-op.
	d.UpdateSkills([]skill.Skill{nonMatchingSkill()})

	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "p", "namespace": "default"},
	}
	// The mock has one response queued. If the old skill were still there, Dispatch
	// would consume it. If it panics or errors, the update wasn't applied correctly.
	if err := d.Dispatch(context.Background(), "/v1/Pod", "Add", "default/p", obj); err != nil {
		t.Fatalf("Dispatch after UpdateSkills returned error: %v", err)
	}
}

// TestUpdateSkills_ConcurrentSafe fires UpdateSkills and matchSkills concurrently
// to surface data races (run with -race).
func TestUpdateSkills_ConcurrentSafe(t *testing.T) {
	prov := mock.New(doneMockScript())
	reg := tools.NewRegistry()
	tools.RegisterMetaTools(reg)

	d := New(Deps{
		Skills:   []skill.Skill{matchingSkill()},
		Tools:    reg,
		Provider: prov,
	})

	var done int32
	go func() {
		for atomic.LoadInt32(&done) == 0 {
			d.UpdateSkills([]skill.Skill{matchingSkill(), nonMatchingSkill()})
		}
	}()

	for i := 0; i < 100; i++ {
		_ = d.matchSkills("/v1/Pod", "Add", map[string]any{})
	}
	atomic.StoreInt32(&done, 1)
}

// TestParseGVR_CoreGroup verifies that "/v1/Pod" resolves via the REST mapper.
func TestParseGVR_CoreGroup(t *testing.T) {
	podGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}
	mapper := newFakeMapper([]schema.GroupVersionKind{podGVK})

	gvr, err := parseGVR(mapper, "/v1/Pod")
	if err != nil {
		t.Fatalf("parseGVR returned error: %v", err)
	}
	if gvr.Version != "v1" {
		t.Errorf("expected version v1, got %q", gvr.Version)
	}
}

// TestParseGVR_CustomGroup verifies that "agentic.io/v1alpha1/ScheduledTask" resolves.
func TestParseGVR_CustomGroup(t *testing.T) {
	mapper := newFakeMapper([]schema.GroupVersionKind{scheduledTaskGVK})

	gvr, err := parseGVR(mapper, "agentic.io/v1alpha1/ScheduledTask")
	if err != nil {
		t.Fatalf("parseGVR returned error: %v", err)
	}
	if gvr.Group != "agentic.io" {
		t.Errorf("expected group agentic.io, got %q", gvr.Group)
	}
}

// TestParseGVR_InvalidString verifies that a malformed GVK string returns an error.
func TestParseGVR_InvalidString(t *testing.T) {
	mapper := newFakeMapper(nil)
	if _, err := parseGVR(mapper, "justonepart"); err == nil {
		t.Fatal("expected error for invalid GVK string, got nil")
	}
}

// TestRun_ExitsOnContextCancel verifies that Run returns after ctx is cancelled.
// No skills → no informers needed → no real cluster required.
func TestRun_ExitsOnContextCancel(t *testing.T) {
	wqReg := kube.NewRegistry()
	d := New(Deps{
		WQRegistry: wqReg,
		Skills:     []skill.Skill{},
		Tools:      tools.NewRegistry(),
		Provider:   mock.New(nil),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.Run(ctx, nil) // factory unused when no skills
	}()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
