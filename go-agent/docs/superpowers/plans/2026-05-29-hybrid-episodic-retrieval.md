# Hybrid Episodic Retrieval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement FTS + pgvector dual-path hybrid retrieval with RRF fusion for the episodes table, supporting HuggingFace/ModelScope cloud embedding APIs.

**Architecture:** Episodes table gets embedding column + HNSW index. Search does parallel FTS (tsvector) and vector (cosine) retrieval, merged via RRF with configurable weights. HuggingFace/ModelScope reuse OpenAIEmbedder with different base URLs.

**Tech Stack:** Go, PostgreSQL, pgvector, tsvector/GIN, HNSW

---

## File Structure

| File | Responsibility | Change Type |
|------|---------------|-------------|
| `memory/config.go` | Hybrid config block, new provider env vars | Modify |
| `memory/embedding.go` | OpenAIEmbedder baseURL, HF model dimensions | Modify |
| `memory/episodic.go` | Hybrid search (FTS+Vector+RRF), embedding in SaveSession | Modify |
| `memory/manager.go` | Pass embedder+hybrid config to episodic store | Modify |
| `migrations/004_hybrid_episodes.sql` | embedding column, HNSW, tsvector, trigger | Create |
| `memory/episodic_test.go` | RRF, FTS, hybrid, embedding tests | Create |

---

### Task 1: Add Hybrid Config Block

**Files:**
- Modify: `memory/config.go`

- [ ] **Step 1: Add Hybrid struct to Config.LongTerm**

In `memory/config.go`, add the `Hybrid` struct inside `LongTerm` after the `Embedder` struct (around line 33):

```go
Hybrid struct {
    FTSWeight    float64 `yaml:"fts_weight" env:"MEMORY_HYBRID_FTS_WEIGHT"`
    VectorWeight float64 `yaml:"vector_weight" env:"MEMORY_HYBRID_VECTOR_WEIGHT"`
    RRFK         int     `yaml:"rrf_k" env:"MEMORY_HYBRID_RRF_K"`
} `yaml:"hybrid"`
```

- [ ] **Step 2: Set defaults in DefaultConfig()**

After the existing `cfg.LongTerm.HalfLife` line (around line 89), add:

```go
cfg.LongTerm.Hybrid.FTSWeight = 0.3
cfg.LongTerm.Hybrid.VectorWeight = 0.7
cfg.LongTerm.Hybrid.RRFK = 60
```

- [ ] **Step 3: Add env overrides in ApplyEnvOverrides()**

After the existing `MEMORY_EMBEDDER_BASE_URL` override (around line 143), add:

```go
if v := os.Getenv("MEMORY_HYBRID_FTS_WEIGHT"); v != "" {
    if f, err := strconv.ParseFloat(v, 64); err == nil {
        cfg.LongTerm.Hybrid.FTSWeight = f
    }
}
if v := os.Getenv("MEMORY_HYBRID_VECTOR_WEIGHT"); v != "" {
    if f, err := strconv.ParseFloat(v, 64); err == nil {
        cfg.LongTerm.Hybrid.VectorWeight = f
    }
}
if v := os.Getenv("MEMORY_HYBRID_RRF_K"); v != "" {
    if n, err := strconv.Atoi(v); err == nil {
        cfg.LongTerm.Hybrid.RRFK = n
    }
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add memory/config.go
git commit -m "feat(memory): add hybrid retrieval config block with RRF weights"
```

---

### Task 2: Extend OpenAIEmbedder with baseURL and HF Dimensions

**Files:**
- Modify: `memory/embedding.go`

- [ ] **Step 1: Add baseURL field to OpenAIEmbedder**

Change the struct (around line 29):

```go
type OpenAIEmbedder struct {
    apiKey  string
    model   string
    baseURL string
    client  *http.Client
}
```

- [ ] **Step 2: Update NewOpenAIEmbedder to accept baseURL**

```go
func NewOpenAIEmbedder(apiKey, model, baseURL string) *OpenAIEmbedder {
    if baseURL == "" {
        baseURL = "https://api.openai.com/v1/embeddings"
    }
    return &OpenAIEmbedder{
        apiKey:  apiKey,
        model:   model,
        baseURL: baseURL,
        client:  &http.Client{Timeout: 30 * time.Second},
    }
}
```

- [ ] **Step 3: Use e.baseURL in EmbedBatch**

In `EmbedBatch()`, change the hardcoded URL (around line 65):

```go
req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL, bytes.NewReader(jsonData))
```

- [ ] **Step 4: Add HF model dimensions to Dimension()**

Replace the `Dimension()` method (around line 107):

