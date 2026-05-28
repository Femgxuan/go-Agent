package memory

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	pool       *pgxpool.Pool
	summarizer Summarizer
	tokenizer  Tokenizer
}

// NewPgEpisodicStore creates a new PG-backed episodic store.
func NewPgEpisodicStore(pool *pgxpool.Pool, summarizer Summarizer, tokenizer Tokenizer) *PgEpisodicStore {
	return &PgEpisodicStore{
		pool:       pool,
		summarizer: summarizer,
		tokenizer:  tokenizer,
	}
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
			_, err = tx.Exec(ctx, `
				INSERT INTO episodes (id, session_id, role, content, created_at, token_count)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (id) DO NOTHING
			`, ep.ID, session.ID, ep.Role, ep.Content, ep.CreatedAt, tokenCount)
			if err != nil {
				return fmt.Errorf("insert episode: %w", err)
			}
		}

		return tx.Commit(ctx)
	})
}

// Search performs full-text search on episodes.
// Uses ILIKE for CJK text compatibility, falls back to FTS for Latin text.
func (s *PgEpisodicStore) Search(ctx context.Context, query string, limit int) ([]Episode, error) {
	if limit <= 0 {
		limit = 5
	}

	// Use ILIKE for text search — works for Chinese and English.
	rows, err := s.pool.Query(ctx, `
		SELECT id, session_id, role, content, created_at, token_count
		FROM episodes
		WHERE content ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var ep Episode
		if err := rows.Scan(&ep.ID, &ep.SessionID, &ep.Role, &ep.Content, &ep.CreatedAt, &ep.TokenCount); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		episodes = append(episodes, ep)
	}

	return episodes, rows.Err()
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
