package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"
)

// sseLines builds an SSE data payload from a list of JSON-serializable objects.
func writeSSE(w http.ResponseWriter, obj any) {
	b, _ := json.Marshal(obj)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeDone(w http.ResponseWriter) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// openAIChunk is a minimal representation of an OpenAI streaming chunk.
type openAIChunk struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Delta        openAIDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type openAIDelta struct {
	Content   string           `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	Index    int                  `json:"index"`
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function openAIToolCallFunc   `json:"function"`
}

type openAIToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func stopStr(s string) *string { return &s }

// TestOpenAIStreamingTextResponse verifies basic SSE text streaming.
func TestOpenAIStreamingTextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer sk-test")
		}
		// Verify Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{Content: "Hello"}}}})
		writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{Content: " world"}}}})
		writeSSE(w, openAIChunk{Choices: []openAIChoice{{FinishReason: stopStr("stop")}}})
		writeDone(w)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "sk-test", "gpt-4o")
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

// TestOpenAIStreamingToolCall verifies tool call accumulation across chunks.
func TestOpenAIStreamingToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// First chunk: tool call header (name, id)
		writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{ToolCalls: []openAIToolCall{
			{Index: 0, ID: "call_abc123", Type: "function", Function: openAIToolCallFunc{Name: "tavily_search", Arguments: ""}},
		}}}}})

		// Second chunk: argument fragment 1
		writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{ToolCalls: []openAIToolCall{
			{Index: 0, Function: openAIToolCallFunc{Arguments: `{"query"`}},
		}}}}})

		// Third chunk: argument fragment 2
		writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{ToolCalls: []openAIToolCall{
			{Index: 0, Function: openAIToolCallFunc{Arguments: `:"golang"}`}},
		}}}}})

		writeDone(w)
	}))
	defer srv.Close()

	client := NewOpenAIClient(srv.URL, "sk-test", "gpt-4o")
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
	if tc.Arguments != `{"query":"golang"}` {
		t.Errorf("tool call arguments = %q, want %q", tc.Arguments, `{"query":"golang"}`)
	}
}

// TestOpenAIContextCancel verifies context cancellation stops streaming quickly.
func TestOpenAIContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		// Send many chunks - client should stop reading after context cancel
		for i := 0; i < 100; i++ {
			writeSSE(w, openAIChunk{Choices: []openAIChoice{{Delta: openAIDelta{Content: fmt.Sprintf("chunk%d ", i)}}}})
			time.Sleep(10 * time.Millisecond) // slow enough so cancel fires mid-stream
		}
		writeDone(w)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewOpenAIClient(srv.URL, "sk-test", "gpt-4o")
	ch, err := client.ChatCompletion(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}

	count := 0
	cancel() // cancel immediately after starting
	for range ch {
		count++
	}

	// After cancel, channel should close quickly (well under all 100 chunks)
	if count >= 10 {
		t.Errorf("expected < 10 chunks after cancel, got %d", count)
	}
}
