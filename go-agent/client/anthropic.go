package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AnthropicClient is an LLMClient implementation for the Anthropic Messages API.
type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewAnthropicClient creates a new AnthropicClient.
func NewAnthropicClient(baseURL, apiKey, model string) *AnthropicClient {
	return &AnthropicClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// --- JSON structures for the Anthropic Messages API ---

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string              `json:"role"`
	Content anthropicMsgContent `json:"content"`
}

// anthropicMsgContent can be either a plain string or a slice of content blocks.
// We use a custom marshal type to handle both.
type anthropicMsgContent interface{}

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicToolUseBlock struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

type anthropicToolResultBlock struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// --- SSE response structures ---

type antStreamEvent struct {
	Type string `json:"type"`

	// message_start
	Message *antMessage `json:"message,omitempty"`

	// content_block_start
	Index        int              `json:"index"`
	ContentBlock *antContentBlock `json:"content_block,omitempty"`

	// content_block_delta
	Delta *antDelta `json:"delta,omitempty"`

	// usage (message_delta)
	Usage *antUsage `json:"usage,omitempty"`
}

type antMessage struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
}

type antContentBlock struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Text string `json:"text,omitempty"`
}

type antDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type antUsage struct {
	OutputTokens int `json:"output_tokens"`
}

// antAccumulatedToolCall holds state for a tool_use block being built across deltas.
type antAccumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

// ChatCompletion sends a streaming chat request to the Anthropic Messages API.
func (c *AnthropicClient) ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	// Separate system messages and build Anthropic messages
	var systemText string
	var msgs []anthropicMessage

	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			systemText = m.Content

		case RoleTool:
			// Tool result → user message with tool_result content block
			msgs = append(msgs, anthropicMessage{
				Role: "user",
				Content: []anthropicToolResultBlock{
					{
						Type:      "tool_result",
						ToolUseID: m.ToolCallID,
						Content:   m.Content,
					},
				},
			})

		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				// Assistant with tool calls → content blocks with tool_use type
				var blocks []any
				if m.Content != "" {
					blocks = append(blocks, anthropicTextBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					var input any
					if tc.Arguments != "" {
						var parsed any
						if err := json.Unmarshal([]byte(tc.Arguments), &parsed); err == nil {
							input = parsed
						} else {
							input = map[string]any{}
						}
					} else {
						input = map[string]any{}
					}
					blocks = append(blocks, anthropicToolUseBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					})
				}
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
			} else {
				msgs = append(msgs, anthropicMessage{Role: "assistant", Content: m.Content})
			}

		default:
			// user role
			msgs = append(msgs, anthropicMessage{Role: "user", Content: m.Content})
		}
	}

	// Convert tools
	var antTools []anthropicTool
	for _, t := range req.Tools {
		params := t.Function.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		antTools = append(antTools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: params,
		})
	}

	antReq := anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    systemText,
		Messages:  msgs,
		Tools:     antTools,
		Stream:    true,
	}

	body, err := json.Marshal(antReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	ch := make(chan StreamChunk, 16)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// accumulated tool calls keyed by content block index
		accumulated := make(map[int]*antAccumulatedToolCall)

		send := func(sc StreamChunk) bool {
			select {
			case <-ctx.Done():
				return false
			case ch <- sc:
				return true
			}
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}

			line := scanner.Text()

			// Anthropic SSE: lines starting with "data: " carry event data
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var event antStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				send(StreamChunk{Err: fmt.Errorf("parse event: %w", err)})
				return
			}

			switch event.Type {
			case "message_start":
				// no action needed

			case "content_block_start":
				if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
					accumulated[event.Index] = &antAccumulatedToolCall{
						id:   event.ContentBlock.ID,
						name: event.ContentBlock.Name,
					}
				}

			case "content_block_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.Type {
				case "text_delta":
					if event.Delta.Text != "" {
						if !send(StreamChunk{Delta: event.Delta.Text}) {
							return
						}
					}
				case "input_json_delta":
					if acc, ok := accumulated[event.Index]; ok {
						acc.arguments.WriteString(event.Delta.PartialJSON)
					}
				}

			case "content_block_stop":
				// no action here; flush happens on message_delta with tool_use stop_reason

			case "message_delta":
				if event.Delta == nil {
					continue
				}
				switch event.Delta.StopReason {
				case "tool_use":
					// Flush all accumulated tool calls
					if len(accumulated) > 0 {
						calls := flushAnthropicToolCalls(accumulated)
						if !send(StreamChunk{ToolCalls: calls, FinishReason: "tool_use"}) {
							return
						}
					}
				case "end_turn":
					if !send(StreamChunk{FinishReason: "stop"}) {
						return
					}
				}

			case "message_stop":
				return
			}
		}

		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			send(StreamChunk{Err: fmt.Errorf("scanner: %w", err)})
		}
	}()

	return ch, nil
}

// flushAnthropicToolCalls converts the accumulated map to a slice of ToolCall.
func flushAnthropicToolCalls(accumulated map[int]*antAccumulatedToolCall) []ToolCall {
	calls := make([]ToolCall, 0, len(accumulated))
	// Build ordered slice by index
	maxIdx := -1
	for idx := range accumulated {
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	result := make([]ToolCall, maxIdx+1)
	for idx, acc := range accumulated {
		result[idx] = ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: acc.arguments.String(),
		}
	}
	for _, tc := range result {
		if tc.ID != "" || tc.Name != "" {
			calls = append(calls, tc)
		}
	}
	return calls
}
