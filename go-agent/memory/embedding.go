package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Embedder is the abstract interface for embedding models
type Embedder interface {
	// Embed converts text to a vector
	Embed(ctx context.Context, text string) ([]float64, error)

	// EmbedBatch performs batch embedding
	EmbedBatch(ctx context.Context, texts []string) ([][]float64, error)

	// Dimension returns the vector dimension
	Dimension() int
}

// OpenAIEmbedder uses the OpenAI API embedding model
type OpenAIEmbedder struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIEmbedder creates a new OpenAIEmbedder
func NewOpenAIEmbedder(apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed converts text to a vector
func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	embeddings, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// EmbedBatch performs batch embedding
func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := map[string]any{
		"input": texts,
		"model": e.model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	embeddings := make([][]float64, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// Dimension returns the vector dimension
func (e *OpenAIEmbedder) Dimension() int {
	switch e.model {
	case "text-embedding-3-small":
		return 1536
	case "text-embedding-3-large":
		return 3072
	default:
		return 1536
	}
}

// OllamaEmbedder uses a local Ollama service embedding model
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaEmbedder creates a new OllamaEmbedder
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Embed converts text to a vector
func (e *OllamaEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := map[string]any{
		"prompt": text,
		"model":  e.model,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", string(body))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return result.Embedding, nil
}

// EmbedBatch performs batch embedding
func (e *OllamaEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	var mu sync.Mutex
	results := make([][]float64, len(texts))
	errs := make([]error, len(texts))

	// Concurrent processing
	var wg sync.WaitGroup
	for i, text := range texts {
		wg.Add(1)
		go func(idx int, t string) {
			defer wg.Done()
			embedding, err := e.Embed(ctx, t)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[idx] = err
			} else {
				results[idx] = embedding
			}
		}(i, text)
	}
	wg.Wait()

	// Check for errors
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// Dimension returns the vector dimension
func (e *OllamaEmbedder) Dimension() int {
	// nomic-embed-text dimension is 768
	if e.model == "nomic-embed-text" {
		return 768
	}
	return 768
}

// MockEmbedder is an embedding model implementation for testing
type MockEmbedder struct {
	dimension int
}

// Embed generates a deterministic vector (for testing)
func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	embedding := make([]float64, m.dimension)
	for i := range embedding {
		embedding[i] = float64(i) / float64(m.dimension)
	}
	return embedding, nil
}

// EmbedBatch performs batch embedding
func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := m.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = embedding
	}
	return results, nil
}

// Dimension returns the vector dimension
func (m *MockEmbedder) Dimension() int {
	return m.dimension
}
