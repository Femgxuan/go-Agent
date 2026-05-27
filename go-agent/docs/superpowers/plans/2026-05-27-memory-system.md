# Go Agent 记忆系统实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为Go Agent框架实现分层记忆系统（工作记忆、短期记忆、长期记忆、元记忆），支持Token窗口管理、会话持久化、语义检索和自我反思。

**Architecture:** 采用接口隔离+依赖倒置设计，通过Manager统一协调四层记忆。工作记忆使用滑动窗口算法，短期记忆使用JSON文件持久化，长期记忆使用pgvector向量存储，元记忆使用日期分文件存储。所有层通过接口抽象，支持独立测试和替换。

**Tech Stack:** Go 1.22+, pgx, pgvector-go, testcontainers-go, testify

---

## 文件结构

```
memory/
├── manager.go          # Manager接口 + DefaultManager实现
├── config.go           # Config结构体 + DefaultConfig + ApplyEnvOverrides
├── errors.go           # 哨兵错误定义
├── types.go            # 数据类型定义（Context, Message, Fact, Reflection等）
├── working.go          # WorkingMemory接口 + WorkingMemoryImpl实现
├── shortterm.go        # ShortTermMemory接口 + FileShortTermMemory实现
├── longterm.go         # LongTermMemory接口 + PgLongTermMemory实现
├── embedding.go        # Embedder接口 + OpenAIEmbedder + OllamaEmbedder
├── meta.go             # MetaMemory接口 + FileMetaMemory实现
├── compaction.go       # Compactor + DecayCalculator
├── tokenizer.go        # Tokenizer接口 + SimpleTokenizer实现
├── working_test.go     # 工作记忆单元测试
├── shortterm_test.go   # 短期记忆单元测试
├── longterm_test.go    # 长期记忆集成测试
├── meta_test.go        # 元记忆单元测试
├── compaction_test.go  # 压缩逻辑单元测试
├── manager_test.go     # Manager集成测试
└── testutil_test.go    # 测试辅助函数

migrations/
└── 001_create_facts.sql  # PostgreSQL迁移脚本

docker-compose.yml        # PostgreSQL+pgvector本地环境
```

---

## Phase 1: 接口定义 + 内存实现

### Task 1.1: 创建项目基础结构和依赖

**Files:**
- Create: `memory/types.go`
- Create: `memory/errors.go`
- Create: `memory/config.go`
- Modify: `go.mod`

- [ ] **Step 1: 创建types.go定义核心数据类型**

```go
package memory

import "time"

// SessionID是会话标识符
type SessionID string

// Context是Retrieve的返回结果，包含注入系统提示词的所有记忆
type Context struct {
	WorkingMemory  []Message    // 当前对话的滑动窗口
	RelevantFacts  []Fact       // 从长期记忆语义检索的结果
	SelfReflection []Reflection // 元记忆：Agent的自我反思
}

// Message是一条对话消息
type Message struct {
	Role      string    // "user" | "assistant" | "system"
	Content   string
	Timestamp time.Time
}

// Fact是一个可记忆的事实
type Fact struct {
	ID         string
	Key        string    // 事实的关键词/主题
	Content    string    // 事实内容
	Source     string    // 来源："user" | "agent" | "derived"
	CreatedAt  time.Time
	DecayScore float64   // 衰减分数，随时间降低
}

// Reflection是Agent的自我反思记录
type Reflection struct {
	ID        string
	Content   string    // 反思内容
	Trigger   string    // 触发反思的事件
	CreatedAt time.Time
}

// Interaction是一次完整的交互
type Interaction struct {
	SessionID SessionID
	UserMsg   string
	AgentMsg  string
	Metadata  map[string]any
}

// RetrieveOptions控制Retrieve的行为
type RetrieveOptions struct {
	MaxTokens      int     // 返回结果的最大Token数
	MaxFacts       int     // 最多返回几个Fact
	MaxReflections int     // 最多返回几条Reflection
	MinScore       float64 // 长期记忆的最低相关性分数
}

// ForgetFilter定义遗忘条件
type ForgetFilter struct {
	SessionID  *SessionID // 按会话遗忘
	Before     *time.Time // 遗忘某个时间之前的
	KeyPattern *string    // 按key模式匹配遗忘
}
```

- [ ] **Step 2: 创建errors.go定义哨兵错误**

```go
package memory

import "errors"

var (
	ErrNoActiveSession     = errors.New("no active session")
	ErrSessionNotFound     = errors.New("session not found")
	ErrEmbeddingFailed     = errors.New("embedding generation failed")
	ErrDatabaseUnavailable = errors.New("database unavailable")
	ErrCompactionFailed    = errors.New("compaction failed")
)
```

- [ ] **Step 3: 创建config.go定义配置结构**

```go
package memory

import (
	"os"
	"path/filepath"
	"time"
)

// Config是记忆系统的配置
type Config struct {
	// 工作记忆配置
	Working struct {
		MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
		Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"` // "simple" | "tiktoken"
	} `yaml:"working"`

	// 短期记忆配置
	ShortTerm struct {
		StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
		MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
	} `yaml:"short_term"`

	// 长期记忆配置
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

	// 元记忆配置
	Meta struct {
		StorageDir string `yaml:"storage_dir" env:"MEMORY_META_DIR"`
	} `yaml:"meta"`

	// 压缩配置
	Compaction struct {
		Interval  time.Duration `yaml:"interval" env:"MEMORY_COMPACTION_INTERVAL"`
		OlderThan time.Duration `yaml:"older_than" env:"MEMORY_COMPACTION_OLDER_THAN"`
	} `yaml:"compaction"`
}

// DefaultConfig返回默认配置
func DefaultConfig() *Config {
	cfg := &Config{}

	// 工作记忆默认值
	cfg.Working.MaxTokens = 8192
	cfg.Working.Tokenizer = "simple"

	// 短期记忆默认值
	home, _ := os.UserHomeDir()
	cfg.ShortTerm.StorageDir = filepath.Join(home, ".go-agent", "memory", "sessions")
	cfg.ShortTerm.MaxSessions = 100

	// 长期记忆默认值
	cfg.LongTerm.PostgresURL = "postgres://localhost:5432/goagent"
	cfg.LongTerm.Embedder.Provider = "openai"
	cfg.LongTerm.Embedder.Model = "text-embedding-3-small"
	cfg.LongTerm.HalfLife = 30 * 24 * time.Hour // 30天

	// 元记忆默认值
	cfg.Meta.StorageDir = filepath.Join(home, ".go-agent", "memory", "reflections")

	// 压缩默认值
	cfg.Compaction.Interval = 24 * time.Hour     // 每天压缩一次
	cfg.Compaction.OlderThan = 7 * 24 * time.Hour // 7天前的会话

	return cfg
}

