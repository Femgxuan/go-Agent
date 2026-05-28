# Hermes 四层记忆系统设计

> 参照 NousResearch Hermes Agent 架构，将现有记忆系统重构为四层记忆 + PromptBuilder + Frozen Snapshot 体系。

## 1. 背景与动机

### 当前系统

现有记忆系统包含四层：
- **Working Memory** — 进程内滑动窗口（messages + keyFacts map），无上下文压缩
- **Short-Term Memory** — 文件系统 JSON 会话归档，无检索能力
- **Long-Term Memory** — PostgreSQL + pgvector 事实存储，FTS 使用 ILIKE 而非 tsvector
- **Meta Memory** — 文件系统自省记录，`Record()` 从未被调用

### 核心问题

1. 记忆作为 system message 注入 `buildMessages()`，破坏 prompt cache
2. 无上下文压缩，旧消息直接丢弃
3. 短期记忆无全文检索能力
4. 无 Frozen Snapshot 机制，系统提示不冻结
5. 无 Prompt Cache 感知设计
6. 无安全扫描

### 目标

重构为 Hermes 风格四层架构：工作记忆 + 情景记忆 + 语义记忆 + 技能记忆，配合 PromptBuilder、Frozen Snapshot、上下文压缩、Cache 感知。

## 2. 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     Agent 主循环                             │
│                                                             │
│  用户输入 → WorkingMemory.Add()                             │
│       ↓                                                     │
│  buildMessages() → PromptBuilder.Build() [冻结]             │
│       ↓              ↓                                      │
│       │    ┌─────────┴──────────┐                           │
│       │    │  Frozen Snapshot   │  MEMORY.md + USER.md      │
│       │    │  Skills Index      │  Level-0 技能列表          │
│       │    │  Persona/SOUL      │  身份定义                  │
│       │    │  Tool Schemas      │  工具 JSON schema          │
│       │    └────────────────────┘                           │
│       ↓                                                     │
│  ContextCompressor.Check() → 触发压缩? → LLM 摘要          │
│       ↓                                                     │
│  LLM 调用 ← system prompt [cache breakpoint]               │
│       ↓         + messages [3 cache breakpoints]            │
│  工具调用 → 工具结果写入 WorkingMemory                      │
│       ↓                                                     │
│  storeInteraction()                                         │
│       ├── WorkingMemory.Add(assistant)                      │
│       ├── EpisodicStore.Save() [异步]                       │
│       ├── Classifier → SemanticStore.Remember() [异步]      │
│       └── SkillStore 检测 [异步]                            │
└─────────────────────────────────────────────────────────────┘
```

### 四层记忆职责

| 层 | 存储介质 | 写入时机 | 读取时机 | 注入位置 |
|---|---------|---------|---------|---------|
| **L1 工作记忆** | 进程内存 | 每条消息 | 每次 LLM 调用 | 消息列表（非系统提示） |
| **L2 情景记忆** | PG + FTS | 每轮结束（异步） | Agent 调用 `session_search` 工具 | **工具结果** |
| **L3 语义记忆** | PG + FTS | 分类器判定后（异步） | `Retrieve()` 时 | **用户消息/工具结果** |
| **L4 技能记忆** | 文件系统 | Agent 创建技能 | 启动时扫描 + 按需读取 | 系统提示（仅 Level-0 索引） |

### 关键约束

1. **系统提示组装后字节级冻结** — 会话内绝不修改
2. **记忆绝不写入系统提示** — 只通过消息/工具结果注入
3. **Prompt Cache 感知** — 系统提示 = cache breakpoint #1，消息窗口 = breakpoints #2-4
4. **安全扫描** — MEMORY.md/USER.md 注入前检测 prompt injection

## 3. L1 工作记忆 + 上下文压缩

### 3.1 接口定义

```go
// memory/working.go

