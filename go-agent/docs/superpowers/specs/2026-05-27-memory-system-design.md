# Go Agent 记忆系统设计文档

## 1. 概述

### 1.1 背景

当前Go Agent项目是一个基于Bubbletea TUI的ReAct风格Agent框架，支持多Provider（OpenAI/Anthropic/DeepSeek）。现有实现中，Agent仅在内存中维护基本的对话历史（`agent.history []client.Message`），存在以下问题：

- **无Token管理**：history无限增长，长对话会超过模型上下文窗口
- **无持久化**：进程退出，所有对话历史丢失
- **无语义检索**：无法从历史对话中找到相关信息
- **无会话隔离**：所有对话混在一个history切片里

### 1.2 目标

实现一个分层记忆系统，包含四个层级：

1. **工作记忆（Working Memory）**：基于Token窗口的滑动管理
2. **短期记忆（Short-term Memory）**：会话历史持久化
3. **长期记忆（Long-term Memory）**：向量存储+语义检索
4. **元记忆（Meta-memory）**：自我反思机制

### 1.3 设计原则

- **接口隔离**：每层有自己的接口，通过Manager统一协调
- **依赖倒置**：所有存储通过接口抽象，方便切换实现
- **并发安全**：所有操作使用sync.RWMutex保护
- **可测试性**：每层可独立mock测试
- **渐进式实现**：每层独立实现、独立测试，然后集成

## 2. 架构设计

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Agent (ReAct Loop)                   │
│                           ↓                                 │
│                    ┌──────┴──────┐                          │
│                    │   Manager   │  ← 统一入口接口          │
│                    └──────┬──────┘                          │
│                           │                                 │
│       ┌───────────────────┼───────────────────┐            │
│       │                   │                   │            │
│  ┌────┴────┐        ┌─────┴─────┐       ┌────┴────┐       │
│  │ Working │        │ ShortTerm │       │  Long   │       │
│  │ Memory  │        │  Memory   │       │  Term   │       │
│  └────┬────┘        └─────┬─────┘       └────┬────┘       │
│       │                   │                   │            │
│  Token窗口管理       文件持久化          pgvector存储       │
│  滑动上下文          会话历史            语义检索           │
│                                                           │
│                    ┌────────────┐                          │
│                    │   Meta     │                          │
│                    │  Memory    │                          │
│                    └────────────┘                          │
│                    自我反思日志                              │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 文件结构

```
memory/
├── manager.go          # Manager接口 + defaultManager实现
├── config.go           # Config结构体，支持环境变量
├── working.go          # WorkingMemory接口 + 实现
├── shortterm.go        # ShortTermMemory接口 + 文件实现
├── longterm.go         # LongTermMemory接口 + pgvector实现
├── embedding.go        # Embedder接口 + OpenAI/Ollama实现
├── meta.go             # MetaMemory接口 + 实现
├── compaction.go       # 压缩逻辑
├── storage/
│   ├── pgvector.go     # pgvector存储实现
│   └── memory.go       # 内存存储（用于测试）
└── memory_test.go      # 集成测试
```

## 3. 核心接口设计

### 3.1 Manager接口

```go
package memory

// Manager是记忆系统的统一入口，协调四层记忆。
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
```

### 3.2 数据结构

```go
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
    ID        string
    Key       string    // 事实的关键词/主题
    Content   string    // 事实内容
    Source    string    // 来源："user" | "agent" | "derived"
    CreatedAt time.Time
    DecayScore float64  // 衰减分数，随时间降低
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
    SessionID *SessionID // 按会话遗忘
    Before    *time.Time // 遗忘某个时间之前的
    KeyPattern *string   // 按key模式匹配遗忘
}

// SessionID是会话标识符
type SessionID string
```

### 3.3 四层子接口

```go
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

// MetaMemory管理元记忆（自我反思）
type MetaMemory interface {
    // Record记录一条反思
    Record(ctx context.Context, reflection Reflection) error
    
    // Recent获取最近的反思
    Recent(ctx context.Context, limit int) ([]Reflection, error)
    
    // Search搜索相关反思
    Search(ctx context.Context, query string, limit int) ([]Reflection, error)
}
```

### 3.4 嵌入模型接口

