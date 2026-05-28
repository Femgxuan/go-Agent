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

## Categories

### soul — About the AGENT itself
How the agent should behave, talk, and make decisions. Personality, tone, values, boundaries.
Ask yourself: "Should this shape how I act in ALL future conversations?"

Examples:
- "be concise, no fluff"
- "don't use emojis"
- "always ask before running destructive commands"
- "回答要简洁，不要废话"

### memory — About the WORK and PROJECT
Facts about the project, codebase, tech stack, infrastructure, team conventions, work environment.
Ask yourself: "Is this about the work I'm helping with, not about the person?"

Examples:
- "the project uses PostgreSQL and Redis"
- "we deploy to AWS, region us-east-1"
- "code style follows Google Go conventions"
- "the repo is at ~/code/brightcart"
- "we use Stripe for payments"
- "项目用的是 PostgreSQL"
- "部署在阿里云华东区"
- "代码风格用 Google 规范"

### user — About the PERSON
Facts about the user's identity, life, personal preferences, habits, constraints.
Ask yourself: "Is this about who the user is as a person?"

Examples:
- "my name is Alice"
- "I live in Shanghai"
- "I have a 2-year-old kid"
- "I prefer dark mode"
- "I'm left-handed"
- "我叫张三"
- "我在上海工作"
- "我有个两岁的孩子"

### none — Not worth storing
Greetings, questions, tool requests, temporary tasks, easily re-derivable facts.

## Decision Guide

When the information could fit multiple categories, ask:
1. Does it describe the PROJECT or WORK context? → memory
2. Does it describe the PERSON? → user
3. Does it describe the AGENT's behavior? → soul
4. Is it trivial or temporary? → none

When in doubt between memory and user: if removing this fact would make the agent worse at helping with work, it's memory. If it would just make the agent less personalized, it's user.

## Rules
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
