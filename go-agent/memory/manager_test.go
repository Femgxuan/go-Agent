package memory

import (
	"context"
	"testing"
	"time"

	"github.com/fengxuan/go-agent/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_StartSession(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Working: struct {
			MaxTokens            int     `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer            string  `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
			CompressionThreshold float64 `yaml:"compression_threshold" env:"MEMORY_COMPRESSION_THRESHOLD"`
		}{
			MaxTokens:            100,
			Tokenizer:            "simple",
			CompressionThreshold: 0.85,
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg, &config.Config{})
	require.NoError(t, err)

	ctx := context.Background()
	sid, err := mgr.StartSession(ctx, "test-user")
	require.NoError(t, err)
	assert.NotEmpty(t, sid)
}

func TestManager_Store(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Working: struct {
			MaxTokens            int     `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer            string  `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
			CompressionThreshold float64 `yaml:"compression_threshold" env:"MEMORY_COMPRESSION_THRESHOLD"`
		}{
			MaxTokens:            100,
			Tokenizer:            "simple",
			CompressionThreshold: 0.85,
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg, &config.Config{})
	require.NoError(t, err)

	ctx := context.Background()
	sid, _ := mgr.StartSession(ctx, "test-user")

	interaction := Interaction{
		SessionID: sid,
		UserMsg:   "hello",
		AgentMsg:  "hi there",
		Metadata:  map[string]any{"timestamp": time.Now()},
	}

	err = mgr.Store(ctx, interaction)
	require.NoError(t, err)
}

func TestManager_Retrieve(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Working: struct {
			MaxTokens            int     `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer            string  `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
			CompressionThreshold float64 `yaml:"compression_threshold" env:"MEMORY_COMPRESSION_THRESHOLD"`
		}{
			MaxTokens:            100,
			Tokenizer:            "simple",
			CompressionThreshold: 0.85,
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg, &config.Config{})
	require.NoError(t, err)

	ctx := context.Background()

	// Add messages to working memory.
	mgr.working.Add(Message{Role: "user", Content: "hello", Timestamp: time.Now()})
	mgr.working.Add(Message{Role: "assistant", Content: "hi", Timestamp: time.Now()})

	memCtx, err := mgr.Retrieve(ctx, "hello", RetrieveOptions{
		MaxTokens: 100,
	})
	require.NoError(t, err)
	assert.Len(t, memCtx.WorkingMemory, 2)
}

func TestManager_Memorize(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Working: struct {
			MaxTokens            int     `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer            string  `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
			CompressionThreshold float64 `yaml:"compression_threshold" env:"MEMORY_COMPRESSION_THRESHOLD"`
		}{
			MaxTokens:            100,
			Tokenizer:            "simple",
			CompressionThreshold: 0.85,
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg, &config.Config{})
	require.NoError(t, err)

	ctx := context.Background()
	fact := Fact{
		ID:        "fact-1",
		Key:       "project",
		Content:   "Go Agent是一个ReAct风格的Agent框架",
		Source:    "user",
		CreatedAt: time.Now(),
	}

	err = mgr.Memorize(ctx, fact)
	require.NoError(t, err)
}