type WorkingMemory interface {
    Add(msg Message) error
    GetWindow(budget int) []Message
    Remember(key, value string)
    TokenCount() int
    NeedsCompression(threshold float64) bool
    Compress(ctx context.Context, summarizer Summarizer) error
    GetCompressedSummary() string
}
```

### 3.2 Token 计数器

使用 `github.com/pkoukk/tiktoken-go` 替换现有 `SimpleTokenizer`：

```go
// memory/tokenizer.go

type TiktokenTokenizer struct {
    encoding *tiktoken.Tiktoken
    model    string
}

func NewTiktokenTokenizer(model string) (*TiktokenTokenizer, error)
func (t *TiktokenTokenizer) Count(text string) int
```

### 3.3 ContextCompressor

```go
// memory/compression.go

type Summarizer interface {
    Summarize(ctx context.Context, messages []Message) (string, error)
}

type ContextCompressor struct {
    working   WorkingMemory
    summarizer Summarizer
    tokenizer  *TiktokenTokenizer
    maxTokens  int     // 来自模型上下文窗口
    threshold  float64 // 来自 config，默认 0.85
}

func (c *ContextCompressor) Check(ctx context.Context) (bool, error)
func (c *ContextCompressor) Compress(ctx context.Context) (int, error)
func (c *ContextCompressor) AvailableBudget() int
```

### 3.4 压缩流程

```
token 使用率 >= threshold?
    │
    ├── 否 → 跳过
    │
    └── 是 → 取出前 70% 消息
              ↓
         调用 LLM 生成摘要
              ↓
         替换为单条 [Context compressed: ...]
              ↓
         保留最近 30% 消息不动
              ↓
         被压缩区域 cache 失效，系统提示 cache 存活
         滚动窗口 1-2 轮内重新建立 caching
```

### 3.5 Token 预算分配

```
总上下文窗口（来自模型配置，如 65536）
├── 系统提示（PromptBuilder 输出，不计入工作记忆预算）
├── 记忆注入（语义记忆 facts，不计入工作记忆预算）
└── 工作记忆预算 = 总窗口 - 系统提示 - 记忆注入 - 安全余量
    ├── 最近 30% 消息（热区，不压缩）
    └── 早期 70% 消息（压缩区，超阈值时压缩）
```

### 3.6 最大上下文自动获取

```go
// memory/context_windows.go

var ModelContextWindows = map[string]int{
    "deepseek-chat":       65536,
    "deepseek-coder":      65536,
    "gpt-4o":              128000,
    "gpt-4-turbo":         128000,
    "claude-sonnet-4-6":   200000,
    "claude-opus-4-7":     200000,
    // 默认 8192
}

func GetContextWindow(model string, configOverride int) int
```

配置优先级：`config.yaml 手动设置 > 模型上下文窗口表 > 默认 8192`

### 3.7 配置

```yaml
# config.yaml 新增 memory 段
memory:
  working:
    compression_threshold: 0.85
    max_tokens: 0  # 0 = 自动从模型获取
```

## 4. L2 情景记忆（Episodic Memory）

### 4.1 接口定义

```go
// memory/episodic.go

type EpisodicStore interface {
    SaveSession(ctx context.Context, session Session) error
    Search(ctx context.Context, query string, limit int) ([]Episode, error)
    Summarize(ctx context.Context, query string, episodes []Episode) (string, error)
    ListSessions(ctx context.Context, limit int) ([]SessionSummary, error)
    DeleteSession(ctx context.Context, sessionID string) error
}

type Session struct {
    ID        string
    Title     string
    Source    string
    CreatedAt time.Time
    Episodes  []Episode
}

type Episode struct {
    ID         string
    SessionID  string
    Role       string
    Content    string
    CreatedAt  time.Time
    TokenCount int
}

type SessionSummary struct {
    ID        string
    Title     string
    Source    string
    CreatedAt time.Time
    MsgCount  int
}
```

### 4.2 PostgreSQL 表结构

```sql
-- 003_create_episodic.sql

CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL DEFAULT 'cli',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_id   TEXT REFERENCES sessions(id)
);

CREATE TABLE IF NOT EXISTS episodes (
    id          TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    token_count INT NOT NULL DEFAULT 0,
    fts_vector  tsvector GENERATED ALWAYS AS (to_tsvector('simple', content)) STORED
);

CREATE INDEX idx_episodes_session ON episodes(session_id);
CREATE INDEX idx_episodes_fts ON episodes USING GIN(fts_vector);
```

### 4.3 FTS 检索实现

使用 PostgreSQL 原生 `tsvector` + `ts_rank`，替换现有 ILIKE 方案：

```go
func (s *PgEpisodicStore) Search(ctx context.Context, query string, limit int) ([]Episode, error) {
    sql := `
        SELECT id, session_id, role, content, created_at, token_count,
               ts_rank(fts_vector, plainto_tsquery('simple', $1)) AS rank
        FROM episodes
        WHERE fts_vector @@ plainto_tsquery('simple', $1)
        ORDER BY rank DESC
        LIMIT $2
    `
    // 执行查询，返回结果
}
```

### 4.4 检索 → 摘要 → 注入流程

```
Agent 调用 session_search 工具
    ↓
EpisodicStore.Search(query, limit=5)
    ↓
返回原始 Episode 片段
    ↓
EpisodicStore.Summarize()
    ├── 拼接片段为上下文
    ├── 调用 LLM 生成摘要
    └── 返回摘要字符串
    ↓
摘要作为工具结果写入 WorkingMemory
    ↓
下一次 LLM 调用时，摘要出现在消息列表中（非系统提示）
```

### 4.5 并发写入控制

```go
func (s *PgEpisodicStore) SaveSession(ctx context.Context, session Session) error {
    return retryWithJitter(3, 20*time.Millisecond, 150*time.Millisecond, func() error {
        tx, err := s.pool.Begin(ctx)
        // INSERT session + batch INSERT episodes
        // COMMIT
    })
}
```

### 4.6 与现有代码的关系

- **替代** `memory/shortterm.go` 的 `FileShortTermMemory`
- **复用** PG 连接池
- **新增** `episodes` 表（现有 `facts` 表保留给 L3）

## 5. L3 语义记忆（Semantic Memory）

### 5.1 接口定义

```go
// memory/semantic.go

type SemanticStore interface {
    Remember(ctx context.Context, fact Fact) error
    Recall(ctx context.Context, query string, topK int) ([]Fact, error)
    Update(ctx context.Context, id string, updates map[string]any) error
    Delete(ctx context.Context, id string) error
    ForgetByFilter(ctx context.Context, filter ForgetFilter) error
}
```

### 5.2 实现层（可插拔）

```go
// 两个实现：
// 1. PgFTSSemanticStore — 当前，基于 PG + FTS
// 2. PgVectorSemanticStore — 未来，基于 pgvector 语义检索

func NewSemanticStore(pool *pgpool.Pool, embedder Embedder, useVector bool) SemanticStore {
    if useVector && embedder != nil {
        return &PgVectorSemanticStore{pool: pool, embedder: embedder}
    }
    return &PgFTSSemanticStore{pool: pool}
}
```

### 5.3 注入策略（关键变化）

```
当前（错误）：
  buildMessages() → facts 注入为 system message → 破坏 prompt cache

Hermes（正确）：
  buildMessages() → facts 不进入系统提示
                 → facts 作为 user message 注入消息列表
```

### 5.4 与现有代码的关系

- **重构** `LongTermMemory` 接口为 `SemanticStore`
- **保留** `facts` 表结构（含 pgvector embedding）
- **保留** FTS + vector 混合检索策略
- **新增** `ForgetByFilter` 实现

## 6. L4 技能记忆（Skill Memory）

### 6.1 接口定义

```go
// memory/skill.go

