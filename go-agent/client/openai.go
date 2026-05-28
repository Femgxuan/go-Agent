package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAIClient is an LLMClient implementation for OpenAI-compatible APIs.
type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAIClient.
func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// --- JSON structures for the OpenAI API ---

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []ToolSchema    `json:"tools,omitempty"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role       string              `json:"role"`
	Content    *string             `json:"content"`
	ToolCalls  []openAIToolCallOut `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

type openAIToolCallOut struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function openAIFunctionOut `json:"function"`
}

type openAIFunctionOut struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// --- SSE response structures ---

type oaiStreamChunk struct {
	Choices []oaiChoice `json:"choices"`
}

type oaiChoice struct {
	Delta        oaiDelta `json:"delta"`
	FinishReason *string  `json:"finish_reason"`
}

type oaiDelta struct {
	Content   string        `json:"content"`
	ToolCalls []oaiToolCall `json:"tool_calls"`
}

type oaiToolCall struct {
	Index    int         `json:"index"`
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFunction `json:"function"`
}

type oaiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// accumulatedToolCall holds the state of a tool call built across SSE chunks.
type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

// ChatCompletion sends a streaming chat request and returns a channel of StreamChunks.
func (c *OpenAIClient) ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	// Convert messages
	msgs := make([]openAIMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		// Always include content field (required by some APIs when tool_calls present).
		// Use pointer so empty string serializes as "content": "" not omitted.
		content := m.Content
		om := openAIMessage{
			Role:       string(m.Role),
			Content:    &content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				om.ToolCalls = append(om.ToolCalls, openAIToolCallOut{
					ID:   tc.ID,
					Type: "function",
					Function: openAIFunctionOut{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}
		msgs = append(msgs, om)
	}

	oaiReq := openAIRequest{
		Model:    model,
		Messages: msgs,
		Tools:    req.Tools,
		Stream:   true,
	}

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	ch := make(chan StreamChunk, 16)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// accumulated tool calls keyed by index
		accumulated := make(map[int]*accumulatedToolCall)

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
			// Check context cancellation between chunks
			if ctx.Err() != nil {
				return
			}

			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// Flush accumulated tool calls
				if len(accumulated) > 0 {
					calls := flushToolCalls(accumulated)
					send(StreamChunk{ToolCalls: calls})
				}
				return
			}

			var chunk oaiStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				send(StreamChunk{Err: fmt.Errorf("parse chunk: %w", err)})
				return
			}

			for _, choice := range chunk.Choices {
				// Handle text delta
				if choice.Delta.Content != "" {
					if !send(StreamChunk{Delta: choice.Delta.Content}) {
						return
					}
				}

				// Handle tool call deltas
				for _, tc := range choice.Delta.ToolCalls {
					acc, ok := accumulated[tc.Index]
					if !ok {
						acc = &accumulatedToolCall{}
						accumulated[tc.Index] = acc
					}
					if tc.ID != "" {
						acc.id = tc.ID
					}
					if tc.Function.Name != "" {
						acc.name = tc.Function.Name
					}
					acc.arguments.WriteString(tc.Function.Arguments)
				}

				// Handle finish reason
				if choice.FinishReason != nil {
					if *choice.FinishReason == "stop" {
						if !send(StreamChunk{FinishReason: "stop"}) {
							return
						}
						return
					}
					if *choice.FinishReason == "tool_calls" {
						// Will be flushed on [DONE]
					}
				}
			}
		}

		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			send(StreamChunk{Err: fmt.Errorf("scanner: %w", err)})
		}
	}()

	return ch, nil
}

// flushToolCalls converts the accumulated map to a slice of ToolCall.
func flushToolCalls(accumulated map[int]*accumulatedToolCall) []ToolCall {
	calls := make([]ToolCall, len(accumulated))
	for idx, acc := range accumulated {
		if idx < len(calls) {
			calls[idx] = ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: acc.arguments.String(),
			}
		}
	}
	return calls
}