```go
func (e *OpenAIEmbedder) Dimension() int {
    switch e.model {
    case "text-embedding-3-small":
        return 1536
    case "text-embedding-3-large":
        return 3072
    case "BAAI/bge-small-zh-v1.5":
        return 512
    case "BAAI/bge-base-zh-v1.5":
        return 768
    case "BAAI/bge-large-zh-v1.5":
        return 1024
    default:
        return 1536
    }
}
```

- [ ] **Step 5: Update all existing NewOpenAIEmbedder call sites**

In `memory/manager.go` around line 119, update:

```go
embedder = NewOpenAIEmbedder(cfg.LongTerm.Embedder.APIKey, cfg.LongTerm.Embedder.Model, "")
```

Search for any other call sites and update similarly.

- [ ] **Step 6: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add memory/embedding.go memory/manager.go
git commit -m "feat(memory): add baseURL to OpenAIEmbedder, support HF model dimensions"
```

---

### Task 3: Add HuggingFace/ModelScope Provider Support

**Files:**
- Modify: `memory/manager.go`

- [ ] **Step 1: Add huggingface and modelscope cases in NewManager()**

In the `switch cfg.LongTerm.Embedder.Provider` block (around line 103), add before the `default` case:

```go
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
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add memory/manager.go
git commit -m "feat(memory): add huggingface and modelscope embedding providers"
```

---

### Task 4: Create Migration SQL

**Files:**
- Create: `migrations/004_hybrid_episodes.sql`

- [ ] **Step 1: Write the migration file**

```sql
-- migrations/004_hybrid_episodes.sql
-- Hybrid retrieval: embedding + HNSW + tsvector for episodes

-- 1. Ensure pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- 2. Add embedding column to episodes
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- 3. Create HNSW index for vector search
CREATE INDEX IF NOT EXISTS idx_episodes_embedding_hnsw
  ON episodes USING hnsw (embedding vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);

-- 4. Ensure tsvector column exists
ALTER TABLE episodes ADD COLUMN IF NOT EXISTS fts_vector tsvector;

-- 5. Create GIN index for FTS
CREATE INDEX IF NOT EXISTS idx_episodes_fts_new ON episodes USING gin(fts_vector);