type SkillStore interface {
    Scan(ctx context.Context) ([]SkillIndex, error)
    Read(ctx context.Context, name string) (string, error)
    ReadFile(ctx context.Context, name string, path string) ([]byte, error)
    Manage(ctx context.Context, action SkillAction, name string, content string) error
    Search(ctx context.Context, query string, limit int) ([]SkillIndex, error)
}

type SkillIndex struct {
    Name        string   `yaml:"name"`
    Description string   `yaml:"description"`
    Version     string   `yaml:"version"`
    Tags        []string `yaml:"tags"`
}

type SkillAction string

const (
    SkillCreate SkillAction = "create"
    SkillPatch  SkillAction = "patch"
    SkillEdit   SkillAction = "edit"
    SkillDelete SkillAction = "delete"
)
```

### 6.2 与现有 skills/ 包的关系

**扩展现有 skills/ 包**，`SkillStore` 接口定义在 `memory/` 包中，实现复用 `skills/` 包的现有代码。

### 6.3 Level-0 索引注入

启动时扫描所有 `SKILL.md`，提取 frontmatter，生成索引注入系统提示：

```
## Available Skills
- novel-writing: 长篇小说创作助手 (v1.0) [写作, 创作]
- daily-poem: 每日诗歌生成 (v1.0) [诗歌, 文学]

使用 read_skill 工具读取完整技能内容。鼓励在完成复杂任务后创建新技能。
```

### 6.4 容量控制

```go
const MaxSkillIndexInPrompt = 20

// 技能数量 > 20 时，仅注入前 20 个 + 提示 Agent 使用搜索
```

## 7. PromptBuilder

### 7.1 接口定义

```go
// prompt/builder.go

type PromptBuilder struct {
    soul       string
    platform   string
    memory     FrozenSnapshot
    skills     string
    context    string
    tools      string
    built      string
    builtOnce  sync.Once
}

type FrozenSnapshot struct {
    Memory string // MEMORY.md（~2200 字符上限）
    User   string // USER.md（~1375 字符上限）
}

func (b *PromptBuilder) Build() string {
    b.builtOnce.Do(func() {
        // 按顺序组装，组装后冻结
    })
    return b.built
}
```

### 7.2 组装顺序

```
1. Persona ──────────────── SOUL.md 或默认身份
2. Platform hints ────────── CLI/Telegram/Slack 平台提示
3. Memory guidance ───────── MEMORY.md § USER.md（冻结快照）
4. Session search hint ───── session_search 工具使用提示
5. Skills guidance ────────── Level-0 技能索引
6. Context files ─────────── AGENTS.md / .agent.md
7. Tool-use enforcement ──── 并行调用、错误恢复规则
8. Tool schemas ──────────── 所有工具 JSON schema
```

### 7.3 Frozen Snapshot 安全扫描

```go
// memory/security.go

func ScanForInjection(content string) []SecurityWarning {
    // 1. Prompt injection 检测
    // 2. 数据外泄检测（curl/wget + env vars）
    // 3. 持久化后门检测
    // 4. 不可见 Unicode 检测
}
```

### 7.4 SOUL.md 默认内容

```markdown
---
name: go-agent
identity: A helpful AI coding assistant
voice: Professional, concise, technically precise
values: Accuracy, clarity, user autonomy
---

