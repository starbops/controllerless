// Package tools provides the tool registry and all built-in tool implementations.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolDef describes a tool exposed to the LLM: its name, description, and JSON Schema.
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// HandlerFunc is the Go implementation of a tool.
type HandlerFunc func(ctx context.Context, args json.RawMessage) (any, error)

// Registry holds all registered tools and dispatches calls by name.
type Registry struct {
	defs     []ToolDef
	handlers map[string]HandlerFunc
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]HandlerFunc)}
}

// Register adds a tool definition and its handler to the registry.
func (r *Registry) Register(def ToolDef, fn HandlerFunc) {
	r.defs = append(r.defs, def)
	r.handlers[def.Name] = fn
}

// Call dispatches a tool call by name, returning its result or an error.
func (r *Registry) Call(ctx context.Context, name string, args json.RawMessage) (any, error) {
	fn, ok := r.handlers[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	return fn(ctx, args)
}

// Defs returns all registered ToolDefs.
func (r *Registry) Defs() []ToolDef {
	out := make([]ToolDef, len(r.defs))
	copy(out, r.defs)
	return out
}

// Subset returns the ToolDefs whose names appear in the allowed list.
// Unknown names are silently ignored.
func (r *Registry) Subset(names []string) []ToolDef {
	allow := make(map[string]bool, len(names))
	for _, n := range names {
		allow[n] = true
	}
	var out []ToolDef
	for _, d := range r.defs {
		if allow[d.Name] {
			out = append(out, d)
		}
	}
	return out
}
