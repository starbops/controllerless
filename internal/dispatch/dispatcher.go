// Package dispatch implements the agentic reconcile loop and multi-skill dispatcher.
package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/starbops/controllerless/internal/kube"
	"github.com/starbops/controllerless/internal/llm"
	"github.com/starbops/controllerless/internal/skill"
	"github.com/starbops/controllerless/internal/tools"
)

const defaultMaxTurns = 20

// Deps holds all external dependencies wired into a Dispatcher.
type Deps struct {
	Kube       *kube.Clients
	WQRegistry *kube.Registry
	Skills     []skill.Skill
	Tools      *tools.Registry
	Provider   llm.Provider
	TracesDir  string
}

// Dispatcher evaluates skills against incoming events and runs the agentic loop.
type Dispatcher struct {
	deps    Deps
	limiter *rateLimiter
}

// New creates a Dispatcher from the given Deps.
func New(deps Deps) *Dispatcher {
	return &Dispatcher{
		deps:    deps,
		limiter: newRateLimiter(),
	}
}

// Dispatch evaluates each skill against the event (S4 CEL filter + S3 rate limit),
// then runs the agentic tool-call loop for each matching skill.
//
// gvk is "group/version/Kind" (e.g., "/v1/Pod", "apps/v1/Deployment").
// eventType is "Add", "Modify", or "Delete".
// resourceKey is "namespace/name".
// obj is the unstructured Kubernetes object.
func (d *Dispatcher) Dispatch(ctx context.Context, gvk, eventType, resourceKey string, obj map[string]any) error {
	matched := d.matchSkills(gvk, eventType, obj)
	for _, s := range matched {
		// S3: per-(resource, skill) rate limit.
		if !d.limiter.allow(resourceKey, s.Frontmatter.Name) {
			slog.Info("dispatch: rate-limited, skipping",
				"skill", s.Frontmatter.Name,
				"key", resourceKey,
			)
			continue
		}

		ns, resName := splitResourceKey(resourceKey)
		eventYAML := objectToYAML(obj)
		sig, err := reconcile(ctx, s, eventYAML, d.deps.Provider, d.deps.Tools, defaultMaxTurns,
			d.deps.TracesDir, ns, gvk, resName)
		if err != nil {
			return fmt.Errorf("dispatch: reconcile skill %q: %w", s.Frontmatter.Name, err)
		}

		slog.Info("dispatch: reconcile complete",
			"skill", s.Frontmatter.Name,
			"key", resourceKey,
			"signal", fmt.Sprintf("%T", sig),
		)
	}
	return nil
}

// matchSkills returns the skills that match gvk + eventType and pass CEL conditions (S4).
func (d *Dispatcher) matchSkills(gvk, eventType string, obj map[string]any) []skill.Skill {
	var matched []skill.Skill
	for _, s := range d.deps.Skills {
		for _, trig := range s.Frontmatter.Triggers {
			if trig.GVK != gvk {
				continue
			}
			if !containsEventType(trig.EventTypes, eventType) {
				continue
			}
			// Evaluate each CEL condition; all must be true.
			allTrue := true
			for _, cond := range trig.Conditions {
				ok, err := skill.EvalTrigger(cond, obj)
				if err != nil {
					slog.Warn("dispatch: CEL eval error, skipping skill",
						"skill", s.Frontmatter.Name, "condition", cond, "err", err)
					allTrue = false
					break
				}
				if !ok {
					allTrue = false
					break
				}
			}
			if allTrue {
				matched = append(matched, s)
				break
			}
		}
	}
	return matched
}

// splitResourceKey parses "namespace/name" into its parts.
// For cluster-scoped resources, the key may be just "name" with no slash.
func splitResourceKey(key string) (namespace, name string) {
	if i := strings.IndexByte(key, '/'); i >= 0 {
		return key[:i], key[i+1:]
	}
	return "", key
}

func containsEventType(list []string, t string) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}

// objectToYAML produces a simple YAML-like representation of the object.
// A real implementation would use sigs.k8s.io/yaml; this version avoids
// adding a new dependency and is sufficient for prompt assembly in tests.
func objectToYAML(obj map[string]any) string {
	var b strings.Builder
	writeYAML(&b, obj, 0)
	return b.String()
}

func writeYAML(b *strings.Builder, v any, indent int) {
	pad := strings.Repeat("  ", indent)
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			switch c := child.(type) {
			case map[string]any:
				fmt.Fprintf(b, "%s%s:\n", pad, k)
				writeYAML(b, c, indent+1)
			case []any:
				fmt.Fprintf(b, "%s%s:\n", pad, k)
				for _, item := range c {
					fmt.Fprintf(b, "%s- ", pad)
					writeYAML(b, item, indent+1)
				}
			default:
				fmt.Fprintf(b, "%s%s: %v\n", pad, k, child)
			}
		}
	default:
		fmt.Fprintf(b, "%v\n", val)
	}
}
