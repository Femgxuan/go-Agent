package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Manager is the unified entry point for the memory system, coordinating four layers of memory.
type Manager interface {
	// Retrieve gets memories relevant to the current query, to be injected into the system prompt.
	Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error)

	// Store saves a single interaction (user input + agent output).
	Store(ctx context.Context, interaction Interaction) error

	// AddToWorkingMemory adds a message to working memory without requiring an active session.
	// Used to make the current user message immediately available to buildMessages().
	AddToWorkingMemory(msg Message) error

	// Memorize explicitly remembers a fact (called by user or agent).
	Memorize(ctx context.Context, fact Fact) error

	// StartSession starts a new session and returns the session ID.
	StartSession(ctx context.Context, userID string) (SessionID, error)

	// EndSession ends a session, triggering short-term memory persistence.
	EndSession(ctx context.Context, sid SessionID) error

	// Compact compresses expired memories (background task).
	Compact(ctx context.Context) error

	// Forget selectively forgets memories.
	Forget(ctx context.Context, filter ForgetFilter) error

	// Episodic returns the episodic memory store (may be nil if disabled).
	Episodic() EpisodicStore

	// Semantic returns the semantic memory store.
	Semantic() SemanticStore

	// Skills returns the skill store (may be nil if not configured).
	Skills() SkillStore

	// Compressor returns the context compressor (may be nil if not configured).
	Compressor() *ContextCompressor
}

// DefaultManager implements the Manager interface.
type DefaultManager struct {
	mu             sync.RWMutex
	config         *Config
	working        *WorkingMemoryImpl
	shortTerm      ShortTermMemory
	longTerm       LongTermMemory
	meta           MetaMemory
	currentSession SessionID
	sessionCounter int
	episodic       EpisodicStore
	skillStore     SkillStore
	compressor     *ContextCompressor
}

// NewManager creates a new DefaultManager.
func NewManager(cfg *Config) (*DefaultManager, error) {
	// Create tokenizer (prefer tiktoken, fallback to simple).
	var tokenizer Tokenizer
	tk, err := NewTiktokenTokenizer("gpt-4")
	if err != nil {
		slog.Warn("tiktoken unavailable, using simple tokenizer", "error", err)
		tokenizer = &SimpleTokenizer{}
	} else {
		tokenizer = tk
	}

	// Create working memory.
	working := NewWorkingMemory(cfg.Working.MaxTokens, tokenizer)

	// Create short-term memory.
	shortTerm := NewFileShortTermMemory(cfg.ShortTerm.StorageDir)

	// Create meta memory.
	meta := NewFileMetaMemory(cfg.Meta.StorageDir)

	// Try to create long-term memory (pgvector). Degrade gracefully if unavailable.
	var longTerm LongTermMemory
	var pool *pgxpool.Pool
	var embedder Embedder
	if cfg.LongTerm.PostgresURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var err error
		pool, err = NewPgPool(ctx, cfg.LongTerm.PostgresURL)
		if err != nil {
			slog.Warn("PostgreSQL unavailable, long-term memory disabled", "error", err, "url", cfg.LongTerm.PostgresURL)
		} else {
			// Create embedder based on provider.
			switch cfg.LongTerm.Embedder.Provider {
			case "ollama":
				baseURL := cfg.LongTerm.Embedder.BaseURL
				if baseURL == "" {
					baseURL = "http://localhost:11434"
				}
				embedder = NewOllamaEmbedder(baseURL, cfg.LongTerm.Embedder.Model)
				slog.Info("using Ollama embedder", "baseURL", baseURL, "model", cfg.LongTerm.Embedder.Model)
			case "hash":
				embedder = NewHashEmbedder(1536)
				slog.Info("using hash embedder (no API needed)")
			case "huggingface":
				baseURL := cfg.LongTerm.Embedder.BaseURL
				if baseURL == "" {
					baseURL = fmt.Sprintf("https://api-inference.huggingface.co/models/%s/embeddings", cfg.LongTerm.Embedder.Model)
				}
				embedder = NewOpenAIEmbedder(cfg.LongTerm.Embedder.APIKey, cfg.LongTerm.Embedder.Model, baseURL)
				slog.Info("using HuggingFace embedder", "baseURL", baseURL, "model", cfg.LongTerm.Embedder.Model)
			case "modelscope":
				baseURL := cfg.LongTerm.Embedder.BaseURL
				if baseURL == "" {
					baseURL = "https://api-inference.modelscope.cn/v1/embeddings"
				}
				embedder = NewOpenAIEmbedder(cfg.LongTerm.Embedder.APIKey, cfg.LongTerm.Embedder.Model, baseURL)
				slog.Info("using ModelScope embedder", "baseURL", baseURL, "model", cfg.LongTerm.Embedder.Model)
			default: // "openai"
				if cfg.LongTerm.Embedder.APIKey == "" {
					slog.Warn("OpenAI API key not set, falling back to hash embedder")
					embedder = NewHashEmbedder(1536)
				} else {
					embedder = NewOpenAIEmbedder(cfg.LongTerm.Embedder.APIKey, cfg.LongTerm.Embedder.Model, "")
					slog.Info("using OpenAI embedder", "model", cfg.LongTerm.Embedder.Model)
				}
			}
			longTerm = NewPgLongTermMemory(pool, embedder)
			slog.Info("long-term memory enabled", "provider", cfg.LongTerm.Embedder.Provider, "postgres", cfg.LongTerm.PostgresURL)
		}
	}

	// Create episodic store.
	var episodic EpisodicStore
	if cfg.Episodic.Enabled && pool != nil {
		episodic = NewPgEpisodicStore(pool, nil, tokenizer, embedder,
			cfg.LongTerm.Hybrid.FTSWeight, cfg.LongTerm.Hybrid.VectorWeight, cfg.LongTerm.Hybrid.RRFK)
		slog.Info("episodic memory enabled")
	}

	// Create skill store.
	var skillStore SkillStore
	if cfg.Skills.Dir != "" {
		skillStore = NewFileSkillStore(cfg.Skills.Dir, cfg.Skills.MaxIndex)
		slog.Info("skill store enabled", "dir", cfg.Skills.Dir)
	}

	// Create context compressor.
	compressor := NewContextCompressor(working, nil, tokenizer, cfg.Working.MaxTokens, cfg.Working.CompressionThreshold)

	return &DefaultManager{
		config:     cfg,
		working:    working,
		shortTerm:  shortTerm,
		longTerm:   longTerm,
		meta:       meta,
		episodic:   episodic,
		skillStore: skillStore,
		compressor: compressor,
	}, nil
}

