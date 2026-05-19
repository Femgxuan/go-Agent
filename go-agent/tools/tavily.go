package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TavilySearch uses the Tavily search API to perform web searches.
type TavilySearch struct {
	apiKey  string
	baseURL string
}

// NewTavilySearch creates a new TavilySearch tool.
func NewTavilySearch(apiKey, baseURL string) *TavilySearch {
	return &TavilySearch{apiKey: apiKey, baseURL: baseURL}
}

func (t *TavilySearch) Name() string        { return "tavily_search" }
func (t *TavilySearch) Description() string { return "Search the web using Tavily." }
func (t *TavilySearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query.",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return (default 5).",
			},
		},
		"required": []string{"query"},
	}
}

func (t *TavilySearch) Execute(ctx context.Context, params map[string]any) (string, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return "", fmt.Errorf("tavily_search: 'query' parameter is required and must be a string")
	}

	maxResults := 5
	if v, ok := params["max_results"]; ok {
		switch n := v.(type) {
		case int:
			maxResults = n
		case float64:
			maxResults = int(n)
		}
	}

	body := map[string]any{
		"api_key":     t.apiKey,
		"query":       query,
		"max_results": maxResults,
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("tavily_search: failed to encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("tavily_search: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tavily_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("tavily_search: failed to read response: %w", err)
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("tavily_search: failed to parse response: %w", err)
	}

	var sb strings.Builder
	for i, r := range result.Results {
		fmt.Fprintf(&sb, "%d. %s\n   URL: %s\n   %s\n\n", i+1, r.Title, r.URL, r.Content)
	}
	return sb.String(), nil
}
