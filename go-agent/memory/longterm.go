package memory

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// PgLongTermMemory is a pgvector-backed implementation of LongTermMemory.
type PgLongTermMemory struct {
	mu           sync.RWMutex
	pool         *pgxpool.Pool
	embedder     Embedder
	hasEmbedding bool // true if the embedding column exists (pgvector installed)
}

// NewPgPool creates a new PostgreSQL connection pool.
func NewPgPool(ctx context.Context, connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}

// NewPgLongTermMemory creates a new PgLongTermMemory.
// It checks whether the embedding column exists (pgvector installed) and degrades gracefully.
func NewPgLongTermMemory(pool *pgxpool.Pool, embedder Embedder) *PgLongTermMemory {
	ltm := &PgLongTermMemory{
		pool:     pool,
		embedder: embedder,
	}

	// Check if embedding column exists.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var colExists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='facts' AND column_name='embedding')",
	).Scan(&colExists)
	if err == nil && colExists {
		ltm.hasEmbedding = true
	} else {
		slog.Info("embedding column not found, using FTS-only mode")
	}

	return ltm
}

// Store stores a Fact, generating an embedding vector if pgvector is available.
func (p *PgLongTermMemory) Store(ctx context.Context, fact Fact) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hasEmbedding {
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

	// FTS-only mode: no embedding column.
	_, err := p.pool.Exec(ctx, `
		INSERT INTO facts (id, category, key, content, source, confidence, decay_score, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			category = EXCLUDED.category,
			key = EXCLUDED.key,
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			confidence = EXCLUDED.confidence,
			decay_score = EXCLUDED.decay_score,
			updated_at = EXCLUDED.updated_at
	`, fact.ID, string(fact.Category), fact.Key, fact.Content, fact.Source, fact.Confidence,
		fact.DecayScore, fact.CreatedAt, time.Now())

	if err != nil {
		return fmt.Errorf("inserting fact: %w", err)
	}
	return nil
}

// Search performs hybrid FTS + vector search (or FTS-only if pgvector is unavailable).
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if query == "" {
		return p.recent(ctx, limit), nil
	}

	ftsResults := p.ftsSearch(ctx, query, limit)

	if !p.hasEmbedding {
		return ftsResults, nil
	}

	vecResults := p.vectorSearch(ctx, query, limit, minScore)
	merged := p.mergeResults(ftsResults, vecResults, limit)

	return merged, nil
}

// cjkStopWords are single CJK characters that are grammatical particles
// and should not be used as search keywords.
var cjkStopWords = map[rune]bool{
	'的': true, '了': true, '吗': true, '呢': true, '吧': true, '啊': true,
	'是': true, '在': true, '有': true, '和': true, '与': true, '或': true,
	'但': true, '而': true, '就': true, '都': true, '也': true, '还': true,
	'只': true, '又': true, '再': true, '把': true, '被': true, '让': true,
	'给': true, '向': true, '从': true, '到': true, '对': true, '为': true,
	'我': true, '你': true, '他': true, '她': true, '它': true,
	'么': true, '什': true, '哪': true, '怎': true, '多': true, '几': true,
	'一': true, '不': true, '没': true, '很': true, '太': true, '最': true,
}

// isCJK returns true if the rune is a CJK unified ideograph.
func isCJK(r rune) bool {
	return (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf)
}

// extractKeywords extracts searchable keywords from a query string.
// For CJK text (no spaces), it removes single-character stop words then
// generates 2-rune sliding windows to capture meaningful substrings.
// For space-separated text, it splits on whitespace and punctuation.
func extractKeywords(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	// Split on whitespace and punctuation (Unicode-aware).
	parts := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})

	// Check if remaining text is primarily CJK (no natural word boundaries).
	hasCJK := false
	for _, r := range query {
		if isCJK(r) {
			hasCJK = true
			break
		}
	}

	// For CJK text that came as one big chunk (no spaces), apply sliding window.
	if hasCJK && len(parts) <= 1 {
		text := query
		if len(parts) == 1 {
			text = parts[0]
		}
		runes := []rune(text)

		// Remove single CJK stop words.
		var filtered []rune
		for _, r := range runes {
			if !cjkStopWords[r] {
				filtered = append(filtered, r)
			}
		}

		if len(filtered) < 2 {
			return nil
		}

		// Generate 2-rune sliding windows.
		seen := make(map[string]bool)
		var keywords []string
		for i := 0; i <= len(filtered)-2; i++ {
			kw := string(filtered[i : i+2])
			if !seen[kw] {
				seen[kw] = true
				keywords = append(keywords, kw)
			}
		}
		return keywords
	}

	// For space-separated text (English, mixed), split and filter.
	seen := make(map[string]bool)
	var keywords []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len([]rune(part)) < 2 {
			continue
		}
		if seen[part] {
			continue
		}
		seen[part] = true
		keywords = append(keywords, part)
	}
	return keywords
}

