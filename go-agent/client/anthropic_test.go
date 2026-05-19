package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAnthropicStreamingTextResponse verifies basic SSE text streaming from Anthropic API.
func TestAnthropicStreamingTextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate required headers
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key header = %q, want %q", got, "sk-ant-test")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version header = %q, want %q", got, "2023-06-01")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-3-5-sonnet-20241022\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":5}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, ev := range events {
			fmt.Fprint(w, ev)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	client := NewAnthropicClient(srv.URL, "sk-ant-test", "claude-3-5-sonnet-20241022")
	ch, err := client.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}

	var sb strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		sb.WriteString(chunk.Delta)
	}

	if got := sb.String(); got != "Hello world" {
		t.Errorf("concatenated output = %q, want %q", got, "Hello world")
	}
}

// TestAnthropicStreamingToolUse verifies tool use streaming from Anthropic API.
func TestAnthropicStreamingToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_2\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-3-5-sonnet-20241022\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"tavily_search\",\"input\":{}}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\": \"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"golang\\\"\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":10}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, ev := range events {
			fmt.Fprint(w, ev)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer srv.Close()

	client := NewAnthropicClient(srv.URL, "sk-ant-test", "claude-3-5-sonnet-20241022")
	ch, err := client.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "search golang"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}

	var finalCalls []ToolCall
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		if len(chunk.ToolCalls) > 0 {
			finalCalls = chunk.ToolCalls
		}
	}

	if len(finalCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(finalCalls))
	}
	tc := finalCalls[0]
	if tc.Name != "tavily_search" {
		t.Errorf("tool call name = %q, want %q", tc.Name, "tavily_search")
	}
	if tc.ID != "toolu_1" {
		t.Errorf("tool call id = %q, want %q", tc.ID, "toolu_1")
	}
}