-- 6. Trigger function: auto-update tsvector on insert/update
CREATE OR REPLACE FUNCTION episodes_fts_trigger() RETURNS trigger AS $$
BEGIN
    NEW.fts_vector := to_tsvector('simple', COALESCE(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tsvector_update ON episodes;
CREATE TRIGGER tsvector_update BEFORE INSERT OR UPDATE ON episodes
    FOR EACH ROW EXECUTE FUNCTION episodes_fts_trigger();

-- 7. Backfill existing rows
UPDATE episodes SET fts_vector = to_tsvector('simple', COALESCE(content, ''))
WHERE fts_vector IS NULL;
```

- [ ] **Step 2: Run migration in Docker**

```bash
docker exec -i go-agent-postgres psql -U FengXuan -d FengXuan < migrations/004_hybrid_episodes.sql
```

- [ ] **Step 3: Verify columns exist**

```bash
docker exec -it go-agent-postgres psql -U FengXuan -d FengXuan -c "\d episodes"
```

Expected: `embedding vector(1536)` and `fts_vector tsvector` columns visible.

- [ ] **Step 4: Commit**

```bash
git add migrations/004_hybrid_episodes.sql
git commit -m "feat(migration): add embedding, HNSW, tsvector to episodes table"
```

---

### Task 5: Refactor PgEpisodicStore for Hybrid Search

**Files:**
- Modify: `memory/episodic.go`

This is the core task. Add embedder and hybrid config fields, refactor Search, add FTS/Vector/RRF methods, update SaveSession.

- [ ] **Step 1: Add new fields to PgEpisodicStore**

```go
type PgEpisodicStore struct {
    pool         *pgxpool.Pool
    summarizer   Summarizer
    tokenizer    Tokenizer
    embedder     Embedder
    hasEmbedding bool
    ftsWeight    float64
    vectorWeight float64
    rrfK         int
}
```

- [ ] **Step 2: Update NewPgEpisodicStore signature**

```go
func NewPgEpisodicStore(pool *pgxpool.Pool, summarizer Summarizer, tokenizer Tokenizer, embedder Embedder, ftsWeight, vectorWeight float64, rrfK int) *PgEpisodicStore {
    s := &PgEpisodicStore{
    pool:         pool,
    summarizer:   summarizer,
    tokenizer:    tokenizer,
    embedder:     embedder,
    ftsWeight:    ftsWeight,
    vectorWeight: vectorWeight,
    rrfK:         rrfK,
    }

    // Check if embedding column exists
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    var colExists bool
    err := pool.QueryRow(ctx,
        "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='episodes' AND column_name='embedding')",
    ).Scan(&colExists)
    if err == nil && colExists {
        s.hasEmbedding = true
        slog.Info("[episodic] embedding column detected, hybrid search enabled")
    } else {
        slog.Info("[episodic] no embedding column, FTS-only mode")
    }

    return s
}
```

- [ ] **Step 3: Rewrite Search() for dual-path + RRF**

Replace the existing `Search()` method entirely:

```go
func (s *PgEpisodicStore) Search(ctx context.Context, query string, limit int) ([]Episode, error) {
    if limit <= 0 {
        limit = 5
    }

    // Path 1: FTS keyword search (always available)
    ftsResults := s.ftsSearch(ctx, query, limit*2)

    // Path 2: Vector semantic search (needs embedding column + embedder)
    var vecResults []Episode
    if s.hasEmbedding && s.embedder != nil {
        vecResults = s.vectorSearch(ctx, query, limit*2, 0.3)
    }

    // No vector capability: return FTS results only
    if len(vecResults) == 0 {
        if len(ftsResults) > limit {
            ftsResults = ftsResults[:limit]
        }
        return ftsResults, nil
    }

    // RRF fusion
    merged := s.rrfMerge(ftsResults, vecResults, limit)
    return merged, nil
}
```

- [ ] **Step 4: Add ftsSearch() method**

```go
func (s *PgEpisodicStore) ftsSearch(ctx context.Context, query string, limit int) []Episode {
    rows, err := s.pool.Query(ctx, `
        SELECT id, session_id, role, content, created_at, token_count
        FROM episodes
        WHERE fts_vector @@ plainto_tsquery('simple', $1)
        ORDER BY ts_rank(fts_vector, plainto_tsquery('simple', $1)) DESC
        LIMIT $2
    `, query, limit)
    if err != nil {
        slog.Warn("[episodic] fts search failed", "error", err)
        return nil
    }
    defer rows.Close()

    var episodes []Episode
    for rows.Next() {
        var ep Episode
        if err := rows.Scan(&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.CreatedAt, &ep.TokenCount); err != nil {
            continue
        }
        episodes = append(episodes, ep)
    }
    return episodes
}
```

- [ ] **Step 5: Add vectorSearch() method**

```go
func (s *PgEpisodicStore) vectorSearch(ctx context.Context, query string, limit int, minScore float64) []Episode {
    embedding, err := s.embedder.Embed(ctx, query)
    if err != nil {
        slog.Warn("[episodic] embedding generation failed", "error", err)
        return nil
    }

    rows, err := s.pool.Query(ctx, `
        SELECT id, session_id, role, content, created_at, token_count,
               1 - (embedding <=> $1) as similarity
        FROM episodes
        WHERE 1 - (embedding <=> $1) > $2
        ORDER BY embedding <=> $1
        LIMIT $3
    `, pgvector.NewVector(toFloat32(embedding)), minScore, limit)
    if err != nil {
        slog.Warn("[episodic] vector search failed", "error", err)
        return nil
    }
    defer rows.Close()

    var episodes []Episode
    for rows.Next() {
        var ep Episode
        var similarity float64
        if err := rows.Scan(&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.CreatedAt, &ep.TokenCount, &similarity); err != nil {
            continue
        }
        episodes = append(episodes, ep)
    }
    return episodes
}
```

- [ ] **Step 6: Add rrfMerge() method**

```go
func (s *PgEpisodicStore) rrfMerge(ftsResults, vecResults []Episode, limit int) []Episode {
    scores := make(map[string]float64)
    episodeMap := make(map[string]Episode)

    k := s.rrfK
    if k <= 0 {
        k = 60
    }

    for rank, ep := range ftsResults {
        scores[ep.ID] += s.ftsWeight / (float64(k) + float64(rank+1))
        episodeMap[ep.ID] = ep
    }

    for rank, ep := range vecResults {
        scores[ep.ID] += s.vectorWeight / (float64(k) + float64(rank+1))
        if _, exists := episodeMap[ep.ID]; !exists {
            episodeMap[ep.ID] = ep
        }
    }

    type scored struct {
        episode Episode
        score   float64
    }
    var sorted []scored
    for id, score := range scores {
        sorted = append(sorted, scored{episodeMap[id], score})
    }
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].score > sorted[j].score
    })

    result := make([]Episode, 0, limit)
    for i, se := range sorted {
        if i >= limit {
            break
        }
        result = append(result, se.episode)
    }
    return result
}
```

- [ ] **Step 7: Add required imports**

Add to the import block:

```go
import (
    "sort"
    "github.com/pgvector/pgvector-go"
)
```

Remove unused `strings` import if `splitKeywords` is no longer called.

- [ ] **Step 8: Remove old Search/splitKeywords code**

Delete the old `Search()` method and `splitKeywords()` function (they are replaced by the new methods above).

- [ ] **Step 9: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add memory/episodic.go
git commit -m "feat(memory): implement hybrid FTS+Vector search with RRF fusion"
```