```go
// Embedder是嵌入模型的抽象接口
type Embedder interface {
    // Embed将文本转为向量
    Embed(ctx context.Context, text string) ([]float64, error)
    
    // EmbedBatch批量嵌入
    EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)
    
    // Dimension返回向量维度
    Dimension() int
}
```

## 4. 工作记忆实现

### 4.1 核心职责

工作记忆解决的问题：**在有限Token窗口内保留最重要的上下文**

### 4.2 实现策略

```go
// WorkingMemoryImpl实现WorkingMemory接口
type WorkingMemoryImpl struct {
    mu           sync.RWMutex
    messages     []Message           // 所有消息
    keyFacts     map[string]string   // 用户显式Remember的关键信息
    tokenizer    Tokenizer           // Token计数器
    maxTokens    int                 // 窗口大小限制
}

// Tokenizer是Token计数的抽象
type Tokenizer interface {
    Count(text string) int
}
```

### 4.3 滑动窗口算法

```
假设 maxTokens = 1000

消息队列（按时间顺序）：
[msg1: 100t] [msg2: 200t] [msg3: 150t] [msg4: 300t] [msg5: 250t]
                                                          ↑
                                                     最新消息

窗口选择逻辑：
1. 从最新消息向前累加Token
2. 直到达到maxTokens限制
3. 保留：[msg3: 150t] [msg4: 300t] [msg5: 250t] = 700t
4. 丢弃：[msg1: 100t] [msg2: 200t]

特殊处理：
- keyFacts始终保留，不参与窗口计算
- 系统消息（如工具调用结果）优先级低于用户/助手消息
```

### 4.4 Token计数方案

由于学习场景，我们不引入外部Tokenizer库，使用简单估算：

```go
// SimpleTokenizer基于字符数估算Token
// 粗略估算：1个中文字符≈2 tokens，1个英文单词≈1.3 tokens
type SimpleTokenizer struct{}

func (t *SimpleTokenizer) Count(text string) int {
    // 简单实现：按空格分词 + 中文字符计数
    // 后续可替换为tiktoken-go等精确实现
}
```

### 4.5 关键方法

```go
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
        
        if tokenCount + msgTokens > maxTokens {
            break  // 超出Token限制，停止
        }
        
        result = append([]Message{msg}, result...)  // 插入到头部
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
```

### 4.6 与现有代码的集成点

```go
// 现有代码（agent/agent.go）
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
    msgs = append(msgs, a.history...)  // ← 当前：无限制追加
    return msgs
}

// 改造后
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
    
    // 使用工作记忆的窗口，而不是全部历史
    window := a.workingMem.GetWindow(a.config.MaxTokens)
    for _, m := range window {
        msgs = append(msgs, client.Message{
            Role:    client.Role(m.Role),
            Content: m.Content,
        })
    }
    return msgs
}
```

## 5. 短期记忆实现

### 5.1 核心职责

短期记忆解决的问题：**会话历史持久化，支持跨会话恢复**

### 5.2 存储方案

学习场景下，用JSON文件持久化，无需数据库：

```
~/.go-agent/memory/
├── sessions/
│   ├── sess_20260527_001.json
│   ├── sess_20260527_002.json
│   └── ...
└── index.json          # 会话索引
```

### 5.3 数据结构

```go
// SessionFile是单个会话文件的结构
type SessionFile struct {
    ID        SessionID     `json:"id"`
    UserID    string        `json:"user_id"`
    CreatedAt time.Time     `json:"created_at"`
    UpdatedAt time.Time     `json:"updated_at"`
    Messages  []Interaction `json:"messages"`
}

// SessionIndex是会话索引
type SessionIndex struct {
    Sessions []SessionEntry `json:"sessions"`
}

type SessionEntry struct {
    ID        SessionID `json:"id"`
    UserID    string    `json:"user_id"`
    CreatedAt time.Time `json:"created_at"`
    MsgCount  int       `json:"msg_count"`
    Preview   string    `json:"preview"`  // 最后一条消息的前50字符
}
```

### 5.4 实现

