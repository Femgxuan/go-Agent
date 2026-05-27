# Long-Term Memory Optimization Design

## Goal

采用 Hermes 风格的语义分层策略，优化"什么该存入 PostgreSQL 长期记忆"的判定逻辑。通过规则初筛 + LLM 精确分类的混合模式，结合 PostgreSQL FTS + pgvector 双索引检索，提升记忆系统的准确率和召回率。

## Current Problems

1. `extractMemoryFact` 纯关键词匹配，只能捕获"记住"前缀和偏好模式
2. 无语义/情景分离，所有信息走同一条路径
3. `buildMessages()` 用空字符串查询 `Retrieve(ctx, "", ...)`，向量搜索无意义
4. Fact 的 `Key` 字段是自由文本（"用户记忆"、"用户偏好"），缺乏结构化分类
5. 无 LLM 辅助提取，无法理解上下文语义

## Architecture

```
用户输入
  ↓
规则初筛 (extractMemoryFact 增强版)
  ↓ 命中 → LLM 分类器
  ↓ 跳过 → 不存储
LLM 分类 → 结构化 Fact[]
  ↓
PostgreSQL (tsvector + pgvector 双索引)
  ↓
检索: FTS + 向量混合 → 合并去重 → 注入 prompt
```

## Design

### 1. Fact 分类枚举

替代当前的自由文本 Key，使用预定义分类：

```go
type FactCategory string

const (
    FactPreference  FactCategory = "preference"   // 用户偏好："我喜欢 Go"
    FactEnvironment FactCategory = "environment"  // 环境事实："服务器是 Debian 12"
    FactCorrection  FactCategory = "correction"   // 纠正信息："不要用 sudo"
    FactNorm        FactCategory = "norm"         // 项目规范："用 tab 缩进"
    FactMilestone   FactCategory = "milestone"    // 已完成工作："完成了 PG 迁移"
    FactExplicit    FactCategory = "explicit"     // 显式请求："记住 API key"
)
```

Fact 结构体修改：

```go
type Fact struct {
    ID         string       `json:"id"`
    Category   FactCategory `json:"category"`    // 新增：结构化分类
    Key        string       `json:"key"`         // 保留：人类可读标题
    Content    string       `json:"content"`
    Source     string       `json:"source"`      // "user" | "agent" | "derived"
    Confidence float64      `json:"confidence"`  // 新增：LLM 分类置信度
    CreatedAt  time.Time    `json:"created_at"`
    DecayScore float64      `json:"decay_score"`
}
```

### 2. 规则初筛器增强

扩展 `extractMemoryFact()` 的模式库，从当前 2 类扩展到 6 类：

| 类型 | 匹配模式（中/英） |
|------|-------------------|
| Preference | "我喜欢"、"我不喜欢"、"I like"、"I prefer" |
| Environment | "服务器是"、"运行在"、"部署在"、"running on"、"deployed on" |
| Correction | "不要用"、"别用"、"don't use"、"stop using" |
| Norm | "代码风格"、"规范是"、"coding style"、"convention" |
| Milestone | "完成了"、"搞定了"、"迁移了"、"finished"、"completed" |
| Explicit | "记住"、"remember" |

初筛结果不直接存储，而是作为候选传给 LLM 分类器。

### 3. LLM 分类器

新文件 `memory/classifier.go`：

```go
type Classifier struct {
    llm client.LLMClient
}

type ClassificationResult struct {
    ShouldStore bool    `json:"should_store"`
    Facts       []Fact  `json:"facts"`
    Reason      string  `json:"reason"`
}
```

工作流程：
1. 接收用户输入 + 规则初筛命中类型
2. 构造轻量 prompt，要求 LLM 输出 JSON：
   - `should_store`: 是否值得存入长期记忆
   - `facts`: 提取的结构化事实列表（可能多条）
   - `reason`: 判定理由
3. 解析 JSON，返回 `[]Fact`

Prompt 设计要点：
- 只在规则初筛通过时调用（避免无意义 API 开销）
- 使用当前对话的 assistant 回复作为上下文（事实可能在回复中而非用户输入中）
- temperature=0 保证确定性输出

### 4. PostgreSQL 双索引

**迁移文件** `migrations/002_add_fts.sql`：

