package memory

import (
	"context"
	"time"
)

// SessionID is the session identifier.
type SessionID string

// Context is the return result of Retrieve, containing all memories to be
// injected into the system prompt.
type Context struct {
	WorkingMemory  []Message    // sliding window of the current conversation
	RelevantFacts  []Fact       // semantic search results from long-term memory
	SelfReflection []Reflection // meta-memory: agent self-reflections
}

// Message is a single conversation message.
type Message struct {
	Role      string    // "user" | "assistant" | "system"
	Content   string
	Timestamp time.Time
}

// FactCategory classifies a fact for structured storage.
type FactCategory string

const (
	FactPreference  FactCategory = "preference"  // 用户偏好："我喜欢 Go"
	FactEnvironment FactCategory = "environment"  // 环境事实："服务器是 Debian 12"
	FactCorrection  FactCategory = "correction"   // 纠正信息："不要用 sudo"
	FactNorm        FactCategory = "norm"         // 项目规范："用 tab 缩进"
	FactMilestone   FactCategory = "milestone"    // 已完成工作："完成了 PG 迁移"
	FactExplicit    FactCategory = "explicit"     // 显式请求："记住 API key"
)

// Fact is a memorable piece of information.
type Fact struct {
	ID         string       `json:"id"`
	Category   FactCategory `json:"category"`    // 结构化分类
	Key        string       `json:"key"`         // 人类可读标题
	Content    string       `json:"content"`
	Source     string       `json:"source"`      // "user" | "agent" | "derived"
	Confidence float64      `json:"confidence"`  // LLM 分类置信度
	CreatedAt  time.Time    `json:"created_at"`
	DecayScore float64      `json:"decay_score"`
}

// Reflection is an agent self-reflection record.
type Reflection struct {
	ID        string
	Content   string    // reflection content
	Trigger   string    // event that triggered the reflection
	CreatedAt time.Time
}

// Interaction is a complete user-agent interaction.
type Interaction struct {
	SessionID SessionID
	UserMsg   string
	AgentMsg  string
	Metadata  map[string]any
}

// RetrieveOptions controls the behavior of Retrieve.
type RetrieveOptions struct {
	MaxTokens      int     // maximum token count for returned results
	MaxFacts       int     // maximum number of Facts to return
	MaxReflections int     // maximum number of Reflections to return
	MinScore       float64 // minimum relevance score for long-term memory
}

// LongTermMemory manages long-term memory (semantic search over facts).
type LongTermMemory interface {
	// Store stores a fact in long-term memory.
	Store(ctx context.Context, fact Fact) error

	// Search searches for relevant facts.
	Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error)

	// Update updates a fact's fields (e.g. decay_score).
	Update(ctx context.Context, id string, updates map[string]any) error

	// Delete deletes a fact by ID.
	Delete(ctx context.Context, id string) error
}

// ForgetFilter defines conditions for forgetting.
type ForgetFilter struct {
	SessionID  *SessionID // forget by session
	Before     *time.Time // forget items before this time
	KeyPattern *string    // forget by key pattern match
}
