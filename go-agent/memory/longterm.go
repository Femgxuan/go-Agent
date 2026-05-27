package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// PgLongTermMemory is a pgvector-backed implementation of LongTermMemory.
type PgLongTermMemory struct {
	mu       sync.RWMutex
	pool     *pgxpool.Pool
	embedder Embedder
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
func NewPgLongTermMemory(pool *pgxpool.Pool, embedder Embedder) *PgLongTermMemory {
	return &PgLongTermMemory{
		pool:     pool,
		embedder: embedder,
	}
}

// Store stores a Fact, generating an embedding vector.
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

// Search performs hybrid FTS + vector search.
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if query == "" {
		return p.recent(ctx, limit), nil
	}

	ftsResults := p.ftsSearch(ctx, query, limit)
	vecResults := p.vectorSearch(ctx, query, limit, minScore)
	merged := p.mergeResults(ftsResults, vecResults, limit)

	return merged, nil
}

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
