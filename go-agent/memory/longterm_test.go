package memory

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgLongTermMemory_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Check environment variable
	pgURL := os.Getenv("MEMORY_LONGTERM_PG_URL")
	if pgURL == "" {
		pgURL = "postgres://goagent:goagent123@localhost:5432/goagent"
	}

	ctx := context.Background()

	// Create connection pool
	pool, err := NewPgPool(ctx, pgURL)
	require.NoError(t, err)
	defer pool.Close()

	// Create embedder mock
	embedder := &MockEmbedder{dimension: 1536}

	// Create long-term memory
	ltm := NewPgLongTermMemory(pool, embedder)

	// Test store
	fact := Fact{
		ID:         "test-1",
		Key:        "project",
		Content:    "Go Agent is a ReAct-style Agent framework",
		Source:     "user",
		CreatedAt:  time.Now(),
		DecayScore: 1.0,
	}

	err = ltm.Store(ctx, fact)
	require.NoError(t, err)

	// Test search
	results, err := ltm.Search(ctx, "Go Agent", 5, 0.5)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "Go Agent", results[0].Key)

	// Test update
	err = ltm.Update(ctx, "test-1", map[string]any{"decay_score": 0.8})
	require.NoError(t, err)

	// Test delete
	err = ltm.Delete(ctx, "test-1")
	require.NoError(t, err)
}
