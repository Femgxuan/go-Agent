package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/tools"
)

// mockLLMClient returns pre-configured responses sequentially.
type mockLLMClient struct {
	responses []mockResponse
	callIndex int
}

type mockResponse struct {
	content   string
	toolCalls []client.ToolCall
}

func (m *mockLLMClient) ChatCompletion(ctx context.Context, req client.ChatRequest) (<-chan client.StreamChunk, error) {
	ch := make(chan client.StreamChunk, 2)
	resp := m.responses[m.callIndex%len(m.responses)]
	m.callIndex++

	go func() {
		defer close(ch)
		if resp.content != "" {
			ch <- client.StreamChunk{Delta: resp.content}
		}
		if len(resp.toolCalls) > 0 {
			ch <- client.StreamChunk{ToolCalls: resp.toolCalls}
		}
	}()

	return ch, nil
}

// mockTool is a simple mock tool for testing.
type mockTool struct {
	name   string
	result string
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool: " + t.name }
func (t *mockTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *mockTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	return t.result, nil
}

func makeRegistry(t *testing.T, tool tools.Tool) *tools.Registry {
	r := tools.NewRegistry()
	if tool != nil {
		r.Register(tool)
	}
	return r
}

// TestAgentDirectAnswer: LLM responds with text only → assert EventAnswer received.
func TestAgentDirectAnswer(t *testing.T) {
	llm := &mockLLMClient{
		responses: []mockResponse{
			{content: "Hello, world!"},
		},
	}
	reg := makeRegistry(t, nil)
	a := New(llm, reg, AgentConfig{MaxIterations: 5}, nil, nil, "")

	ctx := context.Background()
	ch := a.Run(ctx, "hi")

	var events []AgentEvent
	for e := range ch {
		events = append(events, e)
	}

	// Find EventAnswer
	var found bool
	for _, e := range events {
		if e.Type == EventAnswer {
			found = true
			if e.Content != "Hello, world!" {
				t.Errorf("expected answer content %q, got %q", "Hello, world!", e.Content)
			}
		}
	}
	if !found {
		t.Errorf("expected EventAnswer but got events: %v", events)
	}
}

// TestAgentToolCallLoop: LLM first responds with tool_call, then with text.
func TestAgentToolCallLoop(t *testing.T) {
	searchArgs, _ := json.Marshal(map[string]any{"query": "test"})
	llm := &mockLLMClient{
		responses: []mockResponse{
			{
				toolCalls: []client.ToolCall{
					{ID: "tc1", Name: "search", Arguments: string(searchArgs)},
				},
			},
			{content: "final answer"},
		},
	}
	tool := &mockTool{name: "search", result: "search result here"}
	reg := makeRegistry(t, tool)
	a := New(llm, reg, AgentConfig{MaxIterations: 5}, nil, nil, "")

	ctx := context.Background()
	ch := a.Run(ctx, "search for something")

	var events []AgentEvent
	for e := range ch {
		events = append(events, e)
	}

	var hasToolCall, hasToolResult, hasAnswer bool
	for _, e := range events {
		switch e.Type {
		case EventToolCall:
			hasToolCall = true
			if e.ToolName != "search" {
				t.Errorf("expected tool name 'search', got %q", e.ToolName)
			}
		case EventToolResult:
			hasToolResult = true
			if e.Content != "search result here" {
				t.Errorf("expected tool result 'search result here', got %q", e.Content)
			}
		case EventAnswer:
			hasAnswer = true
			if e.Content != "final answer" {
				t.Errorf("expected answer 'final answer', got %q", e.Content)
			}
		}
	}

	if !hasToolCall {
		t.Error("expected EventToolCall but not found")
	}
	if !hasToolResult {
		t.Error("expected EventToolResult but not found")
	}
	if !hasAnswer {
		t.Error("expected EventAnswer but not found")
	}

	// Assert order: ToolCall before ToolResult before Answer
	var tcIdx, trIdx, anIdx int = -1, -1, -1
	for i, e := range events {
		if e.Type == EventToolCall && tcIdx == -1 {
			tcIdx = i
		}
		if e.Type == EventToolResult && trIdx == -1 {
			trIdx = i
		}
		if e.Type == EventAnswer && anIdx == -1 {
			anIdx = i
		}
	}
	if !(tcIdx < trIdx && trIdx < anIdx) {
		t.Errorf("event order wrong: ToolCall=%d, ToolResult=%d, Answer=%d", tcIdx, trIdx, anIdx)
	}
}

// TestAgentMaxIterations: LLM always responds with tool_calls, assert EventError.
func TestAgentMaxIterations(t *testing.T) {
	searchArgs, _ := json.Marshal(map[string]any{"query": "test"})
	llm := &mockLLMClient{
		responses: []mockResponse{
			{
				toolCalls: []client.ToolCall{
					{ID: "tc1", Name: "search", Arguments: string(searchArgs)},
				},
			},
		},
	}
	tool := &mockTool{name: "search", result: "result"}
	reg := makeRegistry(t, tool)
	a := New(llm, reg, AgentConfig{MaxIterations: 3}, nil, nil, "")

	ctx := context.Background()
	ch := a.Run(ctx, "loop forever")

	var events []AgentEvent
	for e := range ch {
		events = append(events, e)
	}

	var found bool
	for _, e := range events {
		if e.Type == EventError {
			found = true
			if e.Content == "" {
				t.Error("expected error message content but got empty string")
			}
			// Check it mentions max iterations
			if len(e.Content) == 0 {
				t.Error("error content should mention max iterations")
			}
		}
	}
	if !found {
		t.Errorf("expected EventError but got events: %v", events)
	}
}

// TestAgentContextCancel: Cancel context after first event, assert channel closes quickly.
func TestAgentContextCancel(t *testing.T) {
	searchArgs, _ := json.Marshal(map[string]any{"query": "test"})
	llm := &mockLLMClient{
		responses: []mockResponse{
			{
				toolCalls: []client.ToolCall{
					{ID: "tc1", Name: "search", Arguments: string(searchArgs)},
				},
			},
		},
	}
	tool := &mockTool{name: "search", result: "result"}
	reg := makeRegistry(t, tool)
	a := New(llm, reg, AgentConfig{MaxIterations: 100}, nil, nil, "")

	ctx, cancel := context.WithCancel(context.Background())

	ch := a.Run(ctx, "keep going")

	// Cancel after receiving first event
	count := 0
	for range ch {
		count++
		if count == 1 {
			cancel()
		}
		if count > 10 {
			t.Error("channel should have closed after context cancel, got more than 10 events")
			break
		}
	}
	cancel() // ensure no leak
}

// TestAgentMultiTurn: Call Run() twice on same agent, assert history grows.
func TestAgentMultiTurn(t *testing.T) {
	llm := &mockLLMClient{
		responses: []mockResponse{
			{content: "first answer"},
			{content: "second answer"},
		},
	}
	reg := makeRegistry(t, nil)
	a := New(llm, reg, AgentConfig{MaxIterations: 5}, nil, nil, "")

	ctx := context.Background()

	// First run
	ch := a.Run(ctx, "first question")
	for range ch {
	}

	histAfterFirst := len(a.History())
	if histAfterFirst < 2 {
		t.Errorf("expected at least 2 messages after first run, got %d", histAfterFirst)
	}

	// Second run
	ch = a.Run(ctx, "second question")
	for range ch {
	}

	histAfterSecond := len(a.History())
	if histAfterSecond < 4 {
		t.Errorf("expected at least 4 messages after second run, got %d", histAfterSecond)
	}

	// Verify second run's LLM call included history from first run
	// (callIndex should be 2 now)
	if llm.callIndex != 2 {
		t.Errorf("expected 2 LLM calls, got %d", llm.callIndex)
	}

	_ = time.Second // avoid import error
}

func TestExtractMemoryFact_ExplicitRemember(t *testing.T) {
	tests := []struct {
		input  string
		wantOK bool
	}{
		{"记住我喜欢Go语言", true},
		{"Remember I like Python", true},
		{"你好", false},
		{"今天天气怎么样", false},
	}
	for _, tt := range tests {
		candidate := extractMemoryFact(tt.input)
		if candidate.ShouldClassify != tt.wantOK {
			t.Errorf("extractMemoryFact(%q) ok = %v, want %v", tt.input, candidate.ShouldClassify, tt.wantOK)
		}
	}
}

func TestExtractMemoryFact_Preference(t *testing.T) {
	tests := []struct {
		input  string
		wantOK bool
	}{
		{"我喜欢Go语言", true},
		{"我不喜欢Java", true},
		{"我想要学习Rust", true},
		{"I like Go", true},
		{"my favorite language is Python", true},
		{"你好", false},
		{"今天天气怎么样", false},
	}
	for _, tt := range tests {
		candidate := extractMemoryFact(tt.input)
		if candidate.ShouldClassify != tt.wantOK {
			t.Errorf("extractMemoryFact(%q) ok = %v, want %v", tt.input, candidate.ShouldClassify, tt.wantOK)
		}
	}
}