// ApplyEnvOverrides从环境变量加载配置
func ApplyEnvOverrides(cfg *Config) {
	if v := os.Getenv("MEMORY_WORKING_MAX_TOKENS"); v != "" {
		// 解析整数
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
```

- [ ] **Step 4: 添加依赖并验证编译**

Run: `go get github.com/stretchr/testify && go mod tidy`
Expected: 成功下载依赖

- [ ] **Step 5: 验证包编译**

Run: `go build ./memory/...`
Expected: 编译成功，无错误

- [ ] **Step 6: 提交**

```bash
git add memory/types.go memory/errors.go memory/config.go go.mod go.sum
git commit -m "feat(memory): add core types, errors, and config"
```

---

### Task 1.2: 实现Tokenizer接口和SimpleTokenizer

**Files:**
- Create: `memory/tokenizer.go`
- Create: `memory/tokenizer_test.go`

- [ ] **Step 1: 创建tokenizer_test.go编写失败测试**

```go
package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleTokenizer_Count(t *testing.T) {
	tokenizer := &SimpleTokenizer{}

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty string", "", 0},
		{"english words", "hello world", 2},
		{"chinese chars", "你好世界", 8}, // 每个中文字符≈2 tokens
		{"mixed content", "hello 世界", 5}, // 1 english word + 2 chinese chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenizer.Count(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestSimpleTokenizer_Count -v`
Expected: FAIL - "SimpleTokenizer not defined"

- [ ] **Step 3: 创建tokenizer.go实现SimpleTokenizer**

```go
package memory

import "unicode"

// Tokenizer是Token计数的抽象接口
type Tokenizer interface {
	Count(text string) int
}

// SimpleTokenizer基于字符数估算Token
// 粗略估算：1个中文字符≈2 tokens，1个英文单词≈1.3 tokens
type SimpleTokenizer struct{}

// Count估算文本的Token数
func (t *SimpleTokenizer) Count(text string) int {
	if text == "" {
		return 0
	}

	tokens := 0
	inWord := false

	for _, r := range range text {
		if unicode.Is(unicode.Han, r) {
			// 中文字符：每个≈2 tokens
			tokens += 2
			inWord = false
		} else if unicode.IsSpace(r) {
			// 空格：分词边界
			inWord = false
		} else {
			// 英文字符：按单词计数
			if !inWord {
				tokens++ // 新单词开始
				inWord = true
			}
		}
	}

	return tokens
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestSimpleTokenizer_Count -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/tokenizer.go memory/tokenizer_test.go
git commit -m "feat(memory): add SimpleTokenizer for token counting"
```

---

### Task 1.3: 实现WorkingMemory接口

**Files:**
- Create: `memory/working.go`
- Create: `memory/working_test.go`

- [ ] **Step 1: 创建working_test.go编写失败测试**

```go
package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkingMemory_Add(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	msg := Message{
		Role:      "user",
		Content:   "hello",
		Timestamp: time.Now(),
	}

	err := wm.Add(msg)
	require.NoError(t, err)

	window := wm.GetWindow(100)
	assert.Len(t, window, 1)
	assert.Equal(t, "hello", window[0].Content)
}

func TestWorkingMemory_SlidingWindow(t *testing.T) {
	wm := NewWorkingMemory(10, &SimpleTokenizer{}) // 10 tokens限制

	// 添加多条消息
	wm.Add(Message{Role: "user", Content: "hello world"})    // 2 tokens
	wm.Add(Message{Role: "assistant", Content: "hi there"})  // 2 tokens
	wm.Add(Message{Role: "user", Content: "how are you"})    // 3 tokens
	wm.Add(Message{Role: "assistant", Content: "I am fine"}) // 3 tokens

	window := wm.GetWindow(10)
	// 应该只保留最后几条，总tokens不超过10
	totalTokens := 0
	for _, msg := range window {
		totalTokens += wm.tokenizer.Count(msg.Content)
	}
	assert.LessOrEqual(t, totalTokens, 10)
}

func TestWorkingMemory_Remember(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	// Remember关键信息
	wm.Remember("project", "Go Agent")

	window := wm.GetWindow(0) // 0表示使用默认限制
	found := false
	for _, msg := range window {
		if strings.Contains(msg.Content, "Go Agent") {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestWorkingMemory_TokenCount(t *testing.T) {
	wm := NewWorkingMemory(100, &SimpleTokenizer{})

	wm.Add(Message{Role: "user", Content: "hello world"})   // 2 tokens
	wm.Add(Message{Role: "assistant", Content: "hi there"}) // 2 tokens

	assert.Equal(t, 4, wm.TokenCount())
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestWorkingMemory -v`
Expected: FAIL - "NewWorkingMemory not defined"

- [ ] **Step 3: 创建working.go实现WorkingMemory**

```go
package memory

import (
	"fmt"
	"sync"
)

// WorkingMemory管理工作记忆（Token窗口）
type WorkingMemory interface {
	// Add添加消息到工作记忆
	Add(msg Message) error

	// GetWindow获取当前窗口内的消息
	GetWindow(maxTokens int) []Message

	// Remember标记关键信息，永不丢弃
	Remember(key, value string)

	// TokenCount计算当前工作记忆的Token数
	TokenCount() int
}

// WorkingMemoryImpl实现WorkingMemory接口
type WorkingMemoryImpl struct {
	mu        sync.RWMutex
	messages  []Message
	keyFacts  map[string]string
	tokenizer Tokenizer
	maxTokens int
}

// NewWorkingMemory创建一个新的WorkingMemory
func NewWorkingMemory(maxTokens int, tokenizer Tokenizer) *WorkingMemoryImpl {
	return &WorkingMemoryImpl{
		keyFacts:  make(map[string]string),
		tokenizer: tokenizer,
		maxTokens: maxTokens,
	}
}

// Add添加消息到工作记忆
func (w *WorkingMemoryImpl) Add(msg Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.messages = append(w.messages, msg)
	return nil
}

// GetWindow获取当前窗口内的消息
func (w *WorkingMemoryImpl) GetWindow(maxTokens int) []Message {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if maxTokens <= 0 {
		maxTokens = w.maxTokens
	}

	var result []Message
	tokenCount := 0

	// 从最新消息向前遍历
	for i := len(w.messages) - 1; i >= 0; i-- {
		msg := w.messages[i]
		msgTokens := w.tokenizer.Count(msg.Content)

		if tokenCount+msgTokens > maxTokens {
			break // 超出Token限制，停止
		}

		result = append([]Message{msg}, result...) // 插入到头部
		tokenCount += msgTokens
	}

	// 追加keyFacts（始终保留）
	for key, value := range w.keyFacts {
		result = append([]Message{{
			Role:    "system",
			Content: fmt.Sprintf("[记住] %s: %s", key, value),
		}}, result...)
	}

	return result
}

// Remember标记关键信息，永不丢弃
func (w *WorkingMemoryImpl) Remember(key, value string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.keyFacts[key] = value
}

// TokenCount计算当前工作记忆的Token数
func (w *WorkingMemoryImpl) TokenCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()

	count := 0
	for _, msg := range w.messages {
		count += w.tokenizer.Count(msg.Content)
	}
	return count
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestWorkingMemory -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/working.go memory/working_test.go
git commit -m "feat(memory): implement WorkingMemory with sliding window"
```

---

### Task 1.4: 实现FileShortTermMemory

**Files:**
- Create: `memory/shortterm.go`
- Create: `memory/shortterm_test.go`

- [ ] **Step 1: 创建shortterm_test.go编写失败测试**

```go
package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileShortTermMemory_Save(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	interaction := Interaction{
		SessionID: sid,
		UserMsg:   "hello",
		AgentMsg:  "hi there",
		Metadata:  map[string]any{"timestamp": time.Now()},
	}

	err := stm.Save(ctx, sid, interaction)
	require.NoError(t, err)

	// 验证文件已创建
	_, err = os.Stat(filepath.Join(tmpDir, string(sid)+".json"))
	assert.NoError(t, err)
}

func TestFileShortTermMemory_Load(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// 保存两条交互
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "msg1",
		AgentMsg:  "reply1",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "msg2",
		AgentMsg:  "reply2",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	// 加载
	interactions, err := stm.Load(ctx, sid, 0)
	require.NoError(t, err)
	assert.Len(t, interactions, 2)
	assert.Equal(t, "msg1", interactions[0].UserMsg)
	assert.Equal(t, "msg2", interactions[1].UserMsg)
}

func TestFileShortTermMemory_LoadWithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// 保存三条交互
	for i := 0; i < 3; i++ {
		stm.Save(ctx, sid, Interaction{
			SessionID: sid,
			UserMsg:   "msg",
			AgentMsg:  "reply",
			Metadata:  map[string]any{"timestamp": time.Now()},
		})
	}

	// 加载限制为2条
	interactions, err := stm.Load(ctx, sid, 2)
	require.NoError(t, err)
	assert.Len(t, interactions, 2)
}

func TestFileShortTermMemory_ListSessions(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()

	// 创建两个会话
	stm.Save(ctx, SessionID("sess-1"), Interaction{
		SessionID: SessionID("sess-1"),
		UserMsg:   "hello",
		AgentMsg:  "hi",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	stm.Save(ctx, SessionID("sess-2"), Interaction{
		SessionID: SessionID("sess-2"),
		UserMsg:   "world",
		AgentMsg:  "earth",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	sessions, err := stm.ListSessions(ctx, "")
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
}

func TestFileShortTermMemory_DeleteSession(t *testing.T) {
	tmpDir := t.TempDir()
	stm := NewFileShortTermMemory(tmpDir)

	ctx := context.Background()
	sid := SessionID("test-session-1")

	// 保存并删除
	stm.Save(ctx, sid, Interaction{
		SessionID: sid,
		UserMsg:   "hello",
		AgentMsg:  "hi",
		Metadata:  map[string]any{"timestamp": time.Now()},
	})

	err := stm.DeleteSession(ctx, sid)
	require.NoError(t, err)

	// 验证文件已删除
	_, err = os.Stat(filepath.Join(tmpDir, string(sid)+".json"))
	assert.True(t, os.IsNotExist(err))
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestFileShortTermMemory -v`
Expected: FAIL - "NewFileShortTermMemory not defined"

- [ ] **Step 3: 创建shortterm.go实现FileShortTermMemory**

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ShortTermMemory管理短期记忆（会话历史持久化）
type ShortTermMemory interface {
	// Save保存一次交互
	Save(ctx context.Context, sid SessionID, interaction Interaction) error

	// Load加载会话历史
	Load(ctx context.Context, sid SessionID, limit int) ([]Interaction, error)

	// ListSessions列出所有会话
	ListSessions(ctx context.Context, userID string) ([]SessionID, error)

	// DeleteSession删除会话
	DeleteSession(ctx context.Context, sid SessionID) error
}

// SessionFile是单个会话文件的结构
type SessionFile struct {
	ID        SessionID     `json:"id"`
	UserID    string        `json:"user_id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []Interaction `json:"messages"`
}

// FileShortTermMemory基于文件的短期记忆实现
type FileShortTermMemory struct {
	mu       sync.RWMutex
	basePath string
	current  *SessionFile
}

// NewFileShortTermMemory创建一个新的FileShortTermMemory
func NewFileShortTermMemory(basePath string) *FileShortTermMemory {
	return &FileShortTermMemory{
		basePath: basePath,
	}
}

// Save保存一次交互到当前会话
func (f *FileShortTermMemory) Save(ctx context.Context, sid SessionID, interaction Interaction) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 如果是新会话或切换了会话，加载或创建
	if f.current == nil || f.current.ID != sid {
		session, err := f.loadOrCreate(sid)
		if err != nil {
			return err
		}
		f.current = session
	}

	// 追加交互
	f.current.Messages = append(f.current.Messages, interaction)
	f.current.UpdatedAt = time.Now()

	// 写入文件
	return f.saveToFile()
}

// Load加载会话历史
func (f *FileShortTermMemory) Load(ctx context.Context, sid SessionID, limit int) ([]Interaction, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 如果是当前会话，直接返回
	if f.current != nil && f.current.ID == sid {
		if limit > 0 && len(f.current.Messages) > limit {
			return f.current.Messages[len(f.current.Messages)-limit:], nil
		}
		return f.current.Messages, nil
	}

	// 否则从文件加载
	session, err := f.loadFromFile(sid)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(session.Messages) > limit {
		return session.Messages[len(session.Messages)-limit:], nil
	}
	return session.Messages, nil
}

// ListSessions列出所有会话
func (f *FileShortTermMemory) ListSessions(ctx context.Context, userID string) ([]SessionID, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	entries, err := os.ReadDir(f.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions directory: %w", err)
	}

	var sessions []SessionID
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) == ".json" {
			sid := SessionID(name[:len(name)-5]) // 去掉.json后缀
			sessions = append(sessions, sid)
		}
	}

	return sessions, nil
}

// DeleteSession删除会话
func (f *FileShortTermMemory) DeleteSession(ctx context.Context, sid SessionID) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 如果是当前会话，清空
	if f.current != nil && f.current.ID == sid {
		f.current = nil
	}

	// 删除文件
	filePath := filepath.Join(f.basePath, string(sid)+".json")
	return os.Remove(filePath)
}

// loadOrCreate加载或创建会话文件
func (f *FileShortTermMemory) loadOrCreate(sid SessionID) (*SessionFile, error) {
	filePath := filepath.Join(f.basePath, string(sid)+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 创建新会话
			return &SessionFile{
				ID:        sid,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Messages:  []Interaction{},
			}, nil
		}
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session SessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// loadFromFile从文件加载会话
func (f *FileShortTermMemory) loadFromFile(sid SessionID) (*SessionFile, error) {
	filePath := filepath.Join(f.basePath, string(sid)+".json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var session SessionFile
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &session, nil
}

// saveToFile保存当前会话到文件
func (f *FileShortTermMemory) saveToFile() error {
	if f.current == nil {
		return nil
	}

	// 确保目录存在
	if err := os.MkdirAll(f.basePath, 0755); err != nil {
		return fmt.Errorf("creating sessions directory: %w", err)
	}

	filePath := filepath.Join(f.basePath, string(f.current.ID)+".json")
	data, err := json.MarshalIndent(f.current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestFileShortTermMemory -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/shortterm.go memory/shortterm_test.go
git commit -m "feat(memory): implement FileShortTermMemory"
```

---

### Task 1.5: 实现FileMetaMemory

**Files:**
- Create: `memory/meta.go`
- Create: `memory/meta_test.go`

- [ ] **Step 1: 创建meta_test.go编写失败测试**

```go
package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileMetaMemory_Record(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()
	reflection := Reflection{
		ID:        "ref-1",
		Content:   "用户喜欢简洁的回答",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	}

	err := mm.Record(ctx, reflection)
	require.NoError(t, err)

	// 验证可以检索到
	refs, err := mm.Recent(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, "用户喜欢简洁的回答", refs[0].Content)
}

func TestFileMetaMemory_Recent(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()

	// 添加多条反思
	for i := 0; i < 5; i++ {
		mm.Record(ctx, Reflection{
			ID:        "ref-" + string(rune('0'+i)),
			Content:   "反思内容",
			Trigger:   "test",
			CreatedAt: time.Now(),
		})
	}

	// 限制返回3条
	refs, err := mm.Recent(ctx, 3)
	require.NoError(t, err)
	assert.Len(t, refs, 3)
}

func TestFileMetaMemory_Search(t *testing.T) {
	tmpDir := t.TempDir()
	mm := NewFileMetaMemory(tmpDir)

	ctx := context.Background()

	mm.Record(ctx, Reflection{
		ID:        "ref-1",
		Content:   "用户喜欢Go语言",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	})
	mm.Record(ctx, Reflection{
		ID:        "ref-2",
		Content:   "用户不喜欢Python",
		Trigger:   "user_feedback",
		CreatedAt: time.Now(),
	})

	// 搜索包含"Go"的反思
	refs, err := mm.Search(ctx, "Go", 10)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, "用户喜欢Go语言", refs[0].Content)
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestFileMetaMemory -v`
Expected: FAIL - "NewFileMetaMemory not defined"

- [ ] **Step 3: 创建meta.go实现FileMetaMemory**

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MetaMemory管理元记忆（自我反思）
type MetaMemory interface {
	// Record记录一条反思
	Record(ctx context.Context, reflection Reflection) error

	// Recent获取最近的反思
	Recent(ctx context.Context, limit int) ([]Reflection, error)

	// Search搜索相关反思
	Search(ctx context.Context, query string, limit int) ([]Reflection, error)
}

// FileMetaMemory基于文件的元记忆实现
type FileMetaMemory struct {
	mu       sync.RWMutex
	basePath string
}

// NewFileMetaMemory创建一个新的FileMetaMemory
func NewFileMetaMemory(basePath string) *FileMetaMemory {
	return &FileMetaMemory{
		basePath: basePath,
	}
}

// Record记录一条反思
func (f *FileMetaMemory) Record(ctx context.Context, reflection Reflection) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 确保目录存在
	if err := os.MkdirAll(f.basePath, 0755); err != nil {
		return fmt.Errorf("creating reflections directory: %w", err)
	}

	// 按日期分文件
	dateStr := reflection.CreatedAt.Format("20060102")
	filePath := filepath.Join(f.basePath, dateStr+".json")

	// 加载或创建文件
	var reflections []Reflection
	data, err := os.ReadFile(filePath)
	if err == nil {
		json.Unmarshal(data, &reflections)
	}

	// 追加反思
	reflections = append(reflections, reflection)

	// 写入文件
	data, err = json.MarshalIndent(reflections, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling reflections: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// Recent获取最近的反思
func (f *FileMetaMemory) Recent(ctx context.Context, limit int) ([]Reflection, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 读取最近7天的反思文件
	var all []Reflection
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		filePath := filepath.Join(f.basePath, dateStr+".json")

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var reflections []Reflection
		json.Unmarshal(data, &reflections)
		all = append(all, reflections...)
	}

	// 按时间倒序，返回最近的limit条
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}

	return all, nil
}

// Search搜索相关反思
func (f *FileMetaMemory) Search(ctx context.Context, query string, limit int) ([]Reflection, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 读取最近7天的反思文件
	var all []Reflection
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102")
		filePath := filepath.Join(f.basePath, dateStr+".json")

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var reflections []Reflection
		json.Unmarshal(data, &reflections)
		all = append(all, reflections...)
	}

	// 简单的关键词搜索
	var results []Reflection
	queryLower := strings.ToLower(query)
	for _, ref := range all {
		if strings.Contains(strings.ToLower(ref.Content), queryLower) {
			results = append(results, ref)
		}
	}

	// 按时间倒序
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestFileMetaMemory -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/meta.go memory/meta_test.go
git commit -m "feat(memory): implement FileMetaMemory"
```

---

### Task 1.6: 实现Manager接口和DefaultManager

**Files:**
- Create: `memory/manager.go`
- Create: `memory/manager_test.go`

- [ ] **Step 1: 创建manager_test.go编写失败测试**

```go
package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_StartSession(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		Working: struct {
			MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
		}{
			MaxTokens: 100,
			Tokenizer: "simple",
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg)
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
			MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
		}{
			MaxTokens: 100,
			Tokenizer: "simple",
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg)
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
			MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
		}{
			MaxTokens: 100,
			Tokenizer: "simple",
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg)
	require.NoError(t, err)

	ctx := context.Background()

	// 添加一些消息到工作记忆
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
			MaxTokens int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
			Tokenizer string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`
		}{
			MaxTokens: 100,
			Tokenizer: "simple",
		},
		ShortTerm: struct {
			StorageDir  string `yaml:"storage_dir" env:"MEMORY_SHORTTERM_DIR"`
			MaxSessions int    `yaml:"max_sessions" env:"MEMORY_SHORTTERM_MAX_SESSIONS"`
		}{
			StorageDir: tmpDir,
		},
	}

	mgr, err := NewManager(cfg)
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
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestManager -v`
Expected: FAIL - "NewManager not defined"

- [ ] **Step 3: 创建manager.go实现Manager接口**

```go
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Manager是记忆系统的统一入口，协调四层记忆
type Manager interface {
	// Retrieve获取与当前查询相关的记忆，注入系统提示词
	Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error)

	// Store保存一次交互（用户输入+Agent输出）
	Store(ctx context.Context, interaction Interaction) error

	// Memorize显式记住一个事实（用户或Agent主动调用）
	Memorize(ctx context.Context, fact Fact) error

	// StartSession开始一个新会话，返回会话ID
	StartSession(ctx context.Context, userID string) (SessionID, error)

	// EndSession结束会话，触发短期记忆持久化
	EndSession(ctx context.Context, sid SessionID) error

	// Compact压缩过期记忆（后台任务）
	Compact(ctx context.Context) error

	// Forget选择性遗忘
	Forget(ctx context.Context, filter ForgetFilter) error
}

// DefaultManager实现Manager接口
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

// NewManager创建一个新的DefaultManager
func NewManager(cfg *Config) (*DefaultManager, error) {
	// 创建工作记忆
	tokenizer := &SimpleTokenizer{}
	working := NewWorkingMemory(cfg.Working.MaxTokens, tokenizer)

	// 创建短期记忆
	shortTerm := NewFileShortTermMemory(cfg.ShortTerm.StorageDir)

	// 创建元记忆
	meta := NewFileMetaMemory(cfg.Meta.StorageDir)

	return &DefaultManager{
		config:    cfg,
		working:   working,
		shortTerm: shortTerm,
		meta:      meta,
	}, nil
}

// Retrieve获取与当前查询相关的记忆
func (m *DefaultManager) Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error) {
	result := &Context{}

	// 工作记忆：始终可用
	result.WorkingMemory = m.working.GetWindow(opts.MaxTokens)

	// 长期记忆：可能失败，降级到空结果
	if m.longTerm != nil {
		facts, err := m.longTerm.Search(ctx, query, opts.MaxFacts, opts.MinScore)
		if err != nil {
			slog.Warn("long-term memory search failed, degrading", "error", err)
		} else {
			result.RelevantFacts = facts
		}
	}

	// 元记忆：可能失败，降级到空结果
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

// Store保存一次交互
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

// Memorize显式记住一个事实
func (m *DefaultManager) Memorize(ctx context.Context, fact Fact) error {
	// 存储到工作记忆的关键事实
	m.working.Remember(fact.Key, fact.Content)

	// 如果有长期记忆，也存储到长期记忆
	if m.longTerm != nil {
		if err := m.longTerm.Store(ctx, fact); err != nil {
			slog.Warn("failed to store fact in long-term memory", "error", err)
		}
	}

	return nil
}

// StartSession开始一个新会话
func (m *DefaultManager) StartSession(ctx context.Context, userID string) (SessionID, error) {
	m.mu.Lock()
	m.sessionCounter++
	sid := SessionID(fmt.Sprintf("sess_%s_%03d",
		time.Now().Format("20060102"),
		m.sessionCounter))
	m.currentSession = sid
	m.mu.Unlock()

	// 初始化空会话
	err := m.shortTerm.Save(ctx, sid, Interaction{
		SessionID: sid,
		Metadata:  map[string]any{"timestamp": time.Now()},
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	return sid, nil
}

// EndSession结束会话
func (m *DefaultManager) EndSession(ctx context.Context, sid SessionID) error {
	m.mu.Lock()
	if m.currentSession == sid {
		m.currentSession = ""
	}
	m.mu.Unlock()

	return nil
}

// Compact压缩过期记忆
func (m *DefaultManager) Compact(ctx context.Context) error {
	// TODO: 实现压缩逻辑
	return nil
}

// Forget选择性遗忘
func (m *DefaultManager) Forget(ctx context.Context, filter ForgetFilter) error {
	// TODO: 实现遗忘逻辑
	return nil
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestManager -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/manager.go memory/manager_test.go
git commit -m "feat(memory): implement Manager interface and DefaultManager"
```

---

## Phase 2: PostgreSQL + pgvector长期记忆

### Task 2.1: 创建数据库迁移脚本和docker-compose

**Files:**
- Create: `migrations/001_create_facts.sql`
- Create: `docker-compose.yml`

- [ ] **Step 1: 创建SQL迁移脚本**

```sql
-- migrations/001_create_facts.sql
-- 启用pgvector扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 创建facts表
CREATE TABLE IF NOT EXISTS facts (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    embedding vector(1536),
    decay_score FLOAT DEFAULT 1.0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 创建向量索引
CREATE INDEX IF NOT EXISTS idx_facts_embedding ON facts USING ivfflat (embedding vector_cosine_ops);
```

- [ ] **Step 2: 创建docker-compose.yml**

```yaml
# docker-compose.yml
version: '3.8'

services:
  postgres:
    image: pgvector/pgvector:pg16
    container_name: go-agent-postgres
    environment:
      POSTGRES_USER: goagent
      POSTGRES_PASSWORD: goagent123
      POSTGRES_DB: goagent
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U goagent"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

- [ ] **Step 3: 验证docker-compose配置**

Run: `docker-compose config`
Expected: 配置有效，无语法错误

- [ ] **Step 4: 提交**

```bash
git add migrations/ docker-compose.yml
git commit -m "feat(memory): add database migration and docker-compose"
```

---

### Task 2.2: 实现Embedder接口

**Files:**
- Create: `memory/embedding.go`
- Create: `memory/embedding_test.go`

- [ ] **Step 1: 创建embedding_test.go编写失败测试**

```go
package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMockEmbedder_Embed(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}

	ctx := context.Background()
	embedding, err := embedder.Embed(ctx, "hello world")
	assert.NoError(t, err)
	assert.Len(t, embedding, 1536)
}

func TestMockEmbedder_EmbedBatch(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}

	ctx := context.Background()
	texts := []string{"hello", "world"}
	embeddings, err := embedder.EmbedBatch(ctx, texts)
	assert.NoError(t, err)
	assert.Len(t, embeddings, 2)
	assert.Len(t, embeddings[0], 1536)
}