```sql
-- 增加 tsvector 列
ALTER TABLE facts ADD COLUMN fts_vector tsvector;

-- 创建 GIN 索引
CREATE INDEX idx_facts_fts ON facts USING gin(fts_vector);

-- 触发器：自动更新 tsvector
CREATE OR REPLACE FUNCTION facts_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.key, '') || ' ' || COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON facts
    FOR EACH ROW EXECUTE FUNCTION facts_fts_trigger();

-- 回填现有数据
UPDATE facts SET fts_vector = to_tsvector('simple', COALESCE(key, '') || ' ' || COALESCE(content, ''));
```

**Store 变更** — `memory/longterm.go`：
- INSERT 时 tsvector 由触发器自动维护，无需改代码

**Search 变更** — `memory/longterm.go`：
```go
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
    if query == "" {
        // 无查询时按 decay_score 排序返回最新
        return p.recent(ctx, limit)
    }
    // FTS + 向量混合
    // 1. FTS 结果
    ftsResults := p.ftsSearch(ctx, query, limit)
    // 2. 向量结果
    vecResults := p.vectorSearch(ctx, query, limit, minScore)
    // 3. 合并去重，按综合得分排序
    return p.mergeResults(ftsResults, vecResults, limit), nil
}
```

合并策略：
- FTS 结果按 `ts_rank` 得分
- 向量结果按余弦相似度
- 同一 fact 出现在两边时取较高分
- 最终按得分降序，取 limit 条

### 5. 检索注入优化

**`agent/agent.go` — `buildMessages()`**：
- 传入当前用户输入作为 query（替代空字符串）
- 检索结果按 Category 分组注入：

```go
// 按类型分组注入
categoryLabels := map[FactCategory]string{
    FactPreference:  "用户偏好",
    FactEnvironment: "环境信息",
    FactCorrection:  "注意事项",
    FactNorm:        "项目规范",
    FactMilestone:   "历史记录",
    FactExplicit:    "记忆",
}
for _, fact := range memCtx.RelevantFacts {
    label := categoryLabels[fact.Category]
    if label == "" {
        label = "相关记忆"
    }
    msgs = append(msgs, client.Message{
        Role:    client.RoleSystem,
        Content: fmt.Sprintf("[%s] %s: %s", label, fact.Key, fact.Content),
    })
}
```

### 6. Agent 集成

**时序变更**：当前 `extractMemoryFact` 在 `Run()` 开头调用（对话开始前）。新设计中，LLM 分类需要 assistant 回复作为上下文，所以分类移到 `storeInteraction()` 中（对话完成后）。

**`agent/agent.go` — `storeInteraction()` 修改**：

```go
func (a *Agent) storeInteraction(ctx context.Context, input string) {
    // ... 现有的 working memory + short term memory 逻辑 ...

    // 新增：规则初筛 + LLM 分类
    if a.config.MemoryEnabled && a.memory != nil {
        if candidate, ok := extractMemoryFact(input); ok {
            // 异步执行 LLM 分类，不阻塞
            go func() {
                facts, err := a.classifier.Classify(ctx, input, lastAssistant, candidate)
                if err != nil {
                    slog.Debug("memory classification failed", "error", err)
                    return
                }
                for _, fact := range facts {
                    if err := a.memory.Memorize(ctx, fact); err != nil {
                        slog.Debug("failed to memorize fact", "error", err)
                    }
                }
            }()
        }
    }
}
```

移除 `Run()` 中的 `extractMemoryFact` + `Memorize` 调用（约第 124-131 行）。

## Files to Modify

| File | Change |
|------|--------|
| `memory/types.go` | Fact 增加 Category、Confidence 字段；增加 FactCategory 枚举 |
| `memory/classifier.go` | 新建：LLM 分类器 |
| `memory/longterm.go` | Search 改为 FTS + 向量混合；增加 ftsSearch、vectorSearch、mergeResults |
| `memory/manager.go` | Memorize 适配新 Fact 结构 |
| `agent/agent.go` | extractMemoryFact 增强；Run() 集成 Classifier；buildMessages() 传 query |
| `migrations/002_add_fts.sql` | 新建：tsvector 列 + GIN 索引 + 触发器 |
| `memory/config.go` | 可选：增加 Classifier 相关配置 |

## Testing

- 单元测试：Classifier 的 JSON 解析、FactCategory 枚举、mergeResults 去重逻辑
- 集成测试：规则初筛 → LLM 分类 → PG 存储 → 检索注入完整链路
- 手动验证：说"我喜欢 Go 语言" → 存为 preference；说"帮我查天气" → 不存储
