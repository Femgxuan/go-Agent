package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ProviderConfig holds configuration for an LLM provider.
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

// TavilyConfig holds configuration for the Tavily search tool.
type TavilyConfig struct {
	APIKey string `yaml:"api_key"`
}

// ShellConfig holds configuration for the shell tool.
type ShellConfig struct {
	BlockedCommands []string `yaml:"blocked_commands"`
}

// FileConfig holds configuration for the file tool.
type FileConfig struct {
	MaxReadSize int64 `yaml:"max_read_size"`
}

// ToolsConfig aggregates all tool configurations.
type ToolsConfig struct {
	Tavily TavilyConfig `yaml:"tavily"`
	Shell  ShellConfig  `yaml:"shell"`
	File   FileConfig   `yaml:"file"`
}

// Config is the top-level configuration structure.
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	MaxIterations   int                       `yaml:"max_iterations"`
	Providers       map[string]ProviderConfig `yaml:"providers"`
	Tools           ToolsConfig               `yaml:"tools"`
}

// LoadFromFile reads a YAML config file and applies defaults.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply defaults
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 10
	}
	if cfg.Tools.File.MaxReadSize == 0 {
		cfg.Tools.File.MaxReadSize = 1048576
	}

	return cfg, nil
}

// ApplyEnvOverrides overrides config values with environment variables when set.
func ApplyEnvOverrides(cfg *Config) {
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	// OPENAI_API_KEY
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		p := cfg.Providers["openai"]
		p.APIKey = v
		cfg.Providers["openai"] = p
	}

	// OPENAI_BASE_URL
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		p := cfg.Providers["openai"]
		p.BaseURL = v
		cfg.Providers["openai"] = p
	}

	// ANTHROPIC_API_KEY
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		p := cfg.Providers["anthropic"]
		p.APIKey = v
		cfg.Providers["anthropic"] = p
	}

	// DEEPSEEK_API_KEY
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		p := cfg.Providers["deepseek"]
		p.APIKey = v
		cfg.Providers["deepseek"] = p
	}

	// TAVILY_API_KEY
	if v := os.Getenv("TAVILY_API_KEY"); v != "" {
		cfg.Tools.Tavily.APIKey = v
	}
}
