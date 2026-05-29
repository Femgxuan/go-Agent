package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// EpisodicStore manages cross-session conversation history.
type EpisodicStore interface {
	SaveSession(ctx context.Context, session Session) error
	Search(ctx context.Context, query string, limit int) ([]Episode, error)
	Summarize(ctx context.Context, query string, episodes []Episode) (string, error)
	ListSessions(ctx context.Context, limit int) ([]SessionSummary, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// PgEpisodicStore implements EpisodicStore using PostgreSQL + FTS.
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

// NewPgEpisodicStore creates a new PG-backed episodic store.
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

// SaveSession saves a session and its episodes to PostgreSQL with retry + jitter.
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

// Search performs hybrid FTS + vector search with RRF fusion.
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

// ftsSearch performs text search using ILIKE for CJK compatibility.
func (s *PgEpisodicStore) ftsSearch(ctx context.Context, query string, limit int) []Episode {
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
		SELECT id, session_id, role, content, created_at, token_count
		FROM episodes
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, where, argIdx)
	args = append(args, limit)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		slog.Warn("[episodic] fts search failed", "error", err, "keywords", keywords)
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

// vectorSearch performs semantic search using pgvector cosine distance.
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

// rrfMerge merges FTS and vector results using Reciprocal Rank Fusion.
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

// Summarize uses an LLM to compress search results into a concise summary.
func (s *PgEpisodicStore) Summarize(ctx context.Context, query string, episodes []Episode) (string, error) {
	if s.summarizer == nil || len(episodes) == 0 {
		var result string
		for _, ep := range episodes {
			result += fmt.Sprintf("[%s] %s\n", ep.Role, ep.Content)
		}
		return result, nil
	}

	var contextParts []string
	for _, ep := range episodes {
		contextParts = append(contextParts, fmt.Sprintf("[%s] %s", ep.Role, ep.Content))
	}

	summaryInput := []Message{
		{Role: "system", Content: fmt.Sprintf(
			"Summarize the following conversation excerpts relevant to the query: %q\n\n%s",
			query, joinStrings(contextParts, "\n"),
		)},
	}

	return s.summarizer.Summarize(ctx, summaryInput)
}

// ListSessions returns recent session summaries.
func (s *PgEpisodicStore) ListSessions(ctx context.Context, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.pool.Query(ctx, `
		SELECT s.id, s.title, s.source, s.created_at, COUNT(e.id) AS msg_count
		FROM sessions s
		LEFT JOIN episodes e ON e.session_id = s.id
		GROUP BY s.id
		ORDER BY s.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var summaries []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.ID, &ss.Title, &ss.Source, &ss.CreatedAt, &ss.MsgCount); err != nil {
			return nil, fmt.Errorf("scan session summary: %w", err)
		}
		summaries = append(summaries, ss)
	}

	return summaries, rows.Err()
}

// DeleteSession deletes a session and its episodes.
func (s *PgEpisodicStore) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// retryWithJitter retries a function with exponential backoff + jitter.
func (s *PgEpisodicStore) retryWithJitter(maxRetries int, minDelay, maxDelay time.Duration, fn func() error) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := fn(); err != nil {
			lastErr = err
			delay := minDelay + time.Duration(rand.Int63n(int64(maxDelay-minDelay)))
			slog.Warn("episodic store retry", "attempt", i+1, "delay", delay, "error", err)
			time.Sleep(delay)
			continue
		}
		return nil
	}
	return fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