---

### Task 6: Update SaveSession to Generate Embeddings

**Files:**
- Modify: `memory/episodic.go`

- [ ] **Step 1: Rewrite SaveSession() to include embedding**

Replace the existing `SaveSession()` method:

```go
func (s *PgEpisodicStore) SaveSession(ctx context.Context, session Session) error {
    return s.retryWithJitter(3, 20*time.Millisecond, 150*time.Millisecond, func() error {
        tx, err := s.pool.Begin(ctx)
        if err != nil {
            return fmt.Errorf("begin tx: %w", err)
        }
        defer tx.Rollback(ctx)

        _, err = tx.Exec(ctx, `
            INSERT INTO sessions (id, title, source, created_at, parent_id)
            VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title
        `, session.ID, session.Title, session.Source, session.CreatedAt, nil)
        if err != nil {
            return fmt.Errorf("insert session: %w", err)
        }

        for _, ep := range session.Episodes {
            tokenCount := 0
            if s.tokenizer != nil {
                tokenCount = s.tokenizer.Count(ep.Content)
            }

            // Generate embedding if embedder available
            var embedding pgvector.Vector
            hasEmbedding := false
            if s.embedder != nil && s.hasEmbedding {
                vec, err := s.embedder.Embed(ctx, ep.Content)
                if err != nil {
                    slog.Warn("[episodic] embedding failed, storing without vector", "error", err, "episode_id", ep.ID)
                } else {
                    embedding = pgvector.NewVector(toFloat32(vec))
                    hasEmbedding = true
                }
            }

            if hasEmbedding {
                _, err = tx.Exec(ctx, `
                    INSERT INTO episodes (id, session_id, role, content, created_at, token_count, embedding)
                    VALUES ($1, $2, $3, $4, $5, $6, $7)
                    ON CONFLICT (id) DO NOTHING
                `, ep.ID, session.ID, ep.Role, ep.Content, ep.CreatedAt, tokenCount, embedding)
            } else {
                _, err = tx.Exec(ctx, `
                    INSERT INTO episodes (id, session_id, role, content, created_at, token_count)
                    VALUES ($1, $2, $3, $4, $5, $6)
                    ON CONFLICT (id) DO NOTHING
                `, ep.ID, session.ID, ep.Role, ep.Content, ep.CreatedAt, tokenCount)
            }
            if err != nil {
                return fmt.Errorf("insert episode: %w", err)
            }
        }

        return tx.Commit(ctx)
    })
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add memory/episodic.go
git commit -m "feat(memory): generate embeddings in SaveSession with graceful degradation"
```

---

### Task 7: Wire Embedder + Hybrid Config into Episodic Store

**Files:**
- Modify: `memory/manager.go`

- [ ] **Step 1: Update NewPgEpisodicStore call in NewManager()**

Change (around line 131):

```go
// Before:
episodic = NewPgEpisodicStore(pool, nil, tokenizer)

// After:
episodic = NewPgEpisodicStore(pool, nil, tokenizer, embedder,
    cfg.LongTerm.Hybrid.FTSWeight, cfg.LongTerm.Hybrid.VectorWeight, cfg.LongTerm.Hybrid.RRFK)
```

Note: `embedder` may be nil if no provider is configured. `NewPgEpisodicStore` handles this gracefully.

- [ ] **Step 2: Move embedder creation before episodic store creation**

Currently the embedder is created inside the `if cfg.LongTerm.PostgresURL != ""` block alongside `longTerm`. The episodic store creation (line 128-133) is outside this block but needs the embedder. Restructure so the embedder variable is accessible:

```go
// Create embedder (may be nil if no PG or no provider configured)
var embedder Embedder
var pool *pgxpool.Pool
if cfg.LongTerm.PostgresURL != "" {
    // ... existing PG connection code ...

    // ... existing embedder switch code ...
}

// Create episodic store.
var episodic EpisodicStore
if cfg.Episodic.Enabled && pool != nil {
    episodic = NewPgEpisodicStore(pool, nil, tokenizer, embedder,
        cfg.LongTerm.Hybrid.FTSWeight, cfg.LongTerm.Hybrid.VectorWeight, cfg.LongTerm.Hybrid.RRFK)
    slog.Info("episodic memory enabled")
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add memory/manager.go
git commit -m "feat(memory): wire embedder and hybrid config into episodic store"
```

