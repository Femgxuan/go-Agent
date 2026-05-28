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
// information worth saving to a markdown file (SOUL.md, MEMORY.md, or USER.md).
type Classifier struct {
	llm client.LLMClient
}

// NewClassifier creates a new Classifier.
func NewClassifier(llm client.LLMClient) *Classifier {
	return &Classifier{llm: llm}
}

const classifyPrompt = `You are a memory classifier. Given a user message and an assistant reply, determine if the message contains information worth saving to a markdown file.

Categories:
- soul: Agent personality, communication style, behavior boundaries, values
  Examples: "be concise", "don't use emojis", "ask before destructive ops"
  中文示例: "记住说话要简洁", "不要用表情符号", "破坏性操作前要确认"
- memory: Project context, tech stack, environment, work conventions
  Examples: "we use PostgreSQL", "code style is Google", "repo at ~/code/proj"
  中文示例: "我们的项目用 PostgreSQL", "代码风格用 Google 规范", "仓库在 ~/code/proj"
- user: User personal facts, preferences, habits, constraints
  Examples: "my name is Alice", "I prefer Go", "I have a toddler"
  中文示例: "我叫张三", "我喜欢用 Go", "我有个小孩"
- none: Trivial info, temporary tasks, easily re-rediscoverable facts

CRITICAL DISTINCTION:
- "我们/我们的" + project/tech → memory (project fact)
- "我/我的" + personal identity/life → user (personal fact)
- "我/我的" + coding preference that affects project → memory (work convention)

Rules:
- DO NOT store: greetings, questions, tool requests, temporary info
- DO store: stable facts that remain true across sessions
- Curate: distill to 1-2 sentences, add § prefix

Respond with ONLY JSON:
{"target": "soul"|"memory"|"user"|"none", "content": "§ curated fact", "reason": "why"}`

// Classify determines whether the user input contains a fact worth saving to a markdown file.
// Returns a ClassificationResult and true if the result should be written, or zero value and false otherwise.
func (c *Classifier) Classify(ctx context.Context, userMsg, assistantMsg string) (ClassificationResult, bool) {
	prompt := fmt.Sprintf("%s\n\nUser: %s\nAssistant: %s", classifyPrompt, userMsg, assistantMsg)

	req := client.ChatRequest{
		Messages: []client.Message{
			{Role: client.RoleUser, Content: prompt},
		},
	}

	streamCh, err := c.llm.ChatCompletion(ctx, req)
	if err != nil {
		slog.Warn("[classifier] LLM call failed", "error", err)
		return ClassificationResult{}, false
	}

	var fullContent string
	for chunk := range streamCh {
		if chunk.Err != nil {
			slog.Warn("[classifier] stream error", "error", chunk.Err)
			return ClassificationResult{}, false
		}
		fullContent += chunk.Delta
	}

	fullContent = strings.TrimSpace(fullContent)
	// Strip markdown code fences if present
	fullContent = strings.TrimPrefix(fullContent, "```json")
	fullContent = strings.TrimPrefix(fullContent, "```")
	fullContent = strings.TrimSuffix(fullContent, "```")
	fullContent = strings.TrimSpace(fullContent)

	slog.Info("[classifier] LLM response", "raw", fullContent)

	var resp ClassificationResult
	if err := json.Unmarshal([]byte(fullContent), &resp); err != nil {
		slog.Warn("[classifier] JSON parse failed", "error", err, "raw", fullContent)
		return ClassificationResult{}, false
	}

	switch resp.Target {
	case "soul", "memory", "user":
		// valid target
	case "none":
		return ClassificationResult{}, false
	default:
		slog.Warn("[classifier] unknown target", "target", resp.Target)
		return ClassificationResult{}, false
	}

	if resp.Content == "" {
		return ClassificationResult{}, false
	}

	_ = time.Now() // ensure time import is used
	return resp, true
}
