package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fengxuan/go-agent/client"
)

// Classifier uses an LLM to classify whether user input contains
// information worth storing in long-term memory.
type Classifier struct {
	llm client.LLMClient
}

// NewClassifier creates a new Classifier.
func NewClassifier(llm client.LLMClient) *Classifier {
	return &Classifier{llm: llm}
}

// classificationResponse is the expected JSON output from the LLM.
type classificationResponse struct {
	ShouldStore bool   `json:"should_store"`
	Category    string `json:"category"`
	Key         string `json:"key"`
	Content     string `json:"content"`
	Reason      string `json:"reason"`
}

const classifyPrompt = `You are a memory classifier. Given a user message and an assistant reply, determine if the user's message contains information worth remembering long-term.

Categories:
- preference: User preferences, likes, dislikes ("I like Go", "我不喜欢Java")
- environment: Environment facts, server info, tech stack ("服务器是Debian 12", "We use PostgreSQL")
- correction: Corrections to agent behavior ("不要用sudo", "Don't use tabs")
- norm: Project conventions, coding style ("代码风格用Google", "Use 120-char lines")
- milestone: Completed work, achievements ("完成了迁移", "Finished the auth module")
- explicit: Direct memory requests ("记住...", "Remember...")

Rules:
- DO NOT store: greetings, general questions, tool requests, temporary info, things easily re-searchable
- DO store: facts/preferences/rules that remain true across sessions
- If user says something like "帮我查天气" (check weather), should_store=false
- If user says "我喜欢Go" (I like Go), should_store=true, category=preference

Respond with ONLY a JSON object:
{"should_store": true/false, "category": "...", "key": "short title", "content": "extracted fact", "reason": "why"}

If should_store is false, category/key/content can be empty strings.`

// Classify determines whether the user input contains a storable fact.
// It returns a Fact if classification succeeds and should_store=true, or zero Fact and false otherwise.
func (c *Classifier) Classify(ctx context.Context, userMsg, assistantMsg string) (Fact, bool) {
	prompt := fmt.Sprintf("%s\n\nUser: %s\nAssistant: %s", classifyPrompt, userMsg, assistantMsg)

	req := client.ChatRequest{
		Messages: []client.Message{
			{Role: client.RoleUser, Content: prompt},
		},
	}

	streamCh, err := c.llm.ChatCompletion(ctx, req)
	if err != nil {
		slog.Debug("classifier LLM call failed", "error", err)
		return Fact{}, false
	}

	var fullContent string
	for chunk := range streamCh {
		if chunk.Err != nil {
			slog.Debug("classifier stream error", "error", chunk.Err)
			return Fact{}, false
		}
		fullContent += chunk.Delta
	}

	fullContent = strings.TrimSpace(fullContent)
	// Strip markdown code fences if present
	fullContent = strings.TrimPrefix(fullContent, "```json")
	fullContent = strings.TrimPrefix(fullContent, "```")
	fullContent = strings.TrimSuffix(fullContent, "```")
	fullContent = strings.TrimSpace(fullContent)

	var resp classificationResponse
	if err := json.Unmarshal([]byte(fullContent), &resp); err != nil {
		slog.Debug("classifier JSON parse failed", "error", err, "raw", fullContent)
		return Fact{}, false
	}

	if !resp.ShouldStore {
		return Fact{}, false
	}

	category := FactCategory(resp.Category)
	switch category {
	case FactPreference, FactEnvironment, FactCorrection, FactNorm, FactMilestone, FactExplicit:
		// valid
	default:
		category = FactExplicit // fallback
	}

	return Fact{
		ID:         fmt.Sprintf("fact_%d", time.Now().UnixNano()),
		Category:   category,
		Key:        resp.Key,
		Content:    resp.Content,
		Source:     "user",
		Confidence: 0.8,
		CreatedAt:  time.Now(),
		DecayScore: 1.0,
	}, true
}
