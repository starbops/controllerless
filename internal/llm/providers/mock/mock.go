// Package mock provides a deterministic, scripted LLM provider for testing.
package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/starbops/controllerless/internal/llm"
)

// Provider replays a fixed script of ChatResponses in order.
type Provider struct {
	mu     sync.Mutex
	script []llm.ChatResponse
	pos    int
}

// New returns a mock Provider that will return the given responses in order.
func New(script []llm.ChatResponse) *Provider {
	return &Provider{script: script}
}

// Name implements llm.Provider.
func (p *Provider) Name() string { return "mock" }

// Chat implements llm.Provider. It returns the next scripted response, or an
// error once the script is exhausted.
func (p *Provider) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pos >= len(p.script) {
		return nil, errors.New("mock: script exhausted")
	}
	resp := p.script[p.pos]
	p.pos++
	return &resp, nil
}
