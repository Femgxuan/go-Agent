package builtin

import (
	"strings"
	"testing"

	"github.com/fengxuan/go-agent/commands"
)

// mockRuntime implements commands.RuntimeAccessor for testing.
type mockRuntime struct {
	skills           []commands.SkillInfo
	tools            []commands.ToolInfo
	currentProvider  string
	currentModel     string
	providers        []string
	promptTrace      string
	activatedSkill   string
	reloadCalled     bool
	clearCalled      bool
	switchedProvider string
	switchedModel    string
}

func (m *mockRuntime) ListSkills() []commands.SkillInfo              { return m.skills }
func (m *mockRuntime) ActivateSkill(name string) error               { m.activatedSkill = name; return nil }
func (m *mockRuntime) ReloadSkillsAndRules() error                   { m.reloadCalled = true; return nil }
func (m *mockRuntime) LastPromptTrace() string                       { return m.promptTrace }
func (m *mockRuntime) ListTools() []commands.ToolInfo                { return m.tools }
func (m *mockRuntime) SwitchProvider(name string) error              { m.switchedProvider = name; return nil }
func (m *mockRuntime) SwitchModel(name string) error                 { m.switchedModel = name; return nil }
func (m *mockRuntime) CurrentProvider() string                       { return m.currentProvider }
func (m *mockRuntime) CurrentModel() string                          { return m.currentModel }
func (m *mockRuntime) AvailableProviders() []string                  { return m.providers }
func (m *mockRuntime) ClearHistory()                                 { m.clearCalled = true }
func (m *mockRuntime) CreateSkill(req commands.CreateSkillRequest) (commands.CreateSkillResult, error) {
	return commands.CreateSkillResult{Name: req.Name, Message: "created"}, nil
}
func (m *mockRuntime) DeleteSkill(name string) error { return nil }
func (m *mockRuntime) UpdateSkill(name string, req commands.CreateSkillRequest) (commands.CreateSkillResult, error) {
	return commands.CreateSkillResult{Name: req.Name, Message: "updated"}, nil
}

func newCtx(rt *mockRuntime) commands.CommandContext {
	return commands.CommandContext{
		Args:    []string{},
		Output:  func(s string) {},
		Runtime: rt,
	}
}

func TestHelpCommand(t *testing.T) {
	reg := commands.NewRegistry()
	reg.Register(NewQuit())
	reg.Register(NewClear())

	help := NewHelp(reg)
	rt := &mockRuntime{}
	ctx := newCtx(rt)

	result, err := help.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty help output")
	}
	if !strings.Contains(result.Message, "quit") {
		t.Error("expected help to contain 'quit'")
	}
	if !strings.Contains(result.Message, "clear") {
		t.Error("expected help to contain 'clear'")
	}
}

func TestClearCommand(t *testing.T) {
	cmd := NewClear()
	rt := &mockRuntime{}
	ctx := newCtx(rt)

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != commands.ActionClearScreen {
		t.Errorf("expected ActionClearScreen, got %v", result.Action)
	}
	if !rt.clearCalled {
		t.Error("expected ClearHistory to be called")
	}
}

func TestQuitCommand(t *testing.T) {
	cmd := NewQuit()
	rt := &mockRuntime{}
	ctx := newCtx(rt)

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != commands.ActionQuit {
		t.Errorf("expected ActionQuit, got %v", result.Action)
	}
}

func TestSkillActivation(t *testing.T) {
	cmd := NewSkill()
	rt := &mockRuntime{}
	ctx := newCtx(rt)
	ctx.Args = []string{"my-skill"}

	_, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.activatedSkill != "my-skill" {
		t.Errorf("expected ActivateSkill called with 'my-skill', got '%s'", rt.activatedSkill)
	}
}

func TestReloadCommand(t *testing.T) {
	cmd := NewReload()
	rt := &mockRuntime{}
	ctx := newCtx(rt)

	_, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rt.reloadCalled {
		t.Error("expected ReloadSkillsAndRules to be called")
	}
}

func TestPromptTraceCommand(t *testing.T) {
	cmd := NewPrompt()
	rt := &mockRuntime{promptTrace: "system: you are an agent\nuser: hello"}
	ctx := newCtx(rt)

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Message, "system: you are an agent") {
		t.Errorf("expected trace in message, got: %s", result.Message)
	}
}

func TestPromptTraceCommandEmpty(t *testing.T) {
	cmd := NewPrompt()
	rt := &mockRuntime{promptTrace: ""}
	ctx := newCtx(rt)

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "No prompt trace available yet." {
		t.Errorf("expected fallback message, got: %s", result.Message)
	}
}

func TestToolsCommand(t *testing.T) {
	cmd := NewTools()
	rt := &mockRuntime{
		tools: []commands.ToolInfo{
			{Name: "search", Description: "Search the web"},
			{Name: "calc", Description: "Perform calculations"},
		},
	}
	ctx := newCtx(rt)

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty tools output")
	}
	if !strings.Contains(result.Message, "search") {
		t.Error("expected tools output to contain 'search'")
	}
	if !strings.Contains(result.Message, "calc") {
		t.Error("expected tools output to contain 'calc'")
	}
}

func TestProviderSwitchCommand(t *testing.T) {
	cmd := NewProvider()
	rt := &mockRuntime{
		currentProvider: "openai",
		providers:       []string{"openai", "anthropic"},
	}
	ctx := newCtx(rt)
	ctx.Args = []string{"anthropic"}

	result, err := cmd.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.switchedProvider != "anthropic" {
		t.Errorf("expected SwitchProvider called with 'anthropic', got '%s'", rt.switchedProvider)
	}
	if !strings.Contains(result.Message, "anthropic") {
		t.Errorf("expected message to mention 'anthropic', got: %s", result.Message)
	}
}
