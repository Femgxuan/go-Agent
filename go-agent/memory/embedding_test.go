package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMockEmbedder_Embed(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}

	ctx := context.Background()
	embedding, err := embedder.Embed(ctx, "hello world")
	assert.NoError(t, err)
	assert.Len(t, embedding, 1536)
}

func TestMockEmbedder_EmbedBatch(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}

	ctx := context.Background()
	texts := []string{"hello", "world"}
	embeddings, err := embedder.EmbedBatch(ctx, texts)
	assert.NoError(t, err)
	assert.Len(t, embeddings, 2)
	assert.Len(t, embeddings[0], 1536)
}

func TestMockEmbedder_Dimension(t *testing.T) {
	embedder := &MockEmbedder{dimension: 1536}
	assert.Equal(t, 1536, embedder.Dimension())
}