```go
// FileShortTermMemory基于文件的短期记忆实现
type FileShortTermMemory struct {
    mu       sync.RWMutex
    basePath string      // ~/.go-agent/memory/sessions/
    current  *SessionFile // 当前活跃会话
}

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

// ListSessions列出用户的所有会话
func (f *FileShortTermMemory) ListSessions(ctx context.Context, userID string) ([]SessionID, error) {
    f.mu.RLock()
    defer f.mu.RUnlock()
    
    index, err := f.loadIndex()
    if err != nil {
        return nil, err
    }
    
    var result []SessionID
    for _, entry := range index.Sessions {
        if entry.UserID == userID {
            result = append(result, entry.ID)
        }
    }
    return result, nil
}
```

### 5.5 与Manager的集成

```go
// Manager.StartSession创建新会话
func (m *DefaultManager) StartSession(ctx context.Context, userID string) (SessionID, error) {
    // 生成会话ID: sess_20260527_001
    sid := SessionID(fmt.Sprintf("sess_%s_%03d", 
        time.Now().Format("20060102"), 
        m.nextSessionNum()))
    
    // 初始化空会话文件
    err := m.shortTerm.Save(ctx, sid, Interaction{})  // 空初始化
    if err != nil {
        return "", err
    }
    
    m.mu.Lock()
    m.currentSession = sid
    m.mu.Unlock()
    
    return sid, nil
}

// Manager.Store保存交互到短期记忆
func (m *DefaultManager) Store(ctx context.Context, interaction Interaction) error {
    m.mu.RLock()
    sid := m.currentSession
    m.mu.RUnlock()
    
    if sid == "" {
        return ErrNoActiveSession
    }
    
    // 保存到短期记忆
    return m.shortTerm.Save(ctx, sid, interaction)
}
```

### 5.6 会话恢复流程

```
用户启动Agent
    ↓
检查 ~/.go-agent/memory/sessions/ 是否有历史会话
    ↓
[有] → 显示最近5个会话，让用户选择
       → 加载选中的会话到工作记忆
    ↓
[无] → 调用 StartSession() 创建新会话
```

## 6. 长期记忆实现

### 6.1 核心职责

长期记忆解决的问题：**从历史对话中语义检索相关信息**

### 6.2 存储方案

使用PostgreSQL + pgvector扩展：

```sql
-- 表结构
CREATE TABLE facts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    embedding vector(1536),  -- OpenAI text-embedding-3-small维度
    decay_score FLOAT DEFAULT 1.0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 向量索引
CREATE INDEX idx_facts_embedding ON facts USING ivfflat (embedding vector_cosine_ops);
```

### 6.3 pgvector实现

```go
// PgLongTermMemory基于pgvector的长期记忆实现
type PgLongTermMemory struct {
    mu       sync.RWMutex
    pool     *pgxpool.Pool
    embedder Embedder
}

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
        INSERT INTO facts (id, key, content, source, embedding, decay_score, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `, fact.ID, fact.Key, fact.Content, fact.Source, embedding, fact.DecayScore, fact.CreatedAt)
    
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
    `, queryEmbedding, minScore, limit)
    
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
        f.DecayScore = similarity  // 使用实际相似度
        results = append(results, f)
    }
    
    return results, nil
}
```

### 6.4 嵌入模型实现

```go
// OpenAIEmbedder使用OpenAI API的嵌入模型
type OpenAIEmbedder struct {
    apiKey string
    model  string
    client *http.Client
}

func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
    return &OpenAIEmbedder{
        apiKey: apiKey,
        model:  model,
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
    // 调用OpenAI Embeddings API
    // POST https://api.openai.com/v1/embeddings
    // {"input": text, "model": e.model}
}

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

func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
    return &OllamaEmbedder{
        baseURL: baseURL,
        model:   model,
        client:  &http.Client{Timeout: 60 * time.Second},
    }
}

func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
    // 调用Ollama Embeddings API
    // POST http://localhost:11434/api/embeddings
    // {"prompt": text, "model": e.model}
}
```

## 7. 元记忆实现

### 7.1 核心职责

元记忆解决的问题：**让Agent知道自己知道什么/不知道什么**

### 7.2 存储方案

使用JSON文件存储反思日志：

```
~/.go-agent/memory/
├── reflections/
│   ├── 20260527.json
│   ├── 20260528.json
│   └── ...
```

### 7.3 实现

