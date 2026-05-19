package runtime

import (
	"context"
	"testing"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/tools"
)

// mockLLMClient implements client.LLMClient and returns a single "test response" delta.
type mockLLMClient struct{}

func (m *mockLLMClient) ChatCompletion(_ context.Context, _ client.ChatRequest) (<-chan client.StreamChunk, error) {
	ch := make(chan client.StreamChunk, 2)
	go func() {
		defer close(ch)
		ch <- client.StreamChunk{Delta: "test response"}
		ch <- client.StreamChunk{FinishReason: "stop"}
	}()
	return ch, nil
}

// makeTestConfig builds a minimal Config with a mock client and two providers.
func makeTestConfig() Config {
	appCfg := &config.Config{
		DefaultProvider: "mock",
		MaxIterations:   3,
		Providers: map[string]config.ProviderConfig{
			"mock": {
				APIKey: "test-key",
				Model:  "mock-model",
			},
			"other": {
				APIKey: "other-key",
				Model:  "other-model",
			},
		},
	}

	toolReg := tools.NewRegistry()

	createClient := func(_ string, _ config.ProviderConfig) client.LLMClient {
		return &mockLLMClient{}
	}

	return Config{
		AppConfig:    appCfg,
		ToolRegistry: toolReg,
		CreateClient: createClient,
	}
}

// TestRuntimeNew verifies that New() creates a Runtime with the expected provider.
func TestRuntimeNew(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() returned unexpected error: %v", err)
	}
	if rt == nil {
		t.Fatal("New() returned nil runtime")
	}
	if got := rt.CurrentProvider(); got != "mock" {
		t.Errorf("CurrentProvider() = %q; want %q", got, "mock")
	}
	if got := rt.CurrentModel(); got != "mock-model" {
		t.Errorf("CurrentModel() = %q; want %q", got, "mock-model")
	}
}

// TestRuntimeNew_MissingProvider verifies that New() errors on missing default provider.
func TestRuntimeNew_MissingProvider(t *testing.T) {
	cfg := makeTestConfig()
	cfg.AppConfig.DefaultProvider = "nonexistent"
	_, err := New(cfg)
	if err == nil {
		t.Fatal("New() expected error for missing provider, got nil")
	}
}

// TestRuntimeExecuteCommand verifies that /help returns a non-empty message.
func TestRuntimeExecuteCommand(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := rt.ExecuteCommand("/help")
	if err != nil {
		t.Fatalf("ExecuteCommand(/help) error: %v", err)
	}
	if result.Message == "" {
		t.Error("ExecuteCommand(/help) returned empty message")
	}
}

// TestRuntimeRunUserInput verifies that RunUserInput sends an EventPromptTrace event first.
func TestRuntimeRunUserInput(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx := context.Background()
	ch := rt.RunUserInput(ctx, "hello")

	var events []agent.AgentEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Fatal("RunUserInput returned no events")
	}

	// First event must be EventPromptTrace.
	if events[0].Type != agent.EventPromptTrace {
		t.Errorf("first event type = %v; want EventPromptTrace", events[0].Type)
	}

	// There should be additional events from the agent (delta + answer).
	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}

	// Verify the trace content is non-empty.
	if events[0].Content == "" {
		t.Error("EventPromptTrace content is empty")
	}
}

// TestRuntimeActivateSkill verifies error for a nonexistent skill.
func TestRuntimeActivateSkill(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	err = rt.ActivateSkill("nonexistent-skill")
	if err == nil {
		t.Error("ActivateSkill('nonexistent-skill') expected error, got nil")
	}
}

// TestRuntimeSwitchProvider verifies provider switching works correctly.
func TestRuntimeSwitchProvider(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := rt.SwitchProvider("other"); err != nil {
		t.Fatalf("SwitchProvider('other') error: %v", err)
	}
	if got := rt.CurrentProvider(); got != "other" {
		t.Errorf("CurrentProvider() after switch = %q; want %q", got, "other")
	}
	if got := rt.CurrentModel(); got != "other-model" {
		t.Errorf("CurrentModel() after switch = %q; want %q", got, "other-model")
	}
}

// TestRuntimeSwitchProvider_Missing verifies error for unknown provider.
func TestRuntimeSwitchProvider_Missing(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := rt.SwitchProvider("unknown"); err == nil {
		t.Error("SwitchProvider('unknown') expected error, got nil")
	}
}

// TestRuntimeListTools verifies ListTools returns ToolInfo slice from the registry.
func TestRuntimeListTools(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Empty registry should return empty slice (no panic).
	tools := rt.ListTools()
	if tools == nil {
		t.Error("ListTools() returned nil, want empty slice")
	}
}

// TestRuntimeAvailableProviders verifies AvailableProviders lists all configured providers.
func TestRuntimeAvailableProviders(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	providers := rt.AvailableProviders()
	if len(providers) != 2 {
		t.Errorf("AvailableProviders() len = %d; want 2", len(providers))
	}
}

// TestRuntimeLastPromptTrace verifies LastPromptTrace is empty before any run.
func TestRuntimeLastPromptTrace(t *testing.T) {
	cfg := makeTestConfig()
	rt, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	trace := rt.LastPromptTrace()
	if trace != "" {
		t.Errorf("LastPromptTrace() before any run = %q; want empty", trace)
	}

	// After running, trace should be populated.
	ctx := context.Background()
	ch := rt.RunUserInput(ctx, "hello")
	// Drain channel.
	for range ch {
	}

	trace = rt.LastPromptTrace()
	if trace == "" {
		t.Error("LastPromptTrace() after run is empty, want non-empty")
	}
}
