# Long-Term Memory Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 采用 Hermes 风格的语义分层策略，通过规则初筛 + LLM 分类 + PostgreSQL FTS + pgvector 双索引，优化长期记忆的存入判定和检索。

**Architecture:** 规则引擎做初筛（快速过滤），通过初筛的用 LLM 做精确分类提取结构化事实，PostgreSQL 同时维护 tsvector 和 pgvector 双索引，检索时 FTS + 向量混合排序。

**Tech Stack:** Go, PostgreSQL, pgvector, Bubbletea (已有)

---

### Task 1: Fact 类型增加 Category 和 Confidence 字段

**Files:**
- Modify: `memory/types.go`

- [ ] **Step 1: 添加 FactCategory 枚举和修改 Fact 结构体**

在 `memory/types.go` 的 `Fact` 结构体之前添加枚举类型，然后修改 `Fact`：

```go
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
```

- [ ] **Step 2: 验证编译**

Run: `go build ./memory/...`
Expected: 编译通过（现有代码中使用 Fact 的地方可能需要适配，但新字段都有零值，不影响现有逻辑）

- [ ] **Step 3: 运行现有测试**

Run: `go test ./memory/... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 4: Commit**

```bash
git add memory/types.go
git commit -m "feat(memory): add FactCategory enum and Category/Confidence to Fact struct"
```

---

### Task 2: PostgreSQL FTS 迁移

**Files:**
- Create: `migrations/002_add_fts.sql`

- [ ] **Step 1: 创建迁移文件**

```sql
-- migrations/002_add_fts.sql
-- 启用 pg_trgm 扩展（用于模糊匹配）
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 增加 tsvector 列
ALTER TABLE facts ADD COLUMN IF NOT EXISTS fts_vector tsvector;

-- 创建 GIN 索引
CREATE INDEX IF NOT EXISTS idx_facts_fts ON facts USING gin(fts_vector);

-- 创建触发器函数：自动更新 tsvector
CREATE OR REPLACE FUNCTION facts_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.key, '') || ' ' || COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 创建触发器
DROP TRIGGER IF EXISTS tsvector_update ON facts;
CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON facts
    FOR EACH ROW EXECUTE FUNCTION facts_fts_trigger();