```go
// FileMetaMemory基于文件的元记忆实现
type FileMetaMemory struct {
    mu       sync.RWMutex
    basePath string      // ~/.go-agent/memory/reflections/
}

func NewFileMetaMemory(basePath string) *FileMetaMemory {
    return &FileMetaMemory{
        basePath: basePath,
    }
}

// Record记录一条反思
func (f *FileMetaMemory) Record(ctx context.Context, reflection Reflection) error {
    f.mu.Lock()
    defer f.mu.Unlock()
    
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
        return err
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
```

### 7.4 反思触发机制

```go
// 在Agent执行过程中，当遇到以下情况时触发反思：
// 1. 工具调用失败
// 2. 用户纠正Agent的回答
// 3. Agent发现自己之前的回答有误
// 4. 任务完成后的总结

func (m *DefaultManager) triggerReflection(ctx context.Context, trigger string, content string) {
    reflection := Reflection{
        ID:        generateID(),
        Content:   content,
        Trigger:   trigger,
        CreatedAt: time.Now(),
    }
    
    m.meta.Record(ctx, reflection)
}
```

## 8. 压缩与衰减机制

### 8.1 记忆压缩

当短期记忆积累过多时，自动压缩为长期记忆摘要：

```go
// Compactor负责记忆压缩
type Compactor struct {
    shortTerm ShortTermMemory
    longTerm  LongTermMemory
    llm       client.LLMClient
}

// Compact压缩过期的会话历史
func (c *Compactor) Compact(ctx context.Context, olderThan time.Duration) error {
    // 1. 找出过期的会话
    sessions, err := c.shortTerm.ListSessions(ctx, "")
    if err != nil {
        return err
    }
    
    for _, sid := range sessions {
        interactions, err := c.shortTerm.Load(ctx, sid, 0)
        if err != nil {
            continue
        }
        
        // 2. 检查是否过期
        if len(interactions) == 0 {
            continue
        }
        lastMsg := interactions[len(interactions)-1]
        if time.Since(lastMsg.Metadata["timestamp"].(time.Time)) < olderThan {
            continue
        }
        
        // 3. 使用LLM生成摘要
        summary, err := c.summarize(ctx, interactions)
        if err != nil {
            continue
        }
        
        // 4. 存储到长期记忆
        fact := Fact{
            ID:        generateID(),
            Key:       fmt.Sprintf("session_summary_%s", sid),
            Content:   summary,
            Source:    "derived",
            CreatedAt: time.Now(),
            DecayScore: 1.0,
        }
        
        c.longTerm.Store(ctx, fact)
        
        // 5. 删除原始会话
        c.shortTerm.DeleteSession(ctx, sid)
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
    
    // 调用LLM生成摘要
    // ...
}
```

### 8.2 记忆衰减

长期记忆条目随时间降低相关性分数，模拟遗忘曲线：

```go
// DecayCalculator计算记忆衰减
type DecayCalculator struct {
    halfLife time.Duration  // 半衰期，默认30天
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

// UpdateDecayScores批量更新衰减分数
func (p *PgLongTermMemory) UpdateDecayScores(ctx context.Context) error {
    calculator := &DecayCalculator{halfLife: 30 * 24 * time.Hour}
    
    rows, err := p.pool.Query(ctx, `SELECT id, created_at FROM facts`)
    if err != nil {
        return err
    }
    defer rows.Close()
    
    for rows.Next() {
        var id string
        var createdAt time.Time
        rows.Scan(&id, &createdAt)
        
        newScore := calculator.Calculate(createdAt)
        p.pool.Exec(ctx, `UPDATE facts SET decay_score = $1 WHERE id = $2`, newScore, id)
    }
    
    return nil
}
```

## 9. 配置设计

### 9.1 配置结构