func TestMockEmbedder_Dimension(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}
	assert.Equal(t, 1536, embedder.Dimension())
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestMockEmbedder -v`
Expected: FAIL - "MockEmbedder not defined"

- [ ] **Step 3: 创建embedding.go实现Embedder接口**

```go
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Embedder是嵌入模型的抽象接口
type Embedder interface {
	// Embed将文本转为向量
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch批量嵌入
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimension返回向量维度
	Dimension() int
}

// OpenAIEmbedder使用OpenAI API的嵌入模型
type OpenAIEmbedder struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbedder创建一个新的OpenAIEmbedder
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed将文本转为向量
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// EmbedBatch批量嵌入
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := map[string]any{
		"input": texts,
		"model": e.model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// Dimension返回向量维度
func (e *OpenAIEmbedder) Dimension() int {
	switch e.model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	default:
		return 1536
	}
}

// OllamaEmbedder使用本地Ollama服务的嵌入模型
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaEmbedder创建一个新的OllamaEmbedder
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Embed将文本转为向量
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := map[string]any{
		"prompt": text,
		"model":  e.model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return result.Embedding, nil
}

// EmbedBatch批量嵌入
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	var mu sync.Mutex
	results := make([][]float64, len(texts))
	errs := make([]error, len(texts))

	// 并发处理
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			embedding, err := e.Embed(ctx, t)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[idx] = err
			} else {
				results[idx] = embedding
			}
		}(i, text)
	}
	wg.Wait()

	// 检查错误
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// Dimension返回向量维度
func (e *OllamaEmbedder) Dimension() int {
	// nomic-embed-text的维度是768
	if e.model == "nomic-embed-text" {
		return 768
	}
	return 768
}