---

### Task 8: Write Unit Tests

**Files:**
- Create: `memory/episodic_test.go`

- [ ] **Step 1: Test RRF merge algorithm**

```go
package memory

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func makeEpisodes(ids ...string) []Episode {
    var eps []Episode
    for _, id := range ids {
        eps = append(eps, Episode{ID: id, Content: "content-" + id})
    }
    return eps
}

func TestRRFMerge(t *testing.T) {
    store := &PgEpisodicStore{
        ftsWeight:    0.3,
        vectorWeight: 0.7,
        rrfK:         60,
    }

    fts := makeEpisodes("a", "b", "c")    // a ranked 1st, b 2nd, c 3rd
    vec := makeEpisodes("b", "d", "a")    // b ranked 1st, d 2nd, a 3rd

    result := store.rrfMerge(fts, vec, 4)

    ids := make([]string, len(result))
    for i, ep := range result {
        ids[i] = ep.ID
    }

    // a: 0.3/61 + 0.7/63 = 0.004918 + 0.011111 = 0.016029
    // b: 0.3/62 + 0.7/61 = 0.004839 + 0.011475 = 0.016314
    // c: 0.3/63 = 0.004762
    // d: 0.7/62 = 0.011290
    // Expected order: b, a, d, c
    assert.Equal(t, "b", ids[0])
    assert.Equal(t, "a", ids[1])
    assert.Equal(t, "d", ids[2])
    assert.Equal(t, "c", ids[3])
}

func TestRRFMerge_EmptyInputs(t *testing.T) {
    store := &PgEpisodicStore{ftsWeight: 0.3, vectorWeight: 0.7, rrfK: 60}

    // Both empty
    result := store.rrfMerge(nil, nil, 5)
    assert.Empty(t, result)

    // Only FTS
    fts := makeEpisodes("x", "y")
    result = store.rrfMerge(fts, nil, 5)
    assert.Len(t, result, 2)

    // Only Vector
    result = store.rrfMerge(nil, makeEpisodes("z"), 5)
    assert.Len(t, result, 1)
}

func TestRRFMerge_LimitRespected(t *testing.T) {
    store := &PgEpisodicStore{ftsWeight: 0.3, vectorWeight: 0.7, rrfK: 60}

    fts := makeEpisodes("a", "b", "c", "d", "e")
    vec := makeEpisodes("f", "g", "h")

    result := store.rrfMerge(fts, vec, 3)
    assert.Len(t, result, 3)
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./memory/ -run TestRRF -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add memory/episodic_test.go
git commit -m "test(memory): add RRF merge unit tests"
```

---

### Task 9: Deprecate Facts Table

**Files:**
- Modify: `memory/manager.go`

- [ ] **Step 1: Disable Memorize() writes to facts**

In `manager.go`, find the `Memorize()` method and comment out or guard the `longTerm.Store()` call:

```go
func (m *DefaultManager) Memorize(ctx context.Context, fact Fact) error {
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
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add memory/manager.go
git commit -m "refactor(memory): deprecate facts table, stop writing to long-term storage"
```

---

### Task 10: End-to-End Verification

- [ ] **Step 1: Run all tests**

Run: `go test ./... -count=1`
Expected: All pass (except PG integration test if no PG)

- [ ] **Step 2: Start Docker PostgreSQL**

```bash
docker compose up -d postgres
```

- [ ] **Step 3: Run migration**

```bash
docker exec -i go-agent-postgres psql -U FengXuan -d FengXuan < migrations/004_hybrid_episodes.sql
```

- [ ] **Step 4: Verify columns and indexes**

```bash
docker exec -it go-agent-postgres psql -U FengXuan -d FengXuan -c "\d episodes"
```

Expected: `embedding vector(1536)`, `fts_vector tsvector` columns, HNSW and GIN indexes.

- [ ] **Step 5: Start agent and have a conversation**

```bash
go run .
```

Have a few conversations, then verify episodes are stored with embeddings:

```bash
docker exec -it go-agent-postgres psql -U FengXuan -d FengXuan -c "SELECT id, role, LEFT(content, 40), embedding IS NOT NULL as has_emb FROM episodes ORDER BY created_at DESC LIMIT 5;"
```

- [ ] **Step 6: Test session_search**

In the agent, ask: "我们之前聊过什么"

Expected: Agent calls session_search, finds relevant episodes via hybrid search.

- [ ] **Step 7: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: end-to-end verification fixes"
```
