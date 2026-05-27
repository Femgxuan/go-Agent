package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager is the unified entry point for the memory system, coordinating four layers of memory.
type Manager interface {
	// Retrieve gets memories relevant to the current query, to be injected into the system prompt.
	Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error)

	// Store saves a single interaction (user input + agent output).
	Store(ctx context.Context, interaction Interaction) error

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
}

// NewManager creates a new DefaultManager.
func NewManager(cfg *Config) (*DefaultManager, error) {
	// Create working memory.
	tokenizer := &SimpleTokenizer{}
	working := NewWorkingMemory(cfg.Working.MaxTokens, tokenizer)

	// Create short-term memory.
	shortTerm := NewFileShortTermMemory(cfg.ShortTerm.StorageDir)

	// Create meta memory.
	meta := NewFileMetaMemory(cfg.Meta.StorageDir)

	return &DefaultManager{
		config:    cfg,
		working:   working,
		shortTerm: shortTerm,
		meta:      meta,
	}, nil
}

// Retrieve gets memories relevant to the current query.
func (m *DefaultManager) Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error) {
	result := &Context{}

	// Working memory: always available.
	result.WorkingMemory = m.working.GetWindow(opts.MaxTokens)

	// Long-term memory: may fail, degrade to empty results.
	if m.longTerm != nil {
		facts, err := m.longTerm.Search(ctx, query, opts.MaxFacts, opts.MinScore)
		if err != nil {
			slog.Warn("long-term memory search failed, degrading", "error", err)
		} else {
			result.RelevantFacts = facts
		}
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

// Store saves a single interaction.
func (m *DefaultManager) Store(ctx context.Context, interaction Interaction) error {
	m.mu.RLock()
	sid := m.currentSession
	m.mu.RUnlock()

	if sid == "" {
		return ErrNoActiveSession
	}

	interaction.SessionID = sid
	return m.shortTerm.Save(ctx, sid, interaction)
}

// Memorize explicitly remembers a fact.
func (m *DefaultManager) Memorize(ctx context.Context, fact Fact) error {
	// Store key fact in working memory.
	m.working.Remember(fact.Key, fact.Content)

	// If long-term memory is available, store there too.
	if m.longTerm != nil {
		if err := m.longTerm.Store(ctx, fact); err != nil {
			slog.Warn("failed to store fact in long-term memory", "error", err)
		}
	}

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
