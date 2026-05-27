package memory

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config is the configuration for the memory system.
type Config struct {
	// Working memory configuration
	Working struct {
		MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
		Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"` // "simple" | "tiktoken"
	} `yaml:"working"`

	// Short-term memory configuration
	ShortTerm struct {
		StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
		MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
	} `yaml:"short_term"`

	// Long-term memory configuration
	LongTerm struct {
		PostgresURL string `yaml:"postgres_url" env:"MEMORY_LONGTERM_PG_URL"`
		Embedder    struct {
			Provider string `yaml:"provider" env:"MEMORY_EMBEDDER_PROVIDER"` // "openai" | "ollama"
			APIKey   string `yaml:"api_key" env:"MEMORY_EMBEDDER_API_KEY"`
			Model    string `yaml:"model" env:"MEMORY_EMBEDDER_MODEL"`
			BaseURL  string `yaml:"base_url" env:"MEMORY_EMBEDDER_BASE_URL"` // for ollama
		} `yaml:"embedder"`
		HalfLife time.Duration `yaml:"half_life" env:"MEMORY_LONGTERM_HALF_LIFE"`
	} `yaml:"long_term"`

	// Meta-memory configuration
	Meta struct {
		StorageDir string `yaml:"storage_dir" env:"MEMORY_META_DIR"`
	} `yaml:"meta"`

	// Compaction configuration
	Compaction struct {
		Interval  time.Duration `yaml:"interval" env:"MEMORY_COMPACTION_INTERVAL"`
		OlderThan time.Duration `yaml:"older_than" env:"MEMORY_COMPACTION_OLDER_THAN"`
	} `yaml:"compaction"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	cfg := &Config{}

	// Working memory defaults
	cfg.Working.MaxTokens = 8192
	cfg.Working.Tokenizer = "simple"

	// Short-term memory defaults
	home, _ := os.UserHomeDir()
	cfg.ShortTerm.StorageDir = filepath.Join(home, ".go-agent", "memory", "sessions")
	cfg.ShortTerm.MaxSessions = 100

	// Long-term memory defaults
	cfg.LongTerm.PostgresURL = "postgres://localhost:5432/goagent"
	cfg.LongTerm.Embedder.Provider = "openai"
	cfg.LongTerm.Embedder.Model = "text-embedding-3-small"
	cfg.LongTerm.HalfLife = 30 * 24 * time.Hour // 30 days

	// Meta-memory defaults
	cfg.Meta.StorageDir = filepath.Join(home, ".go-agent", "memory", "reflections")

	// Compaction defaults
	cfg.Compaction.Interval = 24 * time.Hour      // compact once per day
	cfg.Compaction.OlderThan = 7 * 24 * time.Hour  // sessions older than 7 days

	return cfg
}

// ApplyEnvOverrides loads configuration from environment variables.
func ApplyEnvOverrides(cfg *Config) {
	if v := os.Getenv("MEMORY_WORKING_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Working.MaxTokens = n
		}
	}
	if v := os.Getenv("MEMORY_SHORTTERM_DIR"); v != "" {
		cfg.ShortTerm.StorageDir = v
	}
	if v := os.Getenv("MEMORY_LONGTERM_PG_URL"); v != "" {
		cfg.LongTerm.PostgresURL = v
	}
	if v := os.Getenv("MEMORY_EMBEDDER_PROVIDER"); v != "" {
		cfg.LongTerm.Embedder.Provider = v
	}
	if v := os.Getenv("MEMORY_EMBEDDER_API_KEY"); v != "" {
		cfg.LongTerm.Embedder.APIKey = v
	}
	if v := os.Getenv("MEMORY_EMBEDDER_MODEL"); v != "" {
		cfg.LongTerm.Embedder.Model = v
	}
	if v := os.Getenv("MEMORY_EMBEDDER_BASE_URL"); v != "" {
		cfg.LongTerm.Embedder.BaseURL = v
	}
}