You are a helpful AI assistant running in a terminal environment.
You help users with software engineering tasks, coding, and system administration.
You are precise, concise, and always verify before acting.
```

## 8. Agent 工具

### 8.1 新增工具

| 工具名 | 操作对象 | 功能 |
|--------|---------|------|
| `memory_append` | MEMORY.md / USER.md | 追加内容 |
| `memory_replace` | MEMORY.md / USER.md | 替换内容 |
| `memory_delete` | MEMORY.md / USER.md | 删除内容 |
| `session_search` | 情景记忆 | 全文检索历史会话 |
| `read_skill` | 技能记忆 | 读取 SKILL.md |
| `skill_manage` | 技能记忆 | 创建/编辑/删除技能 |

### 8.2 安全约束

memory_* 工具写入前必须：
1. 容量检查：MEMORY.md <= 2200 字符，USER.md <= 1375 字符
2. 安全扫描：`ScanForInjection(content)`
3. 备份：写入前备份原文件为 `.bak`
4. 冻结：当前会话的 PromptBuilder 不重新构建

### 8.3 session_search 工具流程

```go
func (t *SessionSearchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
    query := params["query"].(string)
    limit := params["limit"].(int) // 默认 5
    
    // 1. 全文检索
    episodes, err := t.episodic.Search(ctx, query, limit)
    
    // 2. LLM 摘要压缩
    summary, err := t.episodic.Summarize(ctx, query, episodes)
    
    // 3. 返回摘要（作为工具结果）
    return summary, nil
}
```

## 9. Agent 集成

### 9.1 Agent 结构体

```go
type Agent struct {
    working       WorkingMemory
    episodic      EpisodicStore
    semantic      SemanticStore
    skillStore    SkillStore
    promptBuilder *PromptBuilder
    compressor    *ContextCompressor
    classifier    *Classifier
    config        AgentConfig
    history       []client.Message
}
```

### 9.2 主循环

```go
func (a *Agent) Run(ctx context.Context, input string) <-chan AgentEvent {
    ch := make(chan AgentEvent, 64)
    go func() {
        defer close(ch)
        
        // 1. 写入工作记忆
        a.working.Add(Message{Role: "user", Content: input})
        
        // 2. 压缩检查
        if a.compressor.NeedsCompression() {
            ch <- AgentEvent{Type: EventCompressing}
            a.compressor.Compress(ctx)
            ch <- AgentEvent{Type: EventCompressed}
        }
        
        // 3. ReAct 循环
        for {
            msgs := a.buildMessages(input)
            resp, err := a.client.Chat(ctx, msgs)
            // ... 工具调用、结果写入工作记忆 ...
        }
        
        // 4. 每轮结束
        a.working.Add(Message{Role: "assistant", Content: answer})
        go a.episodic.SaveSession(ctx, session)
        go a.classifyAndStore(ctx, input, answer)
    }()
    return ch
}
```

### 9.3 buildMessages 改造

```go
func (a *Agent) buildMessages(userQuery string) []client.Message {
    // 1. 系统提示（冻结）
    msgs := []client.Message{{Role: "system", Content: a.promptBuilder.Build()}}
    
    // 2. 语义记忆（作为 user 消息注入，非系统提示，避免破坏 prompt cache）
    facts, _ := a.semantic.Recall(ctx, userQuery, 10)
    if len(facts) > 0 {
        msgs = append(msgs, client.Message{
            Role:    "user",
            Content: "[Memory Context]\n" + formatFacts(facts),
        })
    }
    
    // 3. 工作记忆窗口（预算感知）
    budget := a.compressor.AvailableBudget()
    window := a.working.GetWindow(budget)
    msgs = append(msgs, convertMessages(window)...)
    
    return msgs
}
```

### 9.4 Event 类型扩展

```go
const (
    EventCompressing EventType = "compressing"
    EventCompressed  EventType = "compressed"
)
```

## 10. 前端（TUI）改动

### 10.1 新增状态

```go
const (
    StateReady       AgentState = iota
    StateThinking
    StateExecuting
    StateCompressing  // 新增
)
```

### 10.2 新增面板类型

```go
const (
    PanelCompressed PanelType = iota + 6
)

// 渲染效果
case PanelCompressed:
    header := dimStyle.Render("─── Context Compressed ───")
    detail := dimStyle.Render(fmt.Sprintf("  %s (%d tokens saved)", p.Content, p.TokenCount))
    return "\n" + header + "\n" + detail + "\n"