```go
// Config是记忆系统的配置
type Config struct {
    // 工作记忆配置
    Working struct {
        MaxTokens   int    `yaml:"max_tokens" env:"MEMORY_WORKING_MAX_TOKENS"`
        Tokenizer   string `yaml:"tokenizer" env:"MEMORY_WORKING_TOKENIZER"`   // "simple" | "tiktoken"
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
            Provider string `yaml:"provider" env:"MEMORY_EMBEDDER_PROVIDER"`   // "openai" | "ollama"
            APIKey   string `yaml:"api_key" env:"MEMORY_EMBEDDER_API_KEY"`
            Model    string `yaml:"model" env:"MEMORY_EMBEDDER_MODEL"`
            BaseURL  string `yaml:"base_url" env:"MEMORY_EMBEDDER_BASE_URL"`   // for ollama
        } `yaml:"embedder"`
        HalfLife    time.Duration `yaml:"half_life" env:"MEMORY_LONGTERM_HALF_LIFE"`
    } `yaml:"long_term"`
    
    // 元记忆配置
    Meta struct {
        StorageDir string `yaml:"storage_dir" env:"MEMORY_META_DIR"`
    } `yaml:"meta"`
    
    // 压缩配置
    Compaction struct {
        Interval   time.Duration `yaml:"interval" env:"MEMORY_COMPACTION_INTERVAL"`
        OlderThan  time.Duration `yaml:"older_than" env:"MEMORY_COMPACTION_OLDER_THAN"`
    } `yaml:"compaction"`
}
```

### 9.2 默认配置

```go
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
    cfg.LongTerm.HalfLife = 30 * 24 * time.Hour  // 30天
    
    // 元记忆默认值
    cfg.Meta.StorageDir = filepath.Join(home, ".go-agent", "memory", "reflections")
    
    // 压缩默认值
    cfg.Compaction.Interval = 24 * time.Hour  // 每天压缩一次
    cfg.Compaction.OlderThan = 7 * 24 * time.Hour  // 7天前的会话
    
    return cfg
}
```

## 10. 集成设计

### 10.1 与现有Agent的集成

```go
// 在agent/agent.go中添加记忆管理器
type Agent struct {
    llm       client.LLMClient
    registry  *tools.Registry
    history   []client.Message
    mu        sync.Mutex
    config    AgentConfig
    callbacks AgentCallbacks
    memory    memory.Manager  // 新增
}

// AgentConfig添加记忆相关配置
type AgentConfig struct {
    MaxIterations int
    SystemPrompt  string
    Model         string
    MaxTokens     int           // 工作记忆窗口大小
    MemoryEnabled bool          // 是否启用记忆系统
}

// buildMessages改造
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

### 10.2 与Runtime的集成

```go
// 在runtime/runtime.go中初始化记忆系统
type Runtime struct {
    // ... 现有字段 ...
    memory    memory.Manager  // 新增
}

func New(cfg Config) (*Runtime, error) {
    // ... 现有初始化代码 ...
    
    // 初始化记忆系统
    memCfg := memory.DefaultConfig()
    // 从环境变量加载配置
    memory.ApplyEnvOverrides(memCfg)
    
    memManager, err := memory.NewManager(memCfg)
    if err != nil {
        // 降级：记忆系统不可用，继续运行
        slog.Warn("memory system unavailable, falling back", "error", err)
    }
    
    rt := &Runtime{
        // ... 现有字段 ...
        memory: memManager,
    }
    
    return rt, nil
}

// RunUserInput改造
func (rt *Runtime) RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent {
    // ... 现有prompt构建代码 ...
    
    // 如果记忆系统可用，存储交互
    if rt.memory != nil {
        // 异步存储，不阻塞主流程
        go func() {
            rt.memory.Store(ctx, memory.Interaction{
                SessionID: rt.currentSession,
                UserMsg:   input,
                // AgentMsg会在收到结果后填充
            })
        }()
    }
    
    // ... 现有agent.Run代码 ...
}
```

## 11. 错误处理与降级策略

### 11.1 哨兵错误

```go
package memory

import "errors"

