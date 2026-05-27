package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/commands"
	"github.com/fengxuan/go-agent/commands/builtin"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/memory"
	"github.com/fengxuan/go-agent/prompt"
	"github.com/fengxuan/go-agent/rules"
	"github.com/fengxuan/go-agent/skills"
	"github.com/fengxuan/go-agent/tools"
)

const baseSystemPrompt = `You are a helpful AI assistant with access to tools.

IMPORTANT: Only use tools when the user's request explicitly requires them. For simple greetings, questions, or conversations, respond directly WITHOUT using any tools.

Available tools (use ONLY when necessary):
- tavily_search: Use ONLY when you need to search the internet for current information
- shell_exec: Use ONLY when you need to run shell commands
- read_file: Use ONLY when you need to read file contents
- write_file: Use ONLY when you need to write or create files

When you do use tools, explain your reasoning first. For simple conversations like greetings, just respond naturally.`

// Config holds the dependencies needed to build a Runtime.
type Config struct {
	AppConfig    *config.Config
	ToolRegistry *tools.Registry
	CreateClient func(provider string, cfg config.ProviderConfig) client.LLMClient
}

// Runtime coordinates all subsystems: agent, skills, rules, commands, and prompt building.
type Runtime struct {
	cfg          *config.Config
	agent        *agent.Agent
	toolRegistry *tools.Registry
	skillIndex   *skills.Index
	skillManager *skills.Manager
	rulesLoader  *rules.Loader
	cmdRegistry  *commands.Registry
	memory       memory.Manager

	mu           sync.Mutex
	activeSkills []string
	lastTrace    *prompt.Trace
	provider     string
	model        string
	userDir      string
	projectDir   string
	createClient func(provider string, cfg config.ProviderConfig) client.LLMClient
}

// New creates a Runtime with all subsystems wired together.
// It returns an error if the default provider is not found in cfg.AppConfig.
func New(cfg Config) (*Runtime, error) {
	appCfg := cfg.AppConfig
	if appCfg == nil {
		return nil, fmt.Errorf("AppConfig is required")
	}

	providerName := appCfg.DefaultProvider
	providerCfg, ok := appCfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("default provider %q not found in config", providerName)
	}

	llm := cfg.CreateClient(providerName, providerCfg)

	// Initialize memory system.
	memCfg := memory.DefaultConfig()
	memory.ApplyEnvOverrides(memCfg)
	slog.Info("[runtime] memory config", "postgres_url", memCfg.LongTerm.PostgresURL, "working_max_tokens", memCfg.Working.MaxTokens)

	memManager, err := memory.NewManager(memCfg)
	if err != nil {
		slog.Warn("[runtime] memory system unavailable, falling back", "error", err)
	} else {
		ctx := context.Background()
		memManager.StartSession(ctx, "default")
		slog.Info("[runtime] memory system initialized successfully")
	}

	// Create memory classifier.
	var classifier *memory.Classifier
	if memManager != nil {
		classifier = memory.NewClassifier(llm)
		slog.Info("[runtime] memory classifier created")
	} else {
		slog.Warn("[runtime] memory classifier NOT created (memManager is nil)")
	}

	agentCfg := agent.AgentConfig{
		MaxIterations: appCfg.MaxIterations,
		Model:         providerCfg.Model,
		MaxTokens:     memCfg.Working.MaxTokens,
		MemoryEnabled: memManager != nil,
	}
	slog.Info("[runtime] agent config", "memory_enabled", agentCfg.MemoryEnabled)
	ag := agent.New(llm, cfg.ToolRegistry, agentCfg, memManager, classifier)

	// Determine user and project directories.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	userDir := filepath.Join(homeDir, ".go-agent")
	projectDir := ".go-agent"

	// Load skills from both layers.
	userSkillsDir := filepath.Join(userDir, "skills")
	projectSkillsDir := "skills"
	loadedSkills, err := skills.LoadMultiLayer(userSkillsDir, projectSkillsDir)
	if err != nil {
		return nil, fmt.Errorf("loading skills: %w", err)
	}
	skillIndex := skills.NewIndex(loadedSkills)

	// Create skill manager for CRUD operations.
	skillManager := skills.NewManager(userSkillsDir, projectSkillsDir, skillIndex)

	// Create rules loader.
	rulesLoader := rules.NewLoader(userDir, projectDir)

	// Build command registry with all built-in commands.
	cmdRegistry := commands.NewRegistry()

	rt := &Runtime{
		cfg:          appCfg,
		agent:        ag,
		toolRegistry: cfg.ToolRegistry,
		skillIndex:   skillIndex,
		skillManager: skillManager,
		rulesLoader:  rulesLoader,
		cmdRegistry:  cmdRegistry,
		memory:       memManager,
		provider:     providerName,
		model:        providerCfg.Model,
		userDir:      userDir,
		projectDir:   projectDir,
		createClient: cfg.CreateClient,
	}

	// Register all 10 built-in commands.
	cmdRegistry.Register(builtin.NewHelp(cmdRegistry))
	cmdRegistry.Register(builtin.NewClear())
	cmdRegistry.Register(builtin.NewQuit())
	cmdRegistry.Register(builtin.NewProvider())
	cmdRegistry.Register(builtin.NewModel())
	cmdRegistry.Register(builtin.NewSkills())
	cmdRegistry.Register(builtin.NewSkill())
	cmdRegistry.Register(builtin.NewReload())
	cmdRegistry.Register(builtin.NewPrompt())
	cmdRegistry.Register(builtin.NewTools())
	cmdRegistry.Register(builtin.NewCreateSkill())
	cmdRegistry.Register(builtin.NewDeleteSkill())
	cmdRegistry.Register(builtin.NewUpdateSkill())

	// Register each loaded skill as a slash command.
	for _, s := range loadedSkills {
		cmdRegistry.Register(builtin.NewSkillRun(s.Name, s.Description))
	}

	return rt, nil
}

