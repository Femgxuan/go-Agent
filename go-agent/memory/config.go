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
		MaxTokens            int     `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
		Tokenizer            string  `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"` // "simple" | "tiktoken"
		CompressionThreshold float64 `yaml:"compression_threshold" env:"MEMORY_COMPRESSION_THRESHOLD"`
	} `yaml:"working"`

	// Short-term memory configuration
	ShortTerm struct {
		StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
		MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
	} `yaml:"short_term"`

	// Long-term memory configuration
	LongTerm struct {
		PostgresURL     string `yaml:"postgres_url" env:"MEMORY_LONGTERM_PG_URL"`
		EmbedderProvider string `yaml:"embedder_provider" env:"MEMORY_EMBEDDER_PROVIDER"` // references top-level embedding config
		HalfLife       time.Duration `yaml:"half_life" env:"MEMORY_LONGTERM_HALF_LIFE"`
		Hybrid         struct {
			FTSWeight    float64 `yaml:"fts_weight" env:"MEMORY_HYBRID_FTS_WEIGHT"`
			VectorWeight float64 `yaml:"vector_weight" env:"MEMORY_HYBRID_VECTOR_WEIGHT"`
			RRFK         int     `yaml:"rrf_k" env:"MEMORY_HYBRID_RRF_K"`
		} `yaml:"hybrid"`
	} `yaml:"long_term"`

	// Episodic memory configuration
	Episodic struct {
		Enabled bool `yaml:"enabled" env:"MEMORY_EPISODIC_ENABLED"`
	} `yaml:"episodic"`

	// Semantic memory configuration
	Semantic struct {
		UseVector bool `yaml:"use_vector" env:"MEMORY_SEMANTIC_USE_VECTOR"`
	} `yaml:"semantic"`

	// Skills configuration
	Skills struct {
		Dir      string `yaml:"dir" env:"MEMORY_SKILLS_DIR"`
		MaxIndex int    `yaml:"max_index" env:"MEMORY_SKILLS_MAX_INDEX"`
	} `yaml:"skills"`

	// Memory files directory (SOUL.md, MEMORY.md, USER.md)
	MemoryDir string `yaml:"memory_dir" env:"MEMORY_DIR"`

	// Auto-write control for markdown memory
	AutoWrite bool `yaml:"auto_write" env:"MEMORY_AUTO_WRITE"`

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
	cfg.Working.CompressionThreshold = 0.85

	// Short-term memory defaults
	home, _ := os.UserHomeDir()
	cfg.ShortTerm.StorageDir = filepath.Join(home, ".go-agent", "memory", "sessions")
	cfg.ShortTerm.MaxSessions = 100

	// Long-term memory defaults (匹配 docker-compose.yml 凭据)
	cfg.LongTerm.PostgresURL = "postgres://FengXuan:12345678@localhost:5432/FengXuan"
	cfg.LongTerm.EmbedderProvider = "openai"
	cfg.LongTerm.HalfLife = 30 * 24 * time.Hour // 30 days
	cfg.LongTerm.Hybrid.FTSWeight = 0.3
	cfg.LongTerm.Hybrid.VectorWeight = 0.7
	cfg.LongTerm.Hybrid.RRFK = 60

	// Episodic memory defaults
	cfg.Episodic.Enabled = true

	// Semantic memory defaults
	cfg.Semantic.UseVector = false

	// Skills defaults
	cfg.Skills.Dir = filepath.Join(home, ".go-agent", "skills")
	cfg.Skills.MaxIndex = 20

	// Memory files directory
	cfg.MemoryDir = filepath.Join(home, ".go-agent")

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
	if v := os.Getenv("MEMORY_COMPRESSION_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Working.CompressionThreshold = f
		}
	}
	if v := os.Getenv("MEMORY_SHORTTERM_DIR"); v != "" {
		cfg.ShortTerm.StorageDir = v
	}
	if v := os.Getenv("MEMORY_LONGTERM_PG_URL"); v != "" {
		cfg.LongTerm.PostgresURL = v
	}
	if v := os.Getenv("MEMORY_EMBEDDER_PROVIDER"); v != "" {
		cfg.LongTerm.EmbedderProvider = v
	}
	if v := os.Getenv("MEMORY_HYBRID_FTS_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LongTerm.Hybrid.FTSWeight = f
		}
	}
	if v := os.Getenv("MEMORY_HYBRID_VECTOR_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.LongTerm.Hybrid.VectorWeight = f
		}
	}
	if v := os.Getenv("MEMORY_HYBRID_RRF_K"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.LongTerm.Hybrid.RRFK = n
		}
	}
	if v := os.Getenv("MEMORY_DIR"); v != "" {
		cfg.MemoryDir = v
	}
	if v := os.Getenv("MEMORY_AUTO_WRITE"); v == "true" {
		cfg.AutoWrite = true
	}
}
