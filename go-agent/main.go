package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
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

	// 4. Build tool registry
	registry := buildRegistry(cfg)

	// 5. Create runtime coordinator
	rt, err := runtime.New(runtime.Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: createClient,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot create runtime: %v\n", err)
		os.Exit(1)
	}

	// 6. Build TUI AppConfig
	appCfg := tui.AppConfig{
		Runtime: rt,
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
func buildRegistry(cfg *config.Config) *tools.Registry {
	registry := tools.NewRegistry()

	tavilyBaseURL := "https://api.tavily.com/search"
	registry.Register(tools.NewTavilySearch(cfg.Tools.Tavily.APIKey, tavilyBaseURL))
	registry.Register(tools.NewShellExec(cfg.Tools.Shell.BlockedCommands))
	registry.Register(tools.NewReadFile(cfg.Tools.File.MaxReadSize))
	registry.Register(tools.NewWriteFile())

	return registry
}