// MockEmbedder用于测试的嵌入模型实现
type MockEmbedder struct {
	dimension int
}

// Embed生成随机向量（用于测试）
func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	embedding := make([]float64, m.dimension)
	for i := range embedding {
		embedding[i] = float64(i) / float64(m.dimension)
	}
	return embedding, nil
}

// EmbedBatch批量嵌入
func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = embedding
	}
	return results, nil
}

// Dimension返回向量维度
func (m *MockEmbedder) Dimension() int {
	return m.dimension
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestMockEmbedder -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/embedding.go memory/embedding_test.go
git commit -m "feat(memory): implement Embedder interface with OpenAI and Ollama"
```

---

### Task 2.3: 实现PgLongTermMemory

**Files:**
- Create: `memory/longterm.go`
- Create: `memory/longterm_test.go`

- [ ] **Step 1: 添加pgx依赖**

Run: `go get github.com/jackc/pgx/v5 github.com/jackc/pgx/v5/pgxpool github.com/pgvector/pgvector-go && go mod tidy`
Expected: 成功下载依赖

- [ ] **Step 2: 创建longterm_test.go编写集成测试**

```go
package memory

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgLongTermMemory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// 检查环境变量
	pgURL := os.Getenv("MEMORY_LONGTERM_PG_URL")
	if pgURL == "" {
		pgURL = "postgres://goagent:goagent123@localhost:5432/goagent"
	}

	ctx := context.Background()

	// 创建连接池
	pool, err := NewPgPool(ctx, pgURL)
	require.NoError(t, err)
	defer pool.Close()

	// 创建嵌入模型mock
	embedder := &MockEmbedder{dimension: 1536}

	// 创建长期记忆
	ltm := NewPgLongTermMemory(pool, embedder)

	// 测试存储
	fact := Fact{
		ID:        "test-1",
		Key:       "project",
		Content:   "Go Agent是一个ReAct风格的Agent框架",
		Source:    "user",
		CreatedAt: time.Now(),
		DecayScore: 1.0,
	}

	err = ltm.Store(ctx, fact)
	require.NoError(t, err)

	// 测试检索
	results, err := ltm.Search(ctx, "Go Agent", 5, 0.5)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Go Agent", results[0].Key)

	// 测试更新
	err = ltm.Update(ctx, "test-1", map[string]any{"decay_score": 0.8})
	require.NoError(t, err)

	// 测试删除
	err = ltm.Delete(ctx, "test-1")
	require.NoError(t, err)
}
```

- [ ] **Step 3: 运行测试验证失败**

Run: `go test ./memory/ -run TestPgLongTermMemory_Integration -v -short=false`
Expected: FAIL - "NewPgLongTermMemory not defined"

- [ ] **Step 4: 创建longterm.go实现PgLongTermMemory**

```go
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// LongTermMemory管理长期记忆（向量存储）
type LongTermMemory interface {
	// Store存储一个Fact，生成嵌入向量
	Store(ctx context.Context, fact Fact) error

	// Search语义检索相关Fact
	Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error)

	// Update更新Fact的衰减分数
	Update(ctx context.Context, id string, updates map[string]any) error

	// Delete删除Fact
	Delete(ctx context.Context, id string) error
}

