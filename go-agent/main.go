package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/memory"
	"github.com/fengxuan/go-agent/runtime"
	"github.com/fengxuan/go-agent/tools"
	"github.com/fengxuan/go-agent/tui"
)

func main() {
	// 1. Determine config path
	cfgPath := os.Getenv("GO_AGENT_CONFIG")
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		cfgPath = filepath.Join(home, ".go-agent", "config.yaml")
	}

	// 2. Load config
	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load config from %s: %v\n", cfgPath, err)
		os.Exit(1)
	}

	// 3. Apply env overrides
	config.ApplyEnvOverrides(cfg)

	// 3.5. Configure slog to write to file instead of stderr
	if home, err := os.UserHomeDir(); err == nil {
		logDir := filepath.Join(home, ".go-agent")
		os.MkdirAll(logDir, 0o755)
		if logFile, err := os.OpenFile(filepath.Join(logDir, "debug.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo})))
		}
	}

	// 4. Initialize memory manager
	memCfg := memory.DefaultConfig()
	memory.ApplyEnvOverrides(memCfg)
	// Override with values from config.yaml
	if cfg.Memory.LongTerm.PostgresURL != "" {
		memCfg.LongTerm.PostgresURL = cfg.Memory.LongTerm.PostgresURL
	}
	if cfg.Memory.LongTerm.EmbedderProvider != "" {
		memCfg.LongTerm.EmbedderProvider = cfg.Memory.LongTerm.EmbedderProvider
	}
	if cfg.Memory.LongTerm.Hybrid.FTSWeight > 0 {
		memCfg.LongTerm.Hybrid.FTSWeight = cfg.Memory.LongTerm.Hybrid.FTSWeight
	}
	if cfg.Memory.LongTerm.Hybrid.VectorWeight > 0 {
		memCfg.LongTerm.Hybrid.VectorWeight = cfg.Memory.LongTerm.Hybrid.VectorWeight
	}
	if cfg.Memory.LongTerm.Hybrid.RRFK > 0 {
		memCfg.LongTerm.Hybrid.RRFK = cfg.Memory.LongTerm.Hybrid.RRFK
	}
	memManager, memErr := memory.NewManager(memCfg, cfg)
	if memErr != nil {
		fmt.Fprintf(os.Stderr, "warning: memory system unavailable: %v\n", memErr)
	}

	// 5. Build tool registry
	registry := buildRegistry(cfg, memManager)

	// 6. Create runtime coordinator
	rt, err := runtime.New(runtime.Config{
		AppConfig:     cfg,
		ToolRegistry:  registry,
		MemoryManager: memManager,
		CreateClient:  createClient,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create runtime: %v\n", err)
		os.Exit(1)
	}

	// 6. Build TUI AppConfig
	appCfg := tui.AppConfig{
		Runtime: rt,
		Display: cfg.Display,
	}

	// 7. Start bubbletea
	model := tui.NewModel(appCfg)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// createClient creates the appropriate LLM client based on the provider name.
func createClient(provider string, cfg config.ProviderConfig) client.LLMClient {
	if provider == "anthropic" {
		return client.NewAnthropicClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	}
	return client.NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
}

// buildRegistry creates a tool registry populated with all configured tools.
func buildRegistry(cfg *config.Config, memManager memory.Manager) *tools.Registry {
	registry := tools.NewRegistry()

	// Core tools
	tavilyBaseURL := "https://api.tavily.com/search"
	registry.Register(tools.NewTavilySearch(cfg.Tools.Tavily.APIKey, tavilyBaseURL))
	registry.Register(tools.NewShellExec(cfg.Tools.Shell.BlockedCommands))
	registry.Register(tools.NewReadFile(cfg.Tools.File.MaxReadSize))
	registry.Register(tools.NewWriteFile())

	// Memory tools (SOUL.md / MEMORY.md / USER.md management)
	memCfg := memory.DefaultConfig()
	registry.Register(tools.NewMemoryAppendTool(memCfg.MemoryDir))
	registry.Register(tools.NewMemoryReplaceTool(memCfg.MemoryDir))
	registry.Register(tools.NewMemoryDeleteTool(memCfg.MemoryDir))

	// Episodic memory tool (requires PG-backed episodic store)
	if memManager != nil && memManager.Episodic() != nil {
		registry.Register(tools.NewSessionSearchTool(memManager.Episodic()))
	}

	// Skill tools (filesystem-based skill store)
	if memManager != nil && memManager.Skills() != nil {
		registry.Register(tools.NewReadSkillTool(memManager.Skills()))
		registry.Register(tools.NewSkillManageTool(memManager.Skills()))
	}

	return registry
}