```

### 10.3 压缩动画流程

```
EventCompressing → 状态栏显示 spinner + "Compressing context..."
EventCompressed  → 视口插入 PanelCompressed 面板
                 → 恢复 StateThinking
```

## 11. 数据迁移

### 11.1 新建表

```sql
-- 003_create_episodic.sql
-- 见第 4.2 节
```

### 11.2 旧数据迁移

```go
// migrations/migrate.go

func MigrateOldSessions(ctx, pool, sessionDir) error {
    // 读取 ~/.go-agent/memory/sessions/*.json
    // INSERT INTO sessions + episodes
    // 保留旧 JSON 文件作为备份
}

func MigrateOldFacts(ctx, pool) error {
    // facts 表结构不变，接口重命名
    // 无需数据迁移
}
```

## 12. 文件结构

```
memory/
├── types.go              # 核心类型
├── config.go             # 配置（新增 compression_threshold）
├── errors.go             # 哨兵错误
├── tokenizer.go          # TiktokenTokenizer（替换 SimpleTokenizer）
├── context_windows.go    # 新增：模型上下文窗口表
├── working.go            # L1 工作记忆（新增 Compress）
├── compression.go        # 新增：ContextCompressor
├── episodic.go           # 新增：L2 情景记忆接口 + PG 实现
├── semantic.go           # 新增：L3 语义记忆接口
├── longterm.go           # 保留：PgFTSSemanticStore 实现
├── embedding.go          # 保留：Embedder 系统
├── classifier.go         # 保留：LLM 分类器
├── compaction.go         # 保留：衰减计算
├── security.go           # 新增：安全扫描
├── manager.go            # 重构：统一管理四层
├── shortterm.go          # 废弃：被 episodic.go 替代
├── meta.go               # 废弃：被 semantic.go 吸收
└── *_test.go

prompt/
└── builder.go            # 重构：PromptBuilder

tools/
├── memory_tools.go       # 新增：memory_append/replace/delete
├── session_tool.go       # 新增：session_search
└── skill_tools.go        # 新增：read_skill, skill_manage

agent/
└── agent.go              # 重构：集成四层 + PromptBuilder

tui/
├── app.go                # 修改：新增 StateCompressing
└── viewport.go           # 修改：新增 PanelCompressed

migrations/
└── 003_create_episodic.sql  # 新增
```

## 13. 配置变更

```yaml
# config.yaml 新增 memory 段
memory:
  working:
    compression_threshold: 0.85
    max_tokens: 0  # 0 = 自动从模型获取
  episodic:
    enabled: true
  semantic:
    use_vector: false  # true = pgvector, false = FTS-only
  skills:
    dir: ~/.go-agent/skills/
    max_index: 20
```

## 14. 依赖新增

```
github.com/pkoukk/tiktoken-go  # Token 计数
gopkg.in/yaml.v3               # YAML frontmatter 解析（已有）
```

## 15. 实现阶段

### Phase 1: 基础设施
- PromptBuilder + Frozen Snapshot
- TiktokenTokenizer 替换
- 安全扫描
- SOUL.md / MEMORY.md / USER.md 文件管理

### Phase 2: L1 工作记忆改造
- ContextCompressor 实现
- 前端压缩动画
- 压缩阈值配置
- 模型上下文窗口自动获取

### Phase 3: L2 情景记忆
- episodes 表迁移脚本
- PgEpisodicStore 实现
- session_search 工具
- LLM 摘要压缩
- 旧数据迁移

### Phase 4: L3 语义记忆重构
- SemanticStore 接口
- 注入策略改造（非系统提示）
- memory_append/replace/delete 工具
- ForgetByFilter 实现

### Phase 5: L4 技能记忆
- SkillStore 接口
- Level-0 索引注入
- read_skill / skill_manage 工具

### Phase 6: Agent 集成
- Agent 主循环改造
- buildMessages 重构
- 事件类型扩展
- 端到端测试