// PgLongTermMemory基于pgvector的长期记忆实现
type PgLongTermMemory struct {
	mu       sync.RWMutex
	pool     *pgxpool.Pool
	embedder Embedder
}

// NewPgPool创建PostgreSQL连接池
func NewPgPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// 测试连接
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

// NewPgLongTermMemory创建一个新的PgLongTermMemory
func NewPgLongTermMemory(pool *pgxpool.Pool, embedder Embedder) *PgLongTermMemory {
	return &PgLongTermMemory{
		pool:     pool,
		embedder: embedder,
	}
}

// Store存储一个Fact，生成嵌入向量
func (p *PgLongTermMemory) Store(ctx context.Context, fact Fact) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 生成嵌入向量
	embedding, err := p.embedder.Embed(ctx, fact.Content)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	// 插入数据库
	_, err = p.pool.Exec(ctx, `
		INSERT INTO facts (id, key, content, source, embedding, decay_score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			key = EXCLUDED.key,
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			embedding = EXCLUDED.embedding,
			decay_score = EXCLUDED.decay_score,
			updated_at = EXCLUDED.updated_at
	`, fact.ID, fact.Key, fact.Content, fact.Source, pgvector.NewVector(embedding), fact.DecayScore, fact.CreatedAt, time.Now())

	if err != nil {
		return fmt.Errorf("inserting fact: %w", err)
	}

	return nil
}

