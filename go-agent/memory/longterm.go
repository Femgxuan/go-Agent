package memory

import (
	"context"
	"fmt"
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

	// Generate embedding vector
	embedding, err := p.embedder.Embed(ctx, fact.Content)
	if err != nil {
		return fmt.Errorf("generating embedding: %w", err)
	}

	// Insert into database
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
	`, fact.ID, fact.Key, fact.Content, fact.Source, pgvector.NewVector(toFloat32(embedding)), fact.DecayScore, fact.CreatedAt, time.Now())

	if err != nil {
		return fmt.Errorf("inserting fact: %w", err)
	}

	return nil
}

// Search performs semantic search for relevant Facts.
func (p *PgLongTermMemory) Search(ctx context.Context, query string, limit int, minScore float64) ([]Fact, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Generate query embedding
	queryEmbedding, err := p.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("generating query embedding: %w", err)
	}

	// Semantic search
	rows, err := p.pool.Query(ctx, `
		SELECT id, key, content, source, decay_score, created_at,
		       1 - (embedding <=> $1) as similarity
		FROM facts
		WHERE 1 - (embedding <=> $1) > $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`, pgvector.NewVector(toFloat32(queryEmbedding)), minScore, limit)

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
		f.DecayScore = similarity // Use actual similarity
		results = append(results, f)
	}

	return results, nil
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