// RunUserInput is the main request flow. It builds the system prompt, sets it on the
// agent, calls agent.Run, and returns a wrapped channel that prepends an EventPromptTrace event.
func (rt *Runtime) RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent {
	// Build the prompt.
	pb := prompt.NewBuilder()

	// 1. Base system prompt.
	pb.AddSource(prompt.Source{
		Name:    "base",
		Type:    prompt.TypeSystem,
		Content: baseSystemPrompt,
	})

	// 2. Load and add rules.
	ruleSets := rt.rulesLoader.Load()
	for _, rs := range ruleSets {
		pb.AddSource(prompt.Source{
			Name:    rs.Path,
			Type:    prompt.TypeRules,
			Content: rs.Content,
		})
	}

	// 3. Match skills: auto-match on input + any explicitly activated skills.
	rt.mu.Lock()
	activeSkillNames := make([]string, len(rt.activeSkills))
	copy(activeSkillNames, rt.activeSkills)
	rt.activeSkills = nil // clear after capture
	rt.mu.Unlock()

	// Auto-match skills based on input.
	matchedSkills := rt.skillIndex.Match(input)
	skillsSeen := make(map[string]bool)
	for _, s := range matchedSkills {
		skillsSeen[s.Name] = true
		pb.AddSource(prompt.Source{
			Name:    s.Name,
			Type:    prompt.TypeSkill,
			Content: s.Body,
		})
	}

	// Add explicitly activated skills (if not already included).
	for _, name := range activeSkillNames {
		if skillsSeen[name] {
			continue
		}
		results := rt.skillIndex.MatchByName(name)
		for _, s := range results {
			pb.AddSource(prompt.Source{
				Name:    s.Name,
				Type:    prompt.TypeSkill,
				Content: s.Body,
			})
		}
	}

	builtPrompt, trace := pb.Build()

	// Store the trace.
	rt.mu.Lock()
	rt.lastTrace = trace
	rt.mu.Unlock()

	// Set the built prompt on the agent.
	rt.agent.SetSystemPrompt(builtPrompt)

	// Start the agent run.
	agentCh := rt.agent.Run(ctx, input)

	// Wrap the channel to prepend the EventPromptTrace event.
	outCh := make(chan agent.AgentEvent, 64)
	go func() {
		defer close(outCh)
		// Prepend trace event.
		outCh <- agent.AgentEvent{
			Type:    agent.EventPromptTrace,
			Content: trace.String(),
		}
		// Forward all agent events.
		for ev := range agentCh {
			outCh <- ev
		}
	}()

	return outCh
}

// ExecuteCommand delegates to the command registry.
func (rt *Runtime) ExecuteCommand(input string) (commands.CommandResult, error) {
	cmdCtx := commands.CommandContext{
		Runtime: rt,
	}
	return rt.cmdRegistry.Execute(input, cmdCtx)
}

// CmdRegistry exposes the command registry (e.g., for TUI tab completion).
func (rt *Runtime) CmdRegistry() *commands.Registry {
	return rt.cmdRegistry
}

// --- commands.RuntimeAccessor implementation ---

// ListSkills converts all skills from the index to []SkillInfo.
func (rt *Runtime) ListSkills() []commands.SkillInfo {
	all := rt.skillIndex.List()
	infos := make([]commands.SkillInfo, 0, len(all))
	for _, s := range all {
		infos = append(infos, commands.SkillInfo{
			Name:        s.Name,
			Description: s.Description,
			Source:      s.Source,
			Priority:    s.Priority,
			Tags:        s.Tags,
		})
	}
	return infos
}

