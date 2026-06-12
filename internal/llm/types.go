// Package llm defines the LLM provider interface and message types.
package llm

import "encoding/json"

// Role identifies the author of a chat message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single turn in a conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

// ToolCall represents a model-requested invocation of a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// ToolDef describes a tool exposed to the model.
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonStop      StopReason = "stop"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonError     StopReason = "error"
)

// ChatRequest is sent to a Provider.
type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolDef
	Temperature float32
	MaxTokens   int
}

// ChatResponse is returned by a Provider.
type ChatResponse struct {
	Message    Message
	StopReason StopReason
	Usage      Usage
}

// Usage captures token and latency accounting.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalMs          int64
}
