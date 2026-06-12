package llm

import "context"

// Provider is the interface every LLM backend must satisfy.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Name() string
}
