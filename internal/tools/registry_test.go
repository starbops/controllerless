package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/starbops/controllerless/internal/tools"
)

func TestRegistry_CallDispatchesCorrectly(t *testing.T) {
	reg := tools.NewRegistry()

	called := false
	reg.Register(tools.ToolDef{
		Name:        "echo",
		Description: "returns the input",
		Schema:      json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		called = true
		var p struct{ Msg string }
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return p.Msg, nil
	})

	result, err := reg.Call(context.Background(), "echo", json.RawMessage(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got %v", result)
	}
}

func TestRegistry_CallUnknownToolReturnsError(t *testing.T) {
	reg := tools.NewRegistry()

	_, err := reg.Call(context.Background(), "nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestRegistry_DefsReturnsAll(t *testing.T) {
	reg := tools.NewRegistry()

	reg.Register(tools.ToolDef{Name: "a", Description: "tool a", Schema: json.RawMessage(`{}`)}, func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil })
	reg.Register(tools.ToolDef{Name: "b", Description: "tool b", Schema: json.RawMessage(`{}`)}, func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil })

	defs := reg.Defs()
	if len(defs) != 2 {
		t.Fatalf("expected 2 defs, got %d", len(defs))
	}

	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["a"] || !names["b"] {
		t.Fatalf("missing expected tool defs, got %v", names)
	}
}

func TestRegistry_SubsetFilters(t *testing.T) {
	reg := tools.NewRegistry()

	for _, name := range []string{"a", "b", "c"} {
		n := name
		reg.Register(tools.ToolDef{Name: n, Description: n, Schema: json.RawMessage(`{}`)}, func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil })
	}

	subset := reg.Subset([]string{"a", "c"})
	if len(subset) != 2 {
		t.Fatalf("expected 2 subset defs, got %d", len(subset))
	}

	for _, d := range subset {
		if d.Name != "a" && d.Name != "c" {
			t.Fatalf("unexpected tool in subset: %s", d.Name)
		}
	}
}

func TestRegistry_SubsetUnknownNamesIgnored(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.ToolDef{Name: "a", Description: "a", Schema: json.RawMessage(`{}`)}, func(_ context.Context, _ json.RawMessage) (any, error) { return nil, nil })

	subset := reg.Subset([]string{"a", "does-not-exist"})
	if len(subset) != 1 {
		t.Fatalf("expected 1 subset def, got %d", len(subset))
	}
}

func TestRegistry_HandlerErrorPropagates(t *testing.T) {
	reg := tools.NewRegistry()
	sentinel := errors.New("boom")

	reg.Register(tools.ToolDef{Name: "fail", Description: "always fails", Schema: json.RawMessage(`{}`)}, func(_ context.Context, _ json.RawMessage) (any, error) {
		return nil, sentinel
	})

	_, err := reg.Call(context.Background(), "fail", json.RawMessage(`{}`))
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}