// ftsSearch performs text search using pg_trgm ILIKE (handles CJK text).
// It extracts keywords from the query and builds an OR-based ILIKE search
// so that partial matches work (e.g., "我喜欢吃什么" matches "用户喜欢吃香蕉").
func (p *PgLongTermMemory) ftsSearch(ctx context.Context, query string, limit int) []Fact {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	// Build OR conditions for each keyword.
	var conditions []string
	var args []any
	argIdx := 1
	for _, kw := range keywords {
		conditions = append(conditions,
			fmt.Sprintf("content ILIKE '%%' || $%d || '%%'", argIdx))
		args = append(args, kw)
		argIdx++
	}
	where := strings.Join(conditions, " OR ")

	sql := fmt.Sprintf(`
		SELECT id, key, content, source, category, confidence, decay_score, created_at
		FROM facts
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, limit)

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		slog.Warn("[fts] query failed", "error", err, "keywords", keywords)
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
		f.DecayScore = 1.0
		results = append(results, f)
	}

	slog.Info("[fts] search completed", "query", query, "keywords", keywords, "results", len(results))
	return results
}

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

// mergeResults combines FTS and vector search results, deduplicates by ID,
// and returns top limit results sorted by score descending.
func (p *PgLongTermMemory) mergeResults(ftsResults, vecResults []Fact, limit int) []Fact {
	seen := make(map[string]int)
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

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].DecayScore > merged[j].DecayScore
	})

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}

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

// Update updates a Fact's fields (e.g. decay_score).
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

// Delete deletes a Fact by ID.
func (p *PgLongTermMemory) Delete(ctx context.Context, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, err := p.pool.Exec(ctx, `DELETE FROM facts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting fact: %w", err)
	}

	return nil
}

// toFloat32 converts a []float64 to []float32 for pgvector compatibility.
func toFloat32(src []float64) []float32 {
	dst := make([]float32, len(src))
	for i, v := range src {
		dst[i] = float32(v)
	}
	return dst
}

// Recall retrieves facts relevant to the query using hybrid FTS + vector search.
// This is the SemanticStore interface method (semantic alias for Search).
func (m *PgLongTermMemory) Recall(ctx context.Context, query string, topK int) ([]Fact, error) {
	if topK <= 0 {
		topK = 10
	}
	return m.Search(ctx, query, topK, 0)
}

// Remember stores a fact. SemanticStore interface method (semantic alias for Store).
func (m *PgLongTermMemory) Remember(ctx context.Context, fact Fact) error {
	return m.Store(ctx, fact)
}

// ForgetByFilter deletes facts matching the filter criteria.
func (m *PgLongTermMemory) ForgetByFilter(ctx context.Context, filter ForgetFilter) error {
	if filter.SessionID != nil {
		_, err := m.pool.Exec(ctx, `DELETE FROM facts WHERE source = $1`, string(*filter.SessionID))
		if err != nil {
			return fmt.Errorf("forget by session: %w", err)
		}
	}
	if filter.Before != nil {
		_, err := m.pool.Exec(ctx, `DELETE FROM facts WHERE created_at < $1`, *filter.Before)
		if err != nil {
			return fmt.Errorf("forget by time: %w", err)
		}
	}
	if filter.KeyPattern != nil {
		_, err := m.pool.Exec(ctx, `DELETE FROM facts WHERE key LIKE $1`, "%"+*filter.KeyPattern+"%")
		if err != nil {
			return fmt.Errorf("forget by pattern: %w", err)
		}
	}
	return nil
}
