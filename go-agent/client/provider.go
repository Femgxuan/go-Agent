package client

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolSchema struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSchema
}

type StreamChunk struct {
	Delta        string
	ToolCalls    []ToolCall
	FinishReason string
	Err          error
}

type LLMClient interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}