// Retrieve gets memories relevant to the current query.
func (m *DefaultManager) Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error) {
	result := &Context{}

	// Working memory: always available.
	result.WorkingMemory = m.working.GetWindow(opts.MaxTokens)

	// Long-term memory: may fail, degrade to empty results.
	if m.longTerm != nil {
		slog.Info("[retrieve] searching long-term memory", "query", query, "maxFacts", opts.MaxFacts, "minScore", opts.MinScore)
		facts, err := m.longTerm.Search(ctx, query, opts.MaxFacts, opts.MinScore)
		if err != nil {
			slog.Warn("[retrieve] long-term memory search failed, degrading", "error", err)
		} else {
			slog.Info("[retrieve] found facts", "count", len(facts))
			result.RelevantFacts = facts
		}
	} else {
		slog.Info("[retrieve] long-term memory is nil, skipping search")
	}

	// Meta memory: may fail, degrade to empty results.
	if m.meta != nil {
		reflections, err := m.meta.Recent(ctx, opts.MaxReflections)
		if err != nil {
			slog.Warn("meta memory retrieval failed, degrading", "error", err)
		} else {
			result.SelfReflection = reflections
		}
	}

	return result, nil
}

// Store saves a single interaction to both short-term memory and working memory.
func (m *DefaultManager) Store(ctx context.Context, interaction Interaction) error {
	m.mu.RLock()
	sid := m.currentSession
	m.mu.RUnlock()

	if sid == "" {
		return ErrNoActiveSession
	}

	interaction.SessionID = sid

	// Save to short-term memory for persistence.
	return m.shortTerm.Save(ctx, sid, interaction)
}

// AddToWorkingMemory adds a message to working memory without requiring an active session.
func (m *DefaultManager) AddToWorkingMemory(msg Message) error {
	return m.working.Add(msg)
}

// Memorize explicitly remembers a fact.
func (m *DefaultManager) Memorize(ctx context.Context, fact Fact) error {
	// Use category as key prefix for working memory.
	key := string(fact.Category)
	if fact.Key != "" {
		key = fact.Key
	}
	m.working.Remember(key, fact.Content)
	slog.Info("[memorize] stored in working memory", "key", key, "content", fact.Content)

	// Facts table deprecated — no longer writing to long-term storage.
	// Knowledge is now stored in markdown files (SOUL/MEMORY/USER.md)
	// and conversation history in episodes table.

	return nil
}

// StartSession starts a new session.
func (m *DefaultManager) StartSession(ctx context.Context, userID string) (SessionID, error) {
	m.mu.Lock()
	m.sessionCounter++
	sid := SessionID(fmt.Sprintf("sess_%s_%03d",
		time.Now().Format("20060102"),
		m.sessionCounter))
	m.currentSession = sid
	m.mu.Unlock()

	// Initialize empty session.
	err := m.shortTerm.Save(ctx, sid, Interaction{
		SessionID: sid,
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	return sid, nil
}

// EndSession ends a session.
func (m *DefaultManager) EndSession(ctx context.Context, sid SessionID) error {
	m.mu.Lock()
	if m.currentSession == sid {
		m.currentSession = ""
	}
	m.mu.Unlock()

	return nil
}

// Compact compresses expired memories.
func (m *DefaultManager) Compact(ctx context.Context) error {
	// TODO: implement compaction logic.
	return nil
}

// Forget selectively forgets memories.
func (m *DefaultManager) Forget(ctx context.Context, filter ForgetFilter) error {
	// TODO: implement forget logic.
	return nil
}

// Episodic returns the episodic memory store.
func (m *DefaultManager) Episodic() EpisodicStore {
	return m.episodic
}

// Semantic returns the semantic memory store.
func (m *DefaultManager) Semantic() SemanticStore {
	if m.longTerm != nil {
		return m.longTerm.(SemanticStore)
	}
	return nil
}

// Skills returns the skill store.
func (m *DefaultManager) Skills() SkillStore {
	return m.skillStore
}

// Compressor returns the context compressor.
func (m *DefaultManager) Compressor() *ContextCompressor {
	return m.compressor
}