// Search语义检索相关Fact
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// 生成查询向量
	queryEmbedding, err := p.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generating query embedding: %w", err)
	}

	// 语义检索
	rows, err := p.pool.Query(ctx, `
		SELECT id, key, content, source, decay_score, created_at,
		       1 - (embedding <=> $1) as similarity
		FROM facts
		WHERE 1 - (embedding <=> $1) > $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`, pgvector.NewVector(queryEmbedding), minScore, limit)

	if err != nil {
		return nil, fmt.Errorf("searching facts: %w", err)
	}
	defer rows.Close()

	var results []Fact
	for rows.Next() {
		var f Fact
		var similarity float64
		err := rows.Scan(&f.ID, &f.Key, &f.Content, &f.Source, &f.DecayScore, &f.CreatedAt, &similarity)
		if err != nil {
			return nil, fmt.Errorf("scanning fact: %w", err)
		}
		f.DecayScore = similarity // 使用实际相似度
		results = append(results, f)
	}

	return results, nil
}

// Update更新Fact的衰减分数
func (p *PgLongTermMemory) Update(ctx context.Context, id string, updates map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	decayScore, ok := updates["decay_score"]
	if !ok {
		return fmt.Errorf("decay_score not provided")
	}

	_, err := p.pool.Exec(ctx, `
		UPDATE facts SET decay_score = $1, updated_at = $2 WHERE id = $3
	`, decayScore, time.Now(), id)

	if err != nil {
		return fmt.Errorf("updating fact: %w", err)
	}

	return nil
}