// ActivateSkill verifies the skill exists and adds it to the active list.
func (rt *Runtime) ActivateSkill(name string) error {
	matches := rt.skillIndex.MatchByName(name)
	if len(matches) == 0 {
		return fmt.Errorf("skill %q not found", name)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.activeSkills = append(rt.activeSkills, name)
	return nil
}

// ReloadSkillsAndRules reloads all skills from disk and updates the index.
func (rt *Runtime) ReloadSkillsAndRules() error {
	userSkillsDir := filepath.Join(rt.userDir, "skills")
	projectSkillsDir := "skills"
	loadedSkills, err := skills.LoadMultiLayer(userSkillsDir, projectSkillsDir)
	if err != nil {
		return fmt.Errorf("reloading skills: %w", err)
	}
	rt.skillIndex.Reload(loadedSkills)

	// Re-register skill commands so new skills appear in autocomplete.
	for _, s := range loadedSkills {
		rt.cmdRegistry.Register(builtin.NewSkillRun(s.Name, s.Description))
	}
	return nil
}

// LastPromptTrace returns the string representation of the last prompt trace, or "".
func (rt *Runtime) LastPromptTrace() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTrace == nil {
		return ""
	}
	return rt.lastTrace.String()
}

// ListTools converts all registered tools to []ToolInfo.
func (rt *Runtime) ListTools() []commands.ToolInfo {
	schemas := rt.toolRegistry.ToolSchemas()
	infos := make([]commands.ToolInfo, 0, len(schemas))
	for _, s := range schemas {
		infos = append(infos, commands.ToolInfo{
			Name:        s.Function.Name,
			Description: s.Function.Description,
		})
	}
	return infos
}

// SwitchProvider looks up the provider in config, creates a new client, and resets the agent.
func (rt *Runtime) SwitchProvider(name string) error {
	providerCfg, ok := rt.cfg.Providers[name]
	if !ok {
		return fmt.Errorf("provider %q not found in config", name)
	}
	llm := rt.createClient(name, providerCfg)
	agentCfg := agent.AgentConfig{
		MaxIterations: rt.cfg.MaxIterations,
		Model:         providerCfg.Model,
		MemoryEnabled: rt.memory != nil,
	}
	rt.agent.Reset(llm, agentCfg)

	rt.mu.Lock()
	rt.provider = name
	rt.model = providerCfg.Model
	rt.mu.Unlock()

	return nil
}

// SwitchModel updates the model field on the runtime.
func (rt *Runtime) SwitchModel(name string) error {
	rt.mu.Lock()
	rt.model = name
	rt.mu.Unlock()
	return nil
}

// CurrentProvider returns the current provider name.
func (rt *Runtime) CurrentProvider() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.provider
}

// CurrentModel returns the current model name.
func (rt *Runtime) CurrentModel() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.model
}

// AvailableProviders returns a list of all provider names from config.
func (rt *Runtime) AvailableProviders() []string {
	providers := make([]string, 0, len(rt.cfg.Providers))
	for name := range rt.cfg.Providers {
		providers = append(providers, name)
	}
	return providers
}

// ClearHistory delegates to the agent.
func (rt *Runtime) ClearHistory() {
	rt.agent.ClearHistory()
}

// CreateSkill creates a new skill via the manager and registers it as a slash command.
func (rt *Runtime) CreateSkill(req commands.CreateSkillRequest) (commands.CreateSkillResult, error) {
	draft := skills.SkillDraft{
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Keywords:    req.Keywords,
		Patterns:    req.Patterns,
		Priority:    req.Priority,
		Body:        req.Body,
		Category:    req.Category,
	}

	var skill *skills.Skill
	var err error
	if req.ProjectLevel {
		skill, err = rt.skillManager.CreateInProject(draft)
	} else {
		skill, err = rt.skillManager.Create(draft)
	}
	if err != nil {
		return commands.CreateSkillResult{}, err
	}

	// Re-register the new skill as a slash command.
	rt.cmdRegistry.Register(builtin.NewSkillRun(skill.Name, skill.Description))

	msg := fmt.Sprintf(
		"[1/3] 解析完成: name=%s, category=%s\n[2/3] 已写入: %s\n[3/3] 已注册: /%s 可用",
		skill.Name, skill.Category, skill.BasePath, skill.Name,
	)
	return commands.CreateSkillResult{
		Name:    skill.Name,
		Path:    skill.BasePath,
		Message: msg,
	}, nil
}

// DeleteSkill removes a skill by name.
func (rt *Runtime) DeleteSkill(name string) error {
	return rt.skillManager.Delete(name)
}

// UpdateSkill updates an existing skill's content.
func (rt *Runtime) UpdateSkill(name string, req commands.CreateSkillRequest) (commands.CreateSkillResult, error) {
	draft := skills.SkillDraft{
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Keywords:    req.Keywords,
		Patterns:    req.Patterns,
		Priority:    req.Priority,
		Body:        req.Body,
		Category:    req.Category,
	}
	skill, err := rt.skillManager.Update(name, draft)
	if err != nil {
		return commands.CreateSkillResult{}, err
	}
	return commands.CreateSkillResult{
		Name:    skill.Name,
		Path:    skill.BasePath,
		Message: fmt.Sprintf("Skill '%s' updated", skill.Name),
	}, nil
}