var (
    ErrNoActiveSession    = errors.New("no active session")
    ErrSessionNotFound    = errors.New("session not found")
    ErrEmbeddingFailed    = errors.New("embedding generation failed")
    ErrDatabaseUnavailable = errors.New("database unavailable")
    ErrCompactionFailed   = errors.New("compaction failed")
)
```

### 11.2 降级策略

```go
// Manager的降级逻辑
func (m *DefaultManager) Retrieve(ctx context.Context, query string, opts RetrieveOptions) (*Context, error) {
    result := &Context{}
    
    // 工作记忆：始终可用
    result.WorkingMemory = m.working.GetWindow(opts.MaxTokens)
    
    // 长期记忆：可能失败，降级到空结果
    if m.longTerm != nil {
        facts, err := m.longTerm.Search(ctx, query, opts.MaxFacts, opts.MinScore)
        if err != nil {
            slog.Warn("long-term memory search failed, degrading", "error", err)
            // 降级：返回空结果，不报错
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
```

## 12. 测试策略

### 12.1 单元测试

每层独立测试，使用mock或内存实现：

```go
// working_test.go
func TestWorkingMemory_SlidingWindow(t *testing.T) {
    wm := NewWorkingMemory(100)  // 100 tokens限制
    
    // 添加多条消息
    wm.Add(Message{Role: "user", Content: "消息1"})
    wm.Add(Message{Role: "assistant", Content: "回复1"})
    wm.Add(Message{Role: "user", Content: "消息2"})
    
    // 验证窗口大小
    window := wm.GetWindow(100)
    assert.LessOrEqual(t, len(window), 3)
}

func TestWorkingMemory_Remember(t *testing.T) {
    wm := NewWorkingMemory(100)
    
    // Remember关键信息
    wm.Remember("项目", "Go Agent")
    
    // 验证始终保留
    window := wm.GetWindow(0)  // 0表示使用默认限制
    found := false
    for _, msg := range window {
        if strings.Contains(msg.Content, "Go Agent") {
            found = true
            break
        }
    }
    assert.True(t, found)
}
```

### 12.2 集成测试

使用testcontainers进行PostgreSQL测试：

```go
// longterm_test.go
func TestPgLongTermMemory_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }
    
    // 启动PostgreSQL容器
    ctx := context.Background()
    pool := startTestPostgres(ctx, t)
    defer pool.Close()
    
    // 创建嵌入模型mock
    embedder := &MockEmbedder{dimension: 1536}
    
    // 创建长期记忆
    ltm := NewPgLongTermMemory(pool, embedder)
    
    // 测试存储和检索
    fact := Fact{
        ID:      "test-1",
        Key:     "项目",
        Content: "Go Agent是一个ReAct风格的Agent框架",
        Source:  "user",
    }
    
    err := ltm.Store(ctx, fact)
    require.NoError(t, err)
    
    results, err := ltm.Search(ctx, "Go Agent", 5, 0.5)
    require.NoError(t, err)
    assert.Len(t, results, 1)
    assert.Equal(t, "Go Agent", results[0].Key)
}
```

## 13. 实现阶段

### Phase 1：接口定义 + 内存实现

- 定义所有接口（Manager, WorkingMemory, ShortTermMemory, LongTermMemory, MetaMemory）
- 实现WorkingMemory（内存，Token窗口管理）
- 实现FileShortTermMemory（文件持久化）
- 实现FileMetaMemory（文件持久化）
- 编写单元测试

### Phase 2：PostgreSQL + pgvector长期记忆

- 实现PgLongTermMemory
- 编写数据库迁移脚本
- 使用testcontainers编写集成测试

### Phase 3：嵌入模型抽象

- 实现Embedder接口
- 实现OpenAIEmbedder
- 实现OllamaEmbedder
- 编写测试

### Phase 4：压缩与衰减

- 实现Compactor
- 实现DecayCalculator
- 集成到Manager.Compact()

### Phase 5：集成示例

- 修改agent/agent.go集成记忆系统
- 修改runtime/runtime.go初始化记忆系统
- 提供配置示例

### Phase 6：并发压测与边界处理

- 并发安全测试
- Token超限处理
- DB断连降级测试

## 14. 依赖

### 必需依赖

- `github.com/jackc/pgx/v5` - PostgreSQL驱动
- `github.com/pgvector/pgvector-go` - pgvector Go客户端

### 可选依赖

- `github.com/tiktoken-go/tokenizer` - 精确Token计数（Phase 1使用SimpleTokenizer）

### 开发依赖

- `github.com/testcontainers/testcontainers-go` - 集成测试
- `github.com/stretchr/testify` - 断言库

## 15. 设计决策记录

1. **Token计数精度**：Phase 1使用SimpleTokenizer（字符估算），后续可替换为tiktoken-go
2. **向量维度**：默认1536（OpenAI text-embedding-3-small），通过Config.Embedder.Dimension配置
3. **索引策略**：默认使用IVFFlat（简单、适合学习），大数据量可切换HNSW
4. **批量操作**：EmbedBatch使用goroutine并发，通过semaphore控制并发数（默认10）
5. **缓存策略**：Phase 1不缓存Embedding，后续可通过LRU缓存优化