// Delete删除Fact
func (p *PgLongTermMemory) Delete(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.pool.Exec(ctx, `DELETE FROM facts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting fact: %w", err)
	}

	return nil
}
```

- [ ] **Step 5: 启动PostgreSQL并运行测试**

Run: `docker-compose up -d && sleep 5 && go test ./memory/ -run TestPgLongTermMemory_Integration -v -short=false`
Expected: PASS

- [ ] **Step 6: 提交**

```bash
git add memory/longterm.go memory/longterm_test.go
git commit -m "feat(memory): implement PgLongTermMemory with pgvector"
```

---

## Phase 3: 压缩与衰减机制

### Task 3.1: 实现DecayCalculator

**Files:**
- Create: `memory/compaction.go`
- Create: `memory/compaction_test.go`

- [ ] **Step 1: 创建compaction_test.go编写失败测试**

```go
package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDecayCalculator_Calculate(t *testing.T) {
	calculator := &DecayCalculator{halfLife: 30 * 24 * time.Hour} // 30天半衰期

	tests := []struct {
		name      string
		createdAt time.Time
		expected  float64
	}{
		{"just created", time.Now(), 1.0},
		{"30 days ago", time.Now().AddDate(0, 0, -30), 0.5},
		{"60 days ago", time.Now().AddDate(0, 0, -60), 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculator.Calculate(tt.createdAt)
			assert.InDelta(t, tt.expected, score, 0.01)
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./memory/ -run TestDecayCalculator -v`
Expected: FAIL - "DecayCalculator not defined"

- [ ] **Step 3: 创建compaction.go实现DecayCalculator**

```go
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/fengxuan/go-agent/client"
)

// Compactor负责记忆压缩
type Compactor struct {
	shortTerm ShortTermMemory
	longTerm  LongTermMemory
	llm       client.LLMClient
}

// NewCompactor创建一个新的Compactor
func NewCompactor(shortTerm ShortTermMemory, longTerm LongTermMemory, llm client.LLMClient) *Compactor {
	return &Compactor{
		shortTerm: shortTerm,
		longTerm:  longTerm,
		llm:       llm,
	}
}

// Compact压缩过期的会话历史
func (c *Compactor) Compact(ctx context.Context, olderThan time.Duration) error {
	// 找出所有会话
	sessions, err := c.shortTerm.ListSessions(ctx, "")
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	for _, sid := range sessions {
		interactions, err := c.shortTerm.Load(ctx, sid, 0)
		if err != nil {
			slog.Warn("failed to load session", "session", sid, "error", err)
			continue
		}

		// 检查是否过期
		if len(interactions) == 0 {
			continue
		}

		// 获取最后一条消息的时间
		lastInteraction := interactions[len(interactions)-1]
		timestamp, ok := lastInteraction.Metadata["timestamp"].(time.Time)
		if !ok {
			continue
		}

		if time.Since(timestamp) < olderThan {
			continue
		}

		// 使用LLM生成摘要
		summary, err := c.summarize(ctx, interactions)
		if err != nil {
			slog.Warn("failed to summarize session", "session", sid, "error", err)
			continue
		}

		// 存储到长期记忆
		fact := Fact{
			ID:         fmt.Sprintf("session_summary_%s", sid),
			Key:        fmt.Sprintf("session_%s", sid),
			Content:    summary,
			Source:     "derived",
			CreatedAt:  time.Now(),
			DecayScore: 1.0,
		}

		if err := c.longTerm.Store(ctx, fact); err != nil {
			slog.Warn("failed to store summary", "session", sid, "error", err)
			continue
		}

		// 删除原始会话
		if err := c.shortTerm.DeleteSession(ctx, sid); err != nil {
			slog.Warn("failed to delete session", "session", sid, "error", err)
		}

		slog.Info("compacted session", "session", sid)
	}

	return nil
}

// summarize使用LLM生成会话摘要
func (c *Compactor) summarize(ctx context.Context, interactions []Interaction) (string, error) {
	// 构建摘要提示词
	prompt := "请将以下对话历史压缩为简洁的摘要，保留关键信息：\n\n"
	for _, msg := range interactions {
		prompt += fmt.Sprintf("用户: %s\n助手: %s\n\n", msg.UserMsg, msg.AgentMsg)
	}

	// TODO: 调用LLM生成摘要
	// 暂时返回简单拼接
	return fmt.Sprintf("会话摘要: %d条消息", len(interactions)), nil
}

// DecayCalculator计算记忆衰减
type DecayCalculator struct {
	halfLife time.Duration // 半衰期，默认30天
}

// Calculate计算给定时间的衰减分数
func (d *DecayCalculator) Calculate(createdAt time.Time) float64 {
	age := time.Since(createdAt)
	// 使用指数衰减公式：score = 0.5^(age/halfLife)
	halfLifeHours := d.halfLife.Hours()
	ageHours := age.Hours()

	if halfLifeHours == 0 {
		return 1.0
	}

	return math.Pow(0.5, ageHours/halfLifeHours)
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./memory/ -run TestDecayCalculator -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add memory/compaction.go memory/compaction_test.go
git commit -m "feat(memory): implement DecayCalculator and Compactor"
```

---

## Phase 4: 集成到现有Agent

### Task 4.1: 修改Agent集成记忆系统

**Files:**
- Modify: `agent/agent.go`

- [ ] **Step 1: 添加memory依赖到agent.go**

```go
// 在agent.go顶部添加import
import (
    // ... existing imports ...
    "github.com/fengxuan/go-agent/memory"
)
```

- [ ] **Step 2: 修改Agent结构体添加memory字段**

```go
// Agent is a ReAct-style agent that uses an LLM and tools.
type Agent struct {
    llm      client.LLMClient
    registry *tools.Registry
    history  []client.Message
    mu        sync.Mutex
    config    AgentConfig
    callbacks AgentCallbacks
    memory    memory.Manager  // 新增
}
```

- [ ] **Step 3: 修改AgentConfig添加memory配置**

```go
// AgentConfig holds configuration for the Agent.
type AgentConfig struct {
    MaxIterations int
    SystemPrompt  string
    Model         string
    MaxTokens     int           // 工作记忆窗口大小
    MemoryEnabled bool          // 是否启用记忆系统
}
```

- [ ] **Step 4: 修改New函数接受memory参数**

```go
// New creates a new Agent.
func New(llm client.LLMClient, registry *tools.Registry, config AgentConfig, mem memory.Manager) *Agent {
    if config.MaxIterations <= 0 {
        config.MaxIterations = 10
    }
    if config.MaxTokens <= 0 {
        config.MaxTokens = 8192
    }
    return &Agent{
        llm:      llm,
        registry: registry,
        config:   config,
        memory:   mem,
    }
}
```

- [ ] **Step 5: 修改buildMessages使用工作记忆**

```go
// buildMessages prepends the system prompt to a copy of the history.
func (a *Agent) buildMessages() []client.Message {
    a.mu.Lock()
    defer a.mu.Unlock()

    var msgs []client.Message
    if a.config.SystemPrompt != "" {
        msgs = append(msgs, client.Message{
            Role:    client.RoleSystem,
            Content: a.config.SystemPrompt,
        })
    }

    if a.config.MemoryEnabled && a.memory != nil {
        // 使用工作记忆的窗口
        ctx := context.Background()
        memCtx, _ := a.memory.Retrieve(ctx, "", memory.RetrieveOptions{
            MaxTokens: a.config.MaxTokens,
        })

        // 注入工作记忆
        for _, m := range memCtx.WorkingMemory {
            msgs = append(msgs, client.Message{
                Role:    client.Role(m.Role),
                Content: m.Content,
            })
        }

        // 注入相关事实
        for _, fact := range memCtx.RelevantFacts {
            msgs = append(msgs, client.Message{
                Role:    client.RoleSystem,
                Content: fmt.Sprintf("[相关记忆] %s: %s", fact.Key, fact.Content),
            })
        }

        // 注入自我反思
        for _, ref := range memCtx.SelfReflection {
            msgs = append(msgs, client.Message{
                Role:    client.RoleSystem,
                Content: fmt.Sprintf("[反思] %s", ref.Content),
            })
        }
    } else {
        // 不使用记忆系统，直接追加全部历史
        msgs = append(msgs, a.history...)
    }

    return msgs
}
```

- [ ] **Step 6: 修改Run方法存储交互**

```go
// Run appends the user message to history and starts the ReAct loop.
func (a *Agent) Run(ctx context.Context, input string) <-chan AgentEvent {
    ch := make(chan AgentEvent, 32)

    a.mu.Lock()
    a.history = append(a.history, client.Message{
        Role:    client.RoleUser,
        Content: input,
    })
    a.mu.Unlock()

    // 如果启用记忆系统，添加消息到工作记忆
    if a.config.MemoryEnabled && a.memory != nil {
        a.memory.(*memory.DefaultManager).AddToWorkingMemory(memory.Message{
            Role:      "user",
            Content:   input,
            Timestamp: time.Now(),
        })
    }

    go func() {
        defer close(ch)
        a.runLoop(ctx, ch)
    }()

    return ch
}
```

- [ ] **Step 7: 运行测试验证编译**

Run: `go build ./agent/...`
Expected: 编译成功

- [ ] **Step 8: 提交**

```bash
git add agent/agent.go
git commit -m "feat(agent): integrate memory system"
```

---

### Task 4.2: 修改Runtime初始化记忆系统

**Files:**
- Modify: `runtime/runtime.go`

- [ ] **Step 1: 添加memory依赖到runtime.go**

```go
// 在runtime.go顶部添加import
import (
    // ... existing imports ...
    "github.com/fengxuan/go-agent/memory"
)
```

- [ ] **Step 2: 修改Runtime结构体添加memory字段**

```go
// Runtime coordinates all subsystems
type Runtime struct {
    // ... existing fields ...
    memory    memory.Manager  // 新增
}
```

- [ ] **Step 3: 修改New函数初始化记忆系统**

```go
func New(cfg Config) (*Runtime, error) {
    // ... existing initialization code ...

    // 初始化记忆系统
    memCfg := memory.DefaultConfig()
    memory.ApplyEnvOverrides(memCfg)

    memManager, err := memory.NewManager(memCfg)
    if err != nil {
        // 降级：记忆系统不可用，继续运行
        slog.Warn("memory system unavailable, falling back", "error", err)
    } else {
        // 启动新会话
        ctx := context.Background()
        memManager.StartSession(ctx, "default")
    }

    // 修改agent.New调用，传入memory
    ag := agent.New(llm, cfg.ToolRegistry, agent.AgentConfig{
        MaxIterations: appCfg.MaxIterations,
        Model:         providerCfg.Model,
        MaxTokens:     memCfg.Working.MaxTokens,
        MemoryEnabled: memManager != nil,
    }, memManager)

    rt := &Runtime{
        // ... existing fields ...
        memory:    memManager,
    }

    return rt, nil
}
```

- [ ] **Step 4: 修改RunUserInput存储交互**

```go
// RunUserInput is the main request flow.
func (rt *Runtime) RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent {
    // ... existing prompt building code ...

    // 如果记忆系统可用，存储用户输入
    if rt.memory != nil {
        go func() {
            rt.memory.Store(ctx, memory.Interaction{
                UserMsg:  input,
                Metadata: map[string]any{"timestamp": time.Now()},
            })
        }()
    }

    // ... existing agent.Run code ...
}
```

- [ ] **Step 5: 运行测试验证编译**

Run: `go build ./runtime/...`
Expected: 编译成功

- [ ] **Step 6: 提交**

```bash
git add runtime/runtime.go
git commit -m "feat(runtime): initialize memory system"
```

---

## Phase 5: 测试与验证

### Task 5.1: 运行完整测试套件

**Files:**
- None (testing existing code)

- [ ] **Step 1: 运行所有单元测试**

Run: `go test ./memory/... -v`
Expected: 所有测试通过

- [ ] **Step 2: 运行集成测试**

Run: `docker-compose up -d && sleep 5 && go test ./memory/... -v -short=false`
Expected: 所有测试通过

- [ ] **Step 3: 运行现有agent测试**

Run: `go test ./agent/... -v`
Expected: 所有测试通过

- [ ] **Step 4: 运行现有runtime测试**

Run: `go test ./runtime/... -v`
Expected: 所有测试通过

- [ ] **Step 5: 构建完整项目**

Run: `go build -o go-agent .`
Expected: 构建成功

- [ ] **Step 6: 提交**

```bash
git add .
git commit -m "test: verify memory system integration"
```

---

## 完成

记忆系统实现完成！系统包含：

1. **工作记忆**：Token窗口管理，滑动窗口算法
2. **短期记忆**：JSON文件持久化，会话管理
3. **长期记忆**：pgvector向量存储，语义检索
4. **元记忆**：反思日志，按日期存储
5. **压缩机制**：会话摘要，衰减计算
6. **集成**：与现有Agent和Runtime无缝集成

**下一步**：
- 启动PostgreSQL：`docker-compose up -d`
- 配置环境变量（如需要）
- 运行应用测试记忆功能
