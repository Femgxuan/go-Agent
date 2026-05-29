# Hybrid Episodic Retrieval Design

**Date**: 2026-05-29
**Status**: Draft

## Problem

当前 episodes 搜索使用 ILIKE 关键词匹配，无法进行语义搜索。需要引入 embedding 向量检索，结合 FTS 关键词匹配和向量语义搜索，通过 RRF 算法融合排序，提供统一的记忆检索服务。

同时需要支持多种 embedding 提供者，包括 HuggingFace 和 ModelScope 的云端 API。

## Architecture

### 记忆架构总览

| 存储 | 用途 | 搜索方式 | 状态 |
|------|------|---------|------|
| SOUL/MEMORY/USER.md | 精炼知识（热记忆） | 每次会话注入系统提示 | 活跃 |
| episodes | 对话历史 | FTS + 向量混合检索 (RRF) | 活跃，本次重点 |
| facts | 早期结构化知识 | ILIKE | 弃用，不再写入 |

### 混合检索流程

```
用户查询
  │
  ├─── FTS 路径 ──→ tsvector @@ plainto_tsquery ──→ FTS 结果（按 rank 排序）
  │
  ├─── Vector 路径 ──→ Embed(query) → pgvector cosine ──→ Vector 结果（按 similarity 排序）
  │
  └─── RRF 融合 ──→ fts_weight/(k+rank) + vector_weight/(k+rank) ──→ 最终排序
```

降级策略：
- 无 embedder 或无 embedding 列 → 只走 FTS
- FTS 无结果 + vector 有结果 → 返回 vector 结果
- 两者都有 → RRF 融合

### Embedding 提供者

| Provider | 默认 base_url | 说明 |
|----------|--------------|------|
| openai | `https://api.openai.com/v1/embeddings` | 需要 API key |
| huggingface | `https://api-inference.huggingface.co/models/{model}/embeddings` | 需要 HF token |
| modelscope | `https://api-inference.modelscope.cn/v1/embeddings` | 需要 ModelScope token |
| ollama | `http://localhost:11434/api/embeddings` | 本地 Ollama 服务 |
| hash | N/A | 测试用，无语义能力 |

`huggingface` 和 `modelscope` 复用 `OpenAIEmbedder`（API 格式兼容），只改默认 `base_url`。

## Changes

### 1. Config 配置扩展

```yaml
memory:
  long_term:
    postgres_url: "postgres://..."
    embedder:
      provider: "huggingface"           # openai | ollama | huggingface | modelscope | hash
      api_key: "hf_xxxxx"               # HF token / OpenAI key / ModelScope token
      model: "BAAI/bge-small-zh-v1.5"
      base_url: ""                      # 自定义端点（留空用默认）
    hybrid:
      fts_weight: 0.3                   # FTS 关键词权重
      vector_weight: 0.7                # 向量语义权重
      rrf_k: 60                         # RRF 平滑常数
```

config.go 变更：
- `LongTerm.Embedder.Provider` 增加 `"huggingface"` 和 `"modelscope"` 枚举值
- 新增 `LongTerm.Hybrid` 结构体（`FTSWeight`、`VectorWeight`、`RRFK`）
- 新增环境变量：`MEMORY_HYBRID_FTS_WEIGHT`、`MEMORY_HYBRID_VECTOR_WEIGHT`、`MEMORY_HYBRID_RRF_K`
- 默认值：`fts_weight=0.3`、`vector_weight=0.7`、`rrf_k=60`

### 2. Embedding 层变更

embedding.go：
- `OpenAIEmbedder` 新增 `baseURL` 字段，`Embed()` 使用 `e.baseURL` 替代硬编码 URL
- `Dimension()` 增加常见 HF 模型维度映射（`bge-small-zh-v1.5` → 512，`bge-base-zh-v1.5` → 768，`bge-large-zh-v1.5` → 1024）

manager.go：
- `NewManager()` 初始化 embedder 时新增 `huggingface` 和 `modelscope` 分支
- 传入 `hybrid` 配置到 `PgEpisodicStore`

### 3. 数据库 Schema 变更

新增 `migrations/004_hybrid_episodes.sql`：

