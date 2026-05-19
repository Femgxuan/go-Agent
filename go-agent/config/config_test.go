package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
max_iterations: 5
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    model: gpt-4o
  deepseek:
    api_key: sk-ds
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat
tools:
  tavily:
    api_key: tvly-test
  shell:
    blocked_commands: ["sudo", "rm -rf /"]
  file:
    max_read_size: 2097152
`), 0644)

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai", cfg.DefaultProvider)
	}
	if cfg.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d, want 5", cfg.MaxIterations)
	}
	p := cfg.Providers["openai"]
	if p.APIKey != "sk-test" || p.Model != "gpt-4o" {
		t.Errorf("openai provider = %+v", p)
	}
	ds := cfg.Providers["deepseek"]
	if ds.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("deepseek base_url = %q", ds.BaseURL)
	}
	if cfg.Tools.Tavily.APIKey != "tvly-test" {
		t.Errorf("tavily api_key = %q", cfg.Tools.Tavily.APIKey)
	}
	if cfg.Tools.File.MaxReadSize != 2097152 {
		t.Errorf("max_read_size = %d", cfg.Tools.File.MaxReadSize)
	}
}

func TestEnvVarOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
providers:
  openai:
    api_key: sk-from-file
    base_url: https://api.openai.com/v1
    model: gpt-4o
tools:
  tavily:
    api_key: tvly-from-file
`), 0644)

	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("TAVILY_API_KEY", "tvly-from-env")

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	ApplyEnvOverrides(cfg)

	if cfg.Providers["openai"].APIKey != "sk-from-env" {
		t.Errorf("openai api_key = %q, want sk-from-env", cfg.Providers["openai"].APIKey)
	}
	if cfg.Tools.Tavily.APIKey != "tvly-from-env" {
		t.Errorf("tavily api_key = %q, want tvly-from-env", cfg.Tools.Tavily.APIKey)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    model: gpt-4o
`), 0644)

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("MaxIterations default = %d, want 10", cfg.MaxIterations)
	}
	if cfg.Tools.File.MaxReadSize != 1048576 {
		t.Errorf("MaxReadSize default = %d, want 1048576", cfg.Tools.File.MaxReadSize)
	}
}