-- 回填现有数据
UPDATE facts SET fts_vector = to_tsvector('simple', COALESCE(key, '') || ' ' || COALESCE(content, ''))
WHERE fts_vector IS NULL;
```

- [ ] **Step 2: 执行迁移**

Run: `psql -U FengXuan -d FengXuan -f migrations/002_add_fts.sql`
Expected: 所有语句成功执行

- [ ] **Step 3: 验证索引**

Run: `psql -U FengXuan -d FengXuan -c "\d facts"`
Expected: 表结构中包含 `fts_vector tsvector` 列和 `idx_facts_fts` 索引

- [ ] **Step 4: Commit**

```bash
git add migrations/002_add_fts.sql
git commit -m "feat(memory): add PostgreSQL FTS column, index, and trigger"
```

---

### Task 3: longterm.go 支持 FTS + 向量混合检索

**Files:**
- Modify: `memory/longterm.go`

- [ ] **Step 1: 修改 Store 方法支持 Category 和 Confidence**

在 `memory/longterm.go` 的 `Store` 方法中，修改 INSERT 语句增加 `category` 和 `confidence` 列：

```go
func (p *PgLongTermMemory) Store(ctx context.Context, fact Fact) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	embedding, err := p.embedder.Embed(ctx, fact.Content)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	_, err = p.pool.Exec(ctx, `
		INSERT INTO facts (id, category, key, content, source, confidence, embedding, decay_score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			category = EXCLUDED.category,
			key = EXCLUDED.key,
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			confidence = EXCLUDED.confidence,
			embedding = EXCLUDED.embedding,
			decay_score = EXCLUDED.decay_score,
			updated_at = EXCLUDED.updated_at
	`, fact.ID, string(fact.Category), fact.Key, fact.Content, fact.Source, fact.Confidence,
		pgvector.NewVector(toFloat32(embedding)), fact.DecayScore, fact.CreatedAt, time.Now())

	if err != nil {
		return fmt.Errorf("inserting fact: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: 添加 ftsSearch 方法**

在 `Delete` 方法之后添加：

```go
// ftsSearch performs full-text search using PostgreSQL tsvector.
func (p *PgLongTermMemory) ftsSearch(ctx context.Context, query string, limit int) []Fact {
	rows, err := p.pool.Query(ctx, `
		SELECT id, key, content, source, category, confidence, decay_score, created_at,
		       ts_rank(fts_vector, q) as rank
		FROM facts, plainto_tsquery('simple', $1) q
		WHERE fts_vector @@ q
		ORDER BY rank DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []Fact
	for rows.Next() {
		var f Fact
		var category string
		var rank float64
		if err := rows.Scan(&f.ID, &f.Key, &f.Content, &f.Source, &category, &f.Confidence, &f.DecayScore, &f.CreatedAt, &rank); err != nil {
			continue
		}
		f.Category = FactCategory(category)
		f.DecayScore = rank
		results = append(results, f)
	}
	return results
}
```

- [ ] **Step 3: 添加 vectorSearch 方法**

在 `ftsSearch` 之后添加：

```go
// vectorSearch performs semantic search using pgvector cosine distance.
func (p *PgLongTermMemory) vectorSearch(ctx context.Context, query string, limit int, minScore float64) []Fact {
	queryEmbedding, err := p.embedder.Embed(ctx, query)
	if err != nil {
		return nil
	}

	rows, err := p.pool.Query(ctx, `
		SELECT id, key, content, source, category, confidence, decay_score, created_at,
		       1 - (embedding <=> $1) as similarity
		FROM facts
		WHERE 1 - (embedding <=> $1) > $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`, pgvector.NewVector(toFloat32(queryEmbedding)), minScore, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []Fact
	for rows.Next() {
		var f Fact
		var category string
		var similarity float64
		if err := rows.Scan(&f.ID, &f.Key, &f.Content, &f.Source, &category, &f.Confidence, &f.DecayScore, &f.CreatedAt, &similarity); err != nil {
			continue
		}
		f.Category = FactCategory(category)
		f.DecayScore = similarity
		results = append(results, f)
	}
	return results
}
```

- [ ] **Step 4: 添加 mergeResults 方法**

在 `vectorSearch` 之后添加：

```go
// mergeResults combines FTS and vector search results, deduplicates by ID,
// and returns top limit results sorted by score descending.
func (p *PgLongTermMemory) mergeResults(ftsResults, vecResults []Fact, limit int) []Fact {
	seen := make(map[string]int) // ID -> index in merged
	var merged []Fact

	for _, f := range ftsResults {
		if idx, ok := seen[f.ID]; ok {
			if f.DecayScore > merged[idx].DecayScore {
				merged[idx].DecayScore = f.DecayScore
			}
		} else {
			seen[f.ID] = len(merged)
			merged = append(merged, f)
		}
	}
	for _, f := range vecResults {
		if idx, ok := seen[f.ID]; ok {
			if f.DecayScore > merged[idx].DecayScore {
				merged[idx].DecayScore = f.DecayScore
			}
		} else {
			seen[f.ID] = len(merged)
			merged = append(merged, f)
		}
	}

	// Sort by score descending
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].DecayScore > merged[j].DecayScore
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
```

- [ ] **Step 5: 添加 recent 方法（空查询回退）**

在 `mergeResults` 之后添加：

```go
// recent returns the most recent facts ordered by created_at, used when query is empty.
func (p *PgLongTermMemory) recent(ctx context.Context, limit int) []Fact {
	rows, err := p.pool.Query(ctx, `
		SELECT id, key, content, source, category, confidence, decay_score, created_at
		FROM facts
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []Fact
	for rows.Next() {
		var f Fact
		var category string
		if err := rows.Scan(&f.ID, &f.Key, &f.Content, &f.Source, &category, &f.Confidence, &f.DecayScore, &f.CreatedAt); err != nil {
			continue
		}
		f.Category = FactCategory(category)
		results = append(results, f)
	}
	return results
}
```

- [ ] **Step 6: 重写 Search 方法为混合检索**

替换现有的 `Search` 方法：

```go
// Search performs hybrid FTS + vector search.
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if query == "" {
		return p.recent(ctx, limit), nil
	}

	// FTS results
	ftsResults := p.ftsSearch(ctx, query, limit)
	// Vector results
	vecResults := p.vectorSearch(ctx, query, limit, minScore)
	// Merge and deduplicate
	merged := p.mergeResults(ftsResults, vecResults, limit)

	return merged, nil
}
```

- [ ] **Step 7: 添加 sort import**

确保 `memory/longterm.go` 的 import 中包含 `"sort"`。

- [ ] **Step 8: 验证编译**

Run: `go build ./memory/...`
Expected: 编译通过

- [ ] **Step 9: 运行测试**

Run: `go test ./memory/... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 10: Commit**

```bash
git add memory/longterm.go
git commit -m "feat(memory): hybrid FTS + vector search in PgLongTermMemory"
```

---

### Task 4: LLM 分类器

**Files:**
- Create: `memory/classifier.go`

- [ ] **Step 1: 创建 Classifier 结构体和接口**

```go
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fengxuan/go-agent/client"
)

// Classifier uses an LLM to classify whether user input contains
// information worth storing in long-term memory.
type Classifier struct {
	llm client.LLMClient
}

// NewClassifier creates a new Classifier.
func NewClassifier(llm client.LLMClient) *Classifier {
	return &Classifier{llm: llm}
}

// classificationResponse is the expected JSON output from the LLM.
type classificationResponse struct {
	ShouldStore bool   `json:"should_store"`
	Category    string `json:"category"`
	Key         string `json:"key"`
	Content     string `json:"content"`
	Reason      string `json:"reason"`
}
```

- [ ] **Step 2: 实现 Classify 方法**

在 `classificationResponse` 之后添加：

```go
const classifyPrompt = `You are a memory classifier. Given a user message and an assistant reply, determine if the user's message contains information worth remembering long-term.

Categories:
- preference: User preferences, likes, dislikes ("I like Go", "我不喜欢Java")
- environment: Environment facts, server info, tech stack ("服务器是Debian 12", "We use PostgreSQL")
- correction: Corrections to agent behavior ("不要用sudo", "Don't use tabs")
- norm: Project conventions, coding style ("代码风格用Google", "Use 120-char lines")
- milestone: Completed work, achievements ("完成了迁移", "Finished the auth module")
- explicit: Direct memory requests ("记住...", "Remember...")

Rules:
- DO NOT store: greetings, general questions, tool requests, temporary info, things easily re-searchable
- DO store: facts/preferences/rules that remain true across sessions
- If user says something like "帮我查天气" (check weather), should_store=false
- If user says "我喜欢Go" (I like Go), should_store=true, category=preference

Respond with ONLY a JSON object:
{"should_store": true/false, "category": "...", "key": "short title", "content": "extracted fact", "reason": "why"}

If should_store is false, category/key/content can be empty strings.`

// Classify determines whether the user input contains a storable fact.
// It returns a Fact if classification succeeds and should_store=true, or zero Fact and false otherwise.
func (c *Classifier) Classify(ctx context.Context, userMsg, assistantMsg string) (Fact, bool) {
	prompt := fmt.Sprintf("%s\n\nUser: %s\nAssistant: %s", classifyPrompt, userMsg, assistantMsg)

	req := client.ChatRequest{
		Messages: []client.Message{
			{Role: client.RoleUser, Content: prompt},
		},
		MaxTokens: 256,
	}

	streamCh, err := c.llm.ChatCompletion(ctx, req)
	if err != nil {
		slog.Debug("classifier LLM call failed", "error", err)
		return Fact{}, false
	}

	var fullContent string
	for chunk := range streamCh {
		if chunk.Err != nil {
			slog.Debug("classifier stream error", "error", chunk.Err)
			return Fact{}, false
		}
		fullContent += chunk.Delta
	}

	fullContent = strings.TrimSpace(fullContent)
	// Strip markdown code fences if present
	fullContent = strings.TrimPrefix(fullContent, "```json")
	fullContent = strings.TrimPrefix(fullContent, "```")
	fullContent = strings.TrimSuffix(fullContent, "```")
	fullContent = strings.TrimSpace(fullContent)

	var resp classificationResponse
	if err := json.Unmarshal([]byte(fullContent), &resp); err != nil {
		slog.Debug("classifier JSON parse failed", "error", err, "raw", fullContent)
		return Fact{}, false
	}

	if !resp.ShouldStore {
		return Fact{}, false
	}

	category := FactCategory(resp.Category)
	switch category {
	case FactPreference, FactEnvironment, FactCorrection, FactNorm, FactMilestone, FactExplicit:
		// valid
	default:
		category = FactExplicit // fallback
	}

	return Fact{
		ID:         fmt.Sprintf("fact_%d", time.Now().UnixNano()),
		Category:   category,
		Key:        resp.Key,
		Content:    resp.Content,
		Source:     "user",
		Confidence: 0.8,
		CreatedAt:  time.Now(),
		DecayScore: 1.0,
	}, true
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./memory/...`
Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add memory/classifier.go
git commit -m "feat(memory): add LLM-based Classifier for fact extraction"
```

---

### Task 5: 增强 extractMemoryFact 规则初筛

**Files:**
- Modify: `agent/agent.go` (lines 486-535)

- [ ] **Step 1: 重写 extractMemoryFact 为多类型匹配**

替换现有的 `extractMemoryFact` 函数：

```go
// extractMemoryFact performs rule-based pre-filtering on user input.
// Returns a candidate Fact with category hint if patterns match, or zero Fact and false otherwise.
// The candidate is NOT stored directly — it's passed to the LLM classifier for precise extraction.
func extractMemoryFact(input string) (memory.Fact, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return memory.Fact{}, false
	}

	type pattern struct {
		category memory.FactCategory
		keywords []string
	}

	patterns := []pattern{
		// Explicit requests (prefix match)
		{memory.FactExplicit, []string{"记住", "remember"}},
		// Preferences
		{memory.FactPreference, []string{
			"我喜欢", "我不喜欢", "我想要", "我偏好", "我的爱好", "我最爱",
			"I like", "I love", "I prefer", "my favorite", "my hobby",
		}},
		// Environment facts
		{memory.FactEnvironment, []string{
			"服务器是", "运行在", "部署在", "系统是", "数据库是",
			"running on", "deployed on", "server is", "database is",
		}},
		// Corrections
		{memory.FactCorrection, []string{
			"不要用", "别用", "不用", "请不要", "请别",
			"don't use", "stop using", "never use", "please don't",
		}},
		// Norms
		{memory.FactNorm, []string{
			"代码风格", "规范是", "约定是", "格式是", "编码规范",
			"coding style", "convention", "code format",
		}},
		// Milestones
		{memory.FactMilestone, []string{
			"完成了", "搞定了", "迁移了", "部署了", "上线了",
			"finished", "completed", "migrated", "deployed", "shipped",
		}},
	}

	lower := strings.ToLower(input)
	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return memory.Fact{
					ID:         fmt.Sprintf("fact_%d", time.Now().UnixNano()),
					Category:   p.category,
					Key:        string(p.category),
					Content:    input,
					Source:     "user",
					Confidence: 0.5, // rule-based, low confidence
					CreatedAt:  time.Now(),
					DecayScore: 1.0,
				}, true
			}
		}
	}

	return memory.Fact{}, false
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./agent/...`
Expected: 编译通过

- [ ] **Step 3: 运行测试**

Run: `go test ./agent/... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 4: Commit**

```bash
git add agent/agent.go
git commit -m "feat(agent): enhance extractMemoryFact with multi-category rule pre-filter"
```

---

### Task 6: Agent 集成 — Classifier + buildMessages + 时序调整

**Files:**
- Modify: `agent/agent.go` (Agent struct, New, Run, storeInteraction, buildMessages)
- Modify: `runtime/runtime.go` (wiring Classifier)
- Modify: `memory/manager.go` (Memorize 适配新 Fact 字段)

- [ ] **Step 1: Agent 结构体增加 classifier 字段**

在 `agent/agent.go` 的 `Agent` 结构体中添加：

```go
type Agent struct {
	llm        client.LLMClient
	registry   *tools.Registry
	history    []client.Message
	mu         sync.Mutex
	config     AgentConfig
	callbacks  AgentCallbacks
	memory     memory.Manager
	classifier *memory.Classifier // 新增
}
```

- [ ] **Step 2: 修改 New 函数接受 Classifier**

```go
func New(llm client.LLMClient, registry *tools.Registry, config AgentConfig, mem memory.Manager, classifier *memory.Classifier) *Agent {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	if config.MaxTokens <= 0 {
		config.MaxTokens = 8192
	}
	return &Agent{
		llm:        llm,
		registry:   registry,
		config:     config,
		memory:     mem,
		classifier: classifier,
	}
}
```

- [ ] **Step 3: 修改 Run 方法 — 移除旧的 extractMemoryFact + Memorize 调用**

在 `Run` 方法中，删除第 124-131 行的这段代码：

```go
	// If memory system is enabled, auto-extract "记住"/偏好指令到长期记忆.
	if a.config.MemoryEnabled && a.memory != nil {
		if fact, ok := extractMemoryFact(input); ok {
			if err := a.memory.Memorize(ctx, fact); err != nil {
				slog.Warn("failed to memorize fact", "error", err)
			}
		}
	}
```

- [ ] **Step 4: 修改 storeInteraction — 集成 Classifier**

替换 `storeInteraction` 方法：

```go
func (a *Agent) storeInteraction(ctx context.Context, input string) {
	if !a.config.MemoryEnabled || a.memory == nil {
		return
	}
	a.mu.Lock()
	var lastAssistant string
	for i := len(a.history) - 1; i >= 0; i-- {
		if a.history[i].Role == client.RoleAssistant {
			lastAssistant = a.history[i].Content
			break
		}
	}
	a.mu.Unlock()
	if lastAssistant != "" {
		if err := a.memory.AddToWorkingMemory(memory.Message{
			Role:      "assistant",
			Content:   lastAssistant,
			Timestamp: time.Now(),
		}); err != nil {
			slog.Warn("failed to add assistant to working memory", "error", err)
		}
		go func() {
			if err := a.memory.Store(ctx, memory.Interaction{
				UserMsg:  input,
				AgentMsg: lastAssistant,
				Metadata: map[string]any{"timestamp": time.Now()},
			}); err != nil {
				slog.Warn("failed to store interaction", "error", err)
			}
		}()
	}

	// Rule pre-filter + LLM classification for long-term memory.
	if a.classifier != nil {
		if _, ok := extractMemoryFact(input); ok {
			go func() {
				classifyCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if fact, ok := a.classifier.Classify(classifyCtx, input, lastAssistant); ok {
					if err := a.memory.Memorize(ctx, fact); err != nil {
						slog.Debug("failed to memorize classified fact", "error", err)
					}
				}
			}()
		}
	}
}
```

- [ ] **Step 5: 修改 buildMessages — 传入用户输入作为检索 query**

修改 `buildMessages` 方法签名，增加 `query string` 参数：

```go
func (a *Agent) buildMessages(query string) []client.Message {
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
		ctx := context.Background()
		memCtx, err := a.memory.Retrieve(ctx, query, memory.RetrieveOptions{
			MaxTokens: a.config.MaxTokens,
		})
		if err != nil {
			slog.Warn("failed to retrieve memory context", "error", err)
		} else {
			categoryLabels := map[memory.FactCategory]string{
				memory.FactPreference:  "用户偏好",
				memory.FactEnvironment: "环境信息",
				memory.FactCorrection:  "注意事项",
				memory.FactNorm:        "项目规范",
				memory.FactMilestone:   "历史记录",
				memory.FactExplicit:    "记忆",
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
			for _, ref := range memCtx.SelfReflection {
				msgs = append(msgs, client.Message{
					Role:    client.RoleSystem,
					Content: fmt.Sprintf("[反思] %s", ref.Content),
				})
			}
		}
	}

	msgs = append(msgs, a.history...)
	return msgs
}
```

- [ ] **Step 6: 更新 buildMessages 调用点**

在 `runIteration` 中，`buildMessages()` 调用需要传入 query。由于 `runIteration` 没有用户输入参数，需要从 history 中取最后一条 user 消息。

修改 `runIteration` 中的调用：

```go
	// Build messages including system prompt.
	var lastUserMsg string
	for i := len(a.history) - 1; i >= 0; i-- {
		if a.history[i].Role == client.RoleUser {
			lastUserMsg = a.history[i].Content
			break
		}
	}
	msgs := a.buildMessages(lastUserMsg)
```

注意：这段代码在 `a.mu.Lock()` 之前执行，但 `a.history` 的读取需要在锁内。由于 `buildMessages` 内部已经加锁，这里需要把 lastUserMsg 的提取放到 `buildMessages` 内部，或者在 `runIteration` 中加锁读取。

更简洁的做法：把 query 提取移到 `buildMessages` 内部：

```go
func (a *Agent) buildMessages(query string) []client.Message {
```

调用处传入空字符串即可，`buildMessages` 内部从 history 取最后一条 user 消息：

```go
func (a *Agent) buildMessages(userQuery string) []client.Message {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If no query provided, extract from history.
	if userQuery == "" {
		for i := len(a.history) - 1; i >= 0; i-- {
			if a.history[i].Role == client.RoleUser {
				userQuery = a.history[i].Content
				break
			}
		}
	}

	// ... rest of the method
```

然后 `runIteration` 中的调用改为：

```go
	msgs := a.buildMessages("")
```

- [ ] **Step 7: 修改 runtime.go — 传递 Classifier**

在 `runtime/runtime.go` 的 `New` 函数中，创建 Classifier 并传给 Agent：

```go
	// Create memory classifier.
	var classifier *memory.Classifier
	if memManager != nil {
		classifier = memory.NewClassifier(llm)
	}

	ag := agent.New(llm, cfg.ToolRegistry, agentCfg, memManager, classifier)
```

- [ ] **Step 8: 更新 manager.go 的 Memorize 方法**

`Memorize` 需要适配新 Fact 的 Category 和 Confidence 字段。当前实现只用 `Key` 和 `Content`，Store 调用会自动处理新字段（Task 3 已更新 Store）。但 `working.Remember` 的 key 可以用 Category：

```go
func (m *DefaultManager) Memorize(ctx context.Context, fact Fact) error {
	// Use category as key prefix for working memory.
	key := string(fact.Category)
	if fact.Key != "" {
		key = fact.Key
	}
	m.working.Remember(key, fact.Content)

	if m.longTerm != nil {
		if err := m.longTerm.Store(ctx, fact); err != nil {
			slog.Debug("failed to store fact in long-term memory", "error", err, "key", fact.Key)
			return fmt.Errorf("long-term storage failed: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 9: 验证编译**

Run: `go build ./...`
Expected: 编译通过

- [ ] **Step 10: 运行全部测试**

Run: `go test ./... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 11: Commit**

```bash
git add agent/agent.go runtime/runtime.go memory/manager.go
git commit -m "feat: integrate Classifier into agent pipeline with hybrid retrieval"
```

---

### Task 7: 端到端验证

- [ ] **Step 1: 执行 FTS 迁移**

Run: `psql -U FengXuan -d FengXuan -f migrations/002_add_fts.sql`
Expected: 成功

- [ ] **Step 2: 运行完整测试套件**

Run: `go test ./... -short -count=1`
Expected: 所有测试通过

- [ ] **Step 3: 手动验证**

启动 TUI，执行以下操作：
1. 说"我喜欢Go语言" → 验证 PostgreSQL 中写入 `category=preference` 的事实
2. 说"帮我查天气" → 验证不存储（should_store=false）
3. 说"记住我的邮箱是test@example.com" → 验证写入 `category=explicit`
4. 说"服务器是Ubuntu 22.04" → 验证写入 `category=environment`
5. 新会话中问"我喜欢什么语言" → 验证能检索到之前的偏好

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(memory): Hermes-style long-term memory optimization"
```