```sql
-- 1. episodes 表新增 embedding 列
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- 2. HNSW 索引（替代 IVFFlat）
CREATE INDEX IF NOT EXISTS idx_episodes_embedding_hnsw
  ON episodes USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);

-- 3. 确保 tsvector + GIN 索引
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS fts_vector tsvector;
CREATE INDEX IF NOT EXISTS idx_episodes_fts ON episodes USING gin(fts_vector);

-- 4. 触发器：自动更新 tsvector
CREATE OR REPLACE FUNCTION episodes_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tsvector_update ON episodes;
CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON episodes
    FOR EACH ROW EXECUTE FUNCTION episodes_fts_trigger();

-- 5. 回填现有 tsvector
UPDATE episodes SET fts_vector = to_tsvector('simple', COALESCE(content, ''))
WHERE fts_vector IS NULL;
```

### 4. Episodic Store 重构

episodic.go 变更：

**PgEpisodicStore 新增字段：**
- `embedder Embedder`
- `hasEmbedding bool`
- `ftsWeight float64`
- `vectorWeight float64`
- `rrfK int`

**SaveSession() 改造：**
- 写入 episodes 时同步生成 embedding
- embedding 生成失败不阻塞存储，降级为纯文本

**Search() 重构为双路并行 + RRF：**
- 路径 1：`ftsSearch()` — 使用 `tsvector @@ plainto_tsquery`（替换 ILIKE）
- 路径 2：`vectorSearch()` — 使用 `pgvector cosine` 距离
- 融合：`rrfMerge()` — RRF 算法

**新增方法：**
- `ftsSearch(ctx, query, limit) []Episode` — tsvector 全文检索
- `vectorSearch(ctx, query, limit, minScore) []Episode` — pgvector 语义检索
- `rrfMerge(ftsResults, vecResults, ftsWeight, vectorWeight, k, limit) []Episode` — RRF 融合

**RRF 算法：**
```
score(doc) = Σ weight_i / (k + rank_i(doc))
```
其中 `k=60`（标准值），`weight_1=0.3`（FTS），`weight_2=0.7`（Vector）。

### 5. Facts 表弃用

- `manager.go`：`Memorize()` 不再写入 facts 表
- `longterm.go`：保留代码但不主动使用
- `session_search` 工具：改为调 episodic 的混合检索

### 6. 回填现有 episodes 的 embedding

需要一个回填任务，为已有的 episodes 补充 embedding：
- 在 `NewPgEpisodicStore()` 初始化时检查是否有无 embedding 的 episodes
- 异步回填（不阻塞启动）
- 或提供 CLI 命令手动触发

## Files Modified

| File | Change |
|------|--------|
| `memory/config.go` | 新增 `Hybrid` 配置块、`huggingface`/`modelscope` 环境变量 |
| `memory/embedding.go` | `OpenAIEmbedder` 加 `baseURL` 字段，`Dimension()` 加 HF 模型映射 |
| `memory/episodic.go` | 重构 Search 为双路+RRF；ftsSearch 改用 tsvector；新增 vectorSearch、rrfMerge；SaveSession 加 embedding |
| `memory/manager.go` | 新增 `huggingface`/`modelscope` 分支，传入 hybrid 配置 |
| `migrations/004_hybrid_episodes.sql` | 新增：embedding 列、HNSW 索引、fts_vector、触发器 |

## Files Unchanged

| File | Reason |
|------|--------|
| `memory/longterm.go` | facts 表弃用，保留代码但不改动 |
| `agent/agent.go` | storeInteraction 流程不变 |
| `tools/session_tool.go` | session_search 工具接口不变，底层自动使用混合检索 |
| `runtime/runtime.go` | 无变更 |

## Testing

| Test | Coverage |
|------|----------|
| `TestRRFMerge` | 验证 RRF 融合排序正确性 |
| `TestRRFMerge_EmptyInputs` | 空输入边界情况 |
| `TestFTSFallback` | 无 embedder 时降级到纯 FTS |
| `TestHybridSearch` | FTS + Vector 双路召回 + RRF 融合 |
| `TestSaveSession_WithEmbedding` | 保存会话时生成 embedding |
| `TestSaveSession_EmbeddingFailure` | embedding 失败不阻塞存储 |
| `TestHuggingFaceEmbedder` | HF API 调用（mock HTTP） |
| `TestConfigHybrid` | hybrid 配置加载和默认值 |
