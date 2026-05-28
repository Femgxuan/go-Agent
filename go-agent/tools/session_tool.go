package tools

import (
	"context"
	"fmt"

	"github.com/fengxuan/go-agent/memory"
)

// SessionSearchTool searches historical conversations using episodic memory.
type SessionSearchTool struct {
	episodic memory.EpisodicStore
}

func NewSessionSearchTool(episodic memory.EpisodicStore) *SessionSearchTool {
	return &SessionSearchTool{episodic: episodic}
}

func (t *SessionSearchTool) Name() string        { return "session_search" }
func (t *SessionSearchTool) Description() string  { return "Search past conversations for relevant context" }
func (t *SessionSearchTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]string{"type": "string", "description": "Search query"},
			"limit": map[string]string{"type": "integer", "description": "Max results (default 5)"},
		},
		"required": []string{"query"},
	}
}

func (t *SessionSearchTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	limit := 5
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Full-text search
	episodes, err := t.episodic.Search(ctx, query, limit)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(episodes) == 0 {
		return "No relevant past conversations found.", nil
	}

	// LLM summarization
	summary, err := t.episodic.Summarize(ctx, query, episodes)
	if err != nil {
		return "", fmt.Errorf("summarize failed: %w", err)
	}

	return summary, nil
}
