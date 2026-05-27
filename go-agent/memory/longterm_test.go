package memory

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "CJK preference query with sliding window",
			query:    "我喜欢吃什么",
			expected: []string{"喜欢", "欢吃"},
		},
		{
			name:     "CJK two-char query",
			query:    "用户偏好",
			expected: []string{"用户", "户偏", "偏好"},
		},
		{
			name:     "English with spaces",
			query:    "Go Agent framework",
			expected: []string{"Go", "Agent", "framework"},
		},
		{
			name:     "empty query",
			query:    "",
			expected: nil,
		},
		{
			name:     "only stop words",
			query:    "我的",
			expected: nil,
		},
		{
			name:     "question with punctuation",
			query:    "我喜欢什么？",
			expected: []string{"喜欢"},
		},
		{
			name:     "stored fact search",
			query:    "用户喜欢吃香蕉",
			expected: []string{"用户", "户喜", "喜欢", "欢吃", "吃香", "香蕉"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractKeywords(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
