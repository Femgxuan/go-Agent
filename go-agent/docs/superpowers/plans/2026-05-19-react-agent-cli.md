# Go-Agent (ReAct AI Agent CLI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a terminal-based ReAct AI Agent in Go with a hermes-style TUI, multi-provider LLM support, multi-turn conversation, and an extensible tool system.

**Architecture:** The application is layered bottom-up: `config` (YAML + env), `client` (LLM abstraction), `tools` (registry + 4 built-in tools), `agent` (ReAct loop emitting events via channel), `tui` (bubbletea consuming events). Each layer depends only on the one below it. Agent and TUI communicate exclusively via a `<-chan AgentEvent` channel.

**Tech Stack:** Go 1.21+, charmbracelet/bubbletea + bubbles + lipgloss (TUI), gopkg.in/yaml.v3 (config), golang.org/x/sync (errgroup)

---

## File Structure

| File | Responsibility |
|------|---------------|
| `go.mod` | Module definition and dependencies |
| `main.go` | Entry point: load config, wire deps, start bubbletea |
| `config/config.go` | Config struct, YAML loading, env var override |
| `client/provider.go` | `LLMClient` interface, `ChatRequest`, `StreamChunk`, `Message`, `ToolCall`, `ToolSchema` types |
| `client/openai.go` | OpenAI-compatible streaming client (covers DeepSeek, Ollama, etc.) |
| `client/anthropic.go` | Anthropic Messages API streaming client |
| `agent/message.go` | `AgentEvent` type and `EventType` constants |
| `agent/agent.go` | ReAct loop: build messages → call LLM → parse → execute tools → loop |
| `tools/registry.go` | `Tool` interface, `Registry` struct, JSON Schema conversion |
| `tools/tavily.go` | Tavily search tool |
| `tools/shell.go` | Shell exec tool with blocklist |
| `tools/readfile.go` | File read tool with size limit |
| `tools/writefile.go` | File write tool |
| `tui/styles.go` | lipgloss color/border/layout styles |
| `tui/statusbar.go` | Top bar: app name, model, state, elapsed time |
| `tui/viewport.go` | Scrollable message area with collapsible panels |
| `tui/input.go` | Text input component wrapping bubbles/textarea |
| `tui/app.go` | Main bubbletea Model: Init/Update/View, event channel bridge, key handling |
| `config/config_test.go` | Config loading tests |
| `client/openai_test.go` | OpenAI client tests with mock HTTP server |
| `agent/agent_test.go` | ReAct loop tests with mock LLMClient |
| `tools/registry_test.go` | Registry tests |
| `tools/tools_test.go` | Built-in tool unit tests |

---

### Task 1: Project Scaffolding + Config

**Files:**
- Create: `go.mod`
- Create: `config/config.go`
- Create: `config/config_test.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/fengxuan/Desktop/go-agent
go mod init github.com/fengxuan/go-agent
```

- [ ] **Step 2: Write config test**

Create `config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
max_iterations: 5
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    model: gpt-4o
  deepseek:
    api_key: sk-ds
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat
tools:
  tavily:
    api_key: tvly-test
  shell:
    blocked_commands: ["sudo", "rm -rf /"]
  file:
    max_read_size: 2097152
`), 0644)

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai", cfg.DefaultProvider)
	}
	if cfg.MaxIterations != 5 {
		t.Errorf("MaxIterations = %d, want 5", cfg.MaxIterations)
	}
	p := cfg.Providers["openai"]
	if p.APIKey != "sk-test" || p.Model != "gpt-4o" {
		t.Errorf("openai provider = %+v", p)
	}
	ds := cfg.Providers["deepseek"]
	if ds.BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("deepseek base_url = %q", ds.BaseURL)
	}
	if cfg.Tools.Tavily.APIKey != "tvly-test" {
		t.Errorf("tavily api_key = %q", cfg.Tools.Tavily.APIKey)
	}
	if cfg.Tools.File.MaxReadSize != 2097152 {
		t.Errorf("max_read_size = %d", cfg.Tools.File.MaxReadSize)
	}
}

func TestEnvVarOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
providers:
  openai:
    api_key: sk-from-file
    base_url: https://api.openai.com/v1
    model: gpt-4o
tools:
  tavily:
    api_key: tvly-from-file
`), 0644)

	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	t.Setenv("TAVILY_API_KEY", "tvly-from-env")

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	ApplyEnvOverrides(cfg)

	if cfg.Providers["openai"].APIKey != "sk-from-env" {
		t.Errorf("openai api_key = %q, want sk-from-env", cfg.Providers["openai"].APIKey)
	}
	if cfg.Tools.Tavily.APIKey != "tvly-from-env" {
		t.Errorf("tavily api_key = %q, want tvly-from-env", cfg.Tools.Tavily.APIKey)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	os.WriteFile(cfgPath, []byte(`
default_provider: openai
providers:
  openai:
    api_key: sk-test
    base_url: https://api.openai.com/v1
    model: gpt-4o
`), 0644)

	cfg, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("MaxIterations default = %d, want 10", cfg.MaxIterations)
	}
	if cfg.Tools.File.MaxReadSize != 1048576 {
		t.Errorf("MaxReadSize default = %d, want 1048576", cfg.Tools.File.MaxReadSize)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./config/ -v
```

Expected: compilation error — `LoadFromFile`, `ApplyEnvOverrides` not defined.

- [ ] **Step 4: Implement config**

Create `config/config.go`:

```go
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
}

type TavilyConfig struct {
	APIKey string `yaml:"api_key"`
}

type ShellConfig struct {
	AllowedCommands []string `yaml:"allowed_commands"`
	BlockedCommands []string `yaml:"blocked_commands"`
}

type FileConfig struct {
	MaxReadSize int64 `yaml:"max_read_size"`
}

type ToolsConfig struct {
	Tavily TavilyConfig `yaml:"tavily"`
	Shell  ShellConfig  `yaml:"shell"`
	File   FileConfig   `yaml:"file"`
}

type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	MaxIterations   int                       `yaml:"max_iterations"`
	Providers       map[string]ProviderConfig `yaml:"providers"`
	Tools           ToolsConfig               `yaml:"tools"`
}

func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 10
	}
	if cfg.Tools.File.MaxReadSize <= 0 {
		cfg.Tools.File.MaxReadSize = 1048576
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
}

func ApplyEnvOverrides(cfg *Config) {
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		if p, ok := cfg.Providers["openai"]; ok {
			p.APIKey = v
			cfg.Providers["openai"] = p
		}
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		if p, ok := cfg.Providers["openai"]; ok {
			p.BaseURL = v
			cfg.Providers["openai"] = p
		}
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		if p, ok := cfg.Providers["anthropic"]; ok {
			p.APIKey = v
			cfg.Providers["anthropic"] = p
		}
	}
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		if p, ok := cfg.Providers["deepseek"]; ok {
			p.APIKey = v
			cfg.Providers["deepseek"] = p
		}
	}
	if v := os.Getenv("TAVILY_API_KEY"); v != "" {
		cfg.Tools.Tavily.APIKey = v
	}
}
```

- [ ] **Step 5: Add yaml dependency and run tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go get gopkg.in/yaml.v3 && go test ./config/ -v
```

Expected: all 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git init
git add go.mod go.sum config/
git commit -m "feat: project scaffolding and config loading with YAML + env override"
```

---

### Task 2: LLM Client Interface + Types

**Files:**
- Create: `client/provider.go`

- [ ] **Step 1: Define client types**

Create `client/provider.go`:

```go
package client

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID       string
	Name     string
	Arguments string
}

type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

type ToolSchema struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSchema
}

type StreamChunk struct {
	Delta        string
	ToolCalls    []ToolCall
	FinishReason string
	Err          error
}

type LLMClient interface {
	ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go build ./client/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add client/provider.go
git commit -m "feat: define LLMClient interface and message types"
```

---

### Task 3: OpenAI-Compatible Streaming Client

**Files:**
- Create: `client/openai.go`
- Create: `client/openai_test.go`

- [ ] **Step 1: Write OpenAI client test**

Create `client/openai_test.go`:

```go
package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIStreamingTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"id":"1","choices":[{"delta":{"role":"assistant","content":"Hello"},"index":0}]}`,
			`{"id":"1","choices":[{"delta":{"content":" world"},"index":0}]}`,
			`{"id":"1","choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	c := NewOpenAIClient(server.URL, "sk-test", "gpt-4o")
	ch, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var full string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		full += chunk.Delta
	}
	if full != "Hello world" {
		t.Errorf("full = %q, want %q", full, "Hello world")
	}
}

func TestOpenAIStreamingToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"id":"1","choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"tavily_search","arguments":""}}]},"index":0}]}`,
			`{"id":"1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"query\":"}}]},"index":0}]}`,
			`{"id":"1","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"golang\"}"}}]},"index":0}]}`,
			`{"id":"1","choices":[{"delta":{},"index":0,"finish_reason":"tool_calls"}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	c := NewOpenAIClient(server.URL, "sk-test", "gpt-4o")
	ch, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "search golang"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var lastChunk StreamChunk
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		lastChunk = chunk
	}
	if len(lastChunk.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(lastChunk.ToolCalls))
	}
	tc := lastChunk.ToolCalls[0]
	if tc.Name != "tavily_search" || tc.Arguments != `{"query":"golang"}` {
		t.Errorf("tool_call = %+v", tc)
	}
}

func TestOpenAIContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for i := 0; i < 100; i++ {
			fmt.Fprintf(w, "data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"x\"},\"index\":0}]}\n\n")
			flusher.Flush()
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := NewOpenAIClient(server.URL, "sk-test", "gpt-4o")
	ch, err := c.ChatCompletion(ctx, ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	<-ch
	cancel()

	count := 0
	for range ch {
		count++
	}
	if count > 10 {
		t.Errorf("received %d chunks after cancel, expected channel to close quickly", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./client/ -v
```

Expected: compilation error — `NewOpenAIClient` not defined.

- [ ] **Step 3: Implement OpenAI client**

Create `client/openai.go`:

```go
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAIClient(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

type openaiRequest struct {
	Model    string           `json:"model"`
	Messages []openaiMessage  `json:"messages"`
	Tools    []ToolSchema     `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type openaiMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	Index    *int              `json:"index,omitempty"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function openaiFunction    `json:"function"`
}

type openaiFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openaiStreamResponse struct {
	ID      string              `json:"id"`
	Choices []openaiStreamChoice `json:"choices"`
}

type openaiStreamChoice struct {
	Index        int             `json:"index"`
	Delta        openaiDelta     `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type openaiDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
}

func (c *OpenAIClient) ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	msgs := make([]openaiMessage, len(req.Messages))
	for i, m := range req.Messages {
		msg := openaiMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openaiFunction{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		msgs[i] = msg
	}

	body := openaiRequest{
		Model:    model,
		Messages: msgs,
		Tools:    req.Tools,
		Stream:   true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 16)
	go c.streamResponse(ctx, resp.Body, ch)
	return ch, nil
}

func (c *OpenAIClient) streamResponse(ctx context.Context, body io.ReadCloser, ch chan<- StreamChunk) {
	defer close(ch)
	defer body.Close()

	// accumulate tool calls across chunks
	toolCallAccum := make(map[int]*ToolCall)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			// flush accumulated tool calls
			if len(toolCallAccum) > 0 {
				var calls []ToolCall
				for i := 0; i < len(toolCallAccum); i++ {
					if tc, ok := toolCallAccum[i]; ok {
						calls = append(calls, *tc)
					}
				}
				ch <- StreamChunk{ToolCalls: calls, FinishReason: "tool_calls"}
			}
			return
		}

		var resp openaiStreamResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			ch <- StreamChunk{Err: fmt.Errorf("parse SSE: %w", err)}
			return
		}

		for _, choice := range resp.Choices {
			if choice.Delta.Content != "" {
				ch <- StreamChunk{Delta: choice.Delta.Content}
			}
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				if _, ok := toolCallAccum[idx]; !ok {
					toolCallAccum[idx] = &ToolCall{
						ID:   tc.ID,
						Name: tc.Function.Name,
					}
				}
				toolCallAccum[idx].Arguments += tc.Function.Arguments
				if tc.ID != "" {
					toolCallAccum[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCallAccum[idx].Name = tc.Function.Name
				}
			}
			if choice.FinishReason != nil && *choice.FinishReason == "stop" {
				ch <- StreamChunk{FinishReason: "stop"}
				return
			}
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./client/ -v -count=1
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add client/
git commit -m "feat: OpenAI-compatible streaming client with SSE parsing"
```

---

### Task 4: Anthropic Streaming Client

**Files:**
- Create: `client/anthropic.go`
- Create: `client/anthropic_test.go`

- [ ] **Step 1: Write Anthropic client test**

Create `client/anthropic_test.go`:

```go
package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicStreamingTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant-test" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[]}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\" world\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := NewAnthropicClient(server.URL, "sk-ant-test", "claude-sonnet-4-20250514")
	ch, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var full string
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		full += chunk.Delta
	}
	if full != "Hello world" {
		t.Errorf("full = %q, want %q", full, "Hello world")
	}
}

func TestAnthropicStreamingToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[]}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"tavily_search\",\"input\":{}}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"query\\\": \\\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"golang\\\"}\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, e := range events {
			fmt.Fprint(w, e)
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := NewAnthropicClient(server.URL, "sk-ant-test", "claude-sonnet-4-20250514")
	ch, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "search golang"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	var lastChunk StreamChunk
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk error: %v", chunk.Err)
		}
		if len(chunk.ToolCalls) > 0 {
			lastChunk = chunk
		}
	}
	if len(lastChunk.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(lastChunk.ToolCalls))
	}
	tc := lastChunk.ToolCalls[0]
	if tc.Name != "tavily_search" {
		t.Errorf("tool name = %q, want tavily_search", tc.Name)
	}
	if tc.ID != "toolu_1" {
		t.Errorf("tool id = %q, want toolu_1", tc.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./client/ -run TestAnthropic -v
```

Expected: compilation error — `NewAnthropicClient` not defined.

- [ ] **Step 3: Implement Anthropic client**

Create `client/anthropic.go`:

```go
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewAnthropicClient(baseURL, apiKey, model string) *AnthropicClient {
	return &AnthropicClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

func (c *AnthropicClient) ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error) {
	model := req.Model
	if model == "" {
		model = c.model
	}

	var systemPrompt string
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		if m.Role == RoleTool {
			msgs = append(msgs, anthropicMessage{
				Role: "user",
				Content: []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
			continue
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			var blocks []map[string]any
			if m.Content != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": input,
				})
			}
			msgs = append(msgs, anthropicMessage{Role: "assistant", Content: blocks})
			continue
		}
		msgs = append(msgs, anthropicMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}

	var tools []anthropicTool
	for _, t := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	body := anthropicRequest{
		Model:     model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  msgs,
		Tools:     tools,
		Stream:    true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan StreamChunk, 16)
	go c.streamResponse(ctx, resp.Body, ch)
	return ch, nil
}

func (c *AnthropicClient) streamResponse(ctx context.Context, body io.ReadCloser, ch chan<- StreamChunk) {
	defer close(ch)
	defer body.Close()

	type toolAccum struct {
		id        string
		name      string
		inputJSON string
	}
	toolAccums := make(map[int]*toolAccum)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				toolAccums[event.Index] = &toolAccum{
					id:   event.ContentBlock.ID,
					name: event.ContentBlock.Name,
				}
			}
		case "content_block_delta":
			if event.Delta.Type == "text_delta" {
				ch <- StreamChunk{Delta: event.Delta.Text}
			} else if event.Delta.Type == "input_json_delta" {
				if ta, ok := toolAccums[event.Index]; ok {
					ta.inputJSON += event.Delta.PartialJSON
				}
			}
		case "message_delta":
			if event.Delta.StopReason == "tool_use" {
				var calls []ToolCall
				for i := 0; i < len(toolAccums); i++ {
					if ta, ok := toolAccums[i]; ok {
						calls = append(calls, ToolCall{
							ID:        ta.id,
							Name:      ta.name,
							Arguments: ta.inputJSON,
						})
					}
				}
				if len(calls) > 0 {
					ch <- StreamChunk{ToolCalls: calls, FinishReason: "tool_calls"}
				}
			} else if event.Delta.StopReason == "end_turn" {
				ch <- StreamChunk{FinishReason: "stop"}
			}
		case "message_stop":
			return
		}
	}
}
```

- [ ] **Step 4: Run all client tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./client/ -v -count=1
```

Expected: all 5 tests PASS (3 OpenAI + 2 Anthropic).

- [ ] **Step 5: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add client/anthropic.go client/anthropic_test.go
git commit -m "feat: Anthropic Messages API streaming client"
```

---

### Task 5: Tool Interface + Registry

**Files:**
- Create: `tools/registry.go`
- Create: `tools/registry_test.go`

- [ ] **Step 1: Write registry test**

Create `tools/registry_test.go`:

```go
package tools

import (
	"context"
	"testing"
)

type mockTool struct {
	name   string
	desc   string
	result string
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Description() string            { return m.desc }
func (m *mockTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
		"required": []string{"query"},
	}
}
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return m.result, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", desc: "a test tool", result: "ok"}
	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.Name() != "test_tool" {
		t.Errorf("Name = %q", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("Get returned true for nonexistent tool")
	}
}

func TestRegistryToolSchemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "tool_a", desc: "desc a"})
	r.Register(&mockTool{name: "tool_b", desc: "desc b"})

	schemas := r.ToolSchemas()
	if len(schemas) != 2 {
		t.Fatalf("ToolSchemas len = %d, want 2", len(schemas))
	}

	found := map[string]bool{}
	for _, s := range schemas {
		if s.Type != "function" {
			t.Errorf("schema type = %q, want function", s.Type)
		}
		found[s.Function.Name] = true
	}
	if !found["tool_a"] || !found["tool_b"] {
		t.Errorf("schemas = %v", found)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./tools/ -v
```

Expected: compilation error — `NewRegistry` not defined.

- [ ] **Step 3: Implement registry**

Create `tools/registry.go`:

```go
package tools

import (
	"context"

	"github.com/fengxuan/go-agent/client"
)

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, params map[string]any) (string, error)
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) ToolSchemas() []client.ToolSchema {
	schemas := make([]client.ToolSchema, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		schemas = append(schemas, client.ToolSchema{
			Type: "function",
			Function: client.FunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}
	return schemas
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./tools/ -v
```

Expected: all 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tools/
git commit -m "feat: tool interface and registry with JSON Schema generation"
```

---

### Task 6: Built-in Tools (tavily, shell, readfile, writefile)

**Files:**
- Create: `tools/tavily.go`
- Create: `tools/shell.go`
- Create: `tools/readfile.go`
- Create: `tools/writefile.go`
- Create: `tools/tools_test.go`

- [ ] **Step 1: Write tool tests**

Create `tools/tools_test.go`:

```go
package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTavilySearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"results":[{"title":"Go lang","url":"https://go.dev","content":"Go is an open source programming language."}]}`)
	}))
	defer server.Close()

	tool := NewTavilySearch("test-key", server.URL+"/search")
	if tool.Name() != "tavily_search" {
		t.Errorf("Name = %q", tool.Name())
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"query": "golang",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("result is empty")
	}
}

func TestShellExec(t *testing.T) {
	tool := NewShellExec([]string{"sudo", "rm -rf /"})

	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello\n" {
		t.Errorf("result = %q, want %q", result, "hello\n")
	}
}

func TestShellExecBlocked(t *testing.T) {
	tool := NewShellExec([]string{"sudo", "rm -rf /"})

	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "sudo rm -rf /",
	})
	if err == nil {
		t.Fatal("expected error for blocked command")
	}
}

func TestShellExecTimeout(t *testing.T) {
	tool := NewShellExec(nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "sleep 10",
		"timeout": float64(1),
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	tool := NewReadFile(1048576)

	result, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q", result)
	}
}

func TestReadFilePathTraversal(t *testing.T) {
	tool := NewReadFile(1048576)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": "/etc/../etc/passwd",
	})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestReadFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	os.WriteFile(path, make([]byte, 2000), 0644)

	tool := NewReadFile(1000)

	_, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
	})
	if err == nil {
		t.Fatal("expected error for file too large")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	tool := NewWriteFile()

	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == "" {
		t.Error("result is empty")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Errorf("file content = %q", string(data))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./tools/ -v
```

Expected: compilation error — tool constructors not defined.

- [ ] **Step 3: Implement tavily_search**

Create `tools/tavily.go`:

```go
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

type TavilySearch struct {
	apiKey  string
	baseURL string
}

func NewTavilySearch(apiKey, baseURL string) *TavilySearch {
	if baseURL == "" {
		baseURL = "https://api.tavily.com/search"
	}
	return &TavilySearch{apiKey: apiKey, baseURL: baseURL}
}

func (t *TavilySearch) Name() string        { return "tavily_search" }
func (t *TavilySearch) Description() string { return "Search the web using Tavily search engine" }
func (t *TavilySearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Search query"},
			"max_results": map[string]any{"type": "integer", "description": "Maximum number of results (default 5)"},
		},
		"required": []string{"query"},
	}
}

func (t *TavilySearch) Execute(ctx context.Context, params map[string]any) (string, error) {
	query, _ := params["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	maxResults := 5
	if v, ok := params["max_results"].(float64); ok {
		maxResults = int(v)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"api_key":     t.apiKey,
		"query":       query,
		"max_results": maxResults,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", t.baseURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tavily request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("tavily error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse tavily response: %w", err)
	}

	var sb strings.Builder
	for i, r := range result.Results {
		fmt.Fprintf(&sb, "[%d] %s\n%s\n%s\n\n", i+1, r.Title, r.URL, r.Content)
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Implement shell_exec**

Create `tools/shell.go`:

```go
package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ShellExec struct {
	blockedCommands []string
}

func NewShellExec(blockedCommands []string) *ShellExec {
	return &ShellExec{blockedCommands: blockedCommands}
}

func (s *ShellExec) Name() string        { return "shell_exec" }
func (s *ShellExec) Description() string { return "Execute a shell command and return the output" }
func (s *ShellExec) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "Shell command to execute"},
			"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds (default 30)"},
		},
		"required": []string{"command"},
	}
}

func (s *ShellExec) Execute(ctx context.Context, params map[string]any) (string, error) {
	command, _ := params["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	for _, blocked := range s.blockedCommands {
		if strings.Contains(command, blocked) {
			return "", fmt.Errorf("command blocked: contains %q", blocked)
		}
	}

	timeout := 30
	if v, ok := params["timeout"].(float64); ok && v > 0 {
		timeout = int(v)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %ds", timeout)
	}
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}
```

- [ ] **Step 5: Implement read_file**

Create `tools/readfile.go`:

```go
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type ReadFile struct {
	maxSize int64
}

func NewReadFile(maxSize int64) *ReadFile {
	return &ReadFile{maxSize: maxSize}
}

func (r *ReadFile) Name() string        { return "read_file" }
func (r *ReadFile) Description() string { return "Read the contents of a file" }
func (r *ReadFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Absolute path to the file"},
		},
		"required": []string{"path"},
	}
}

func (r *ReadFile) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path traversal not allowed: %q", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > r.maxSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), r.maxSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(data), nil
}
```

- [ ] **Step 6: Implement write_file**

Create `tools/writefile.go`:

```go
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type WriteFile struct{}

func NewWriteFile() *WriteFile { return &WriteFile{} }

func (w *WriteFile) Name() string        { return "write_file" }
func (w *WriteFile) Description() string { return "Write content to a file" }
func (w *WriteFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Absolute path to the file"},
			"content": map[string]any{"type": "string", "description": "Content to write"},
		},
		"required": []string{"path", "content"},
	}
}

func (w *WriteFile) Execute(_ context.Context, params map[string]any) (string, error) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}
```

- [ ] **Step 7: Run all tool tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./tools/ -v -count=1
```

Expected: all 9 tests PASS (2 registry + 7 tools).

- [ ] **Step 8: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tools/
git commit -m "feat: built-in tools - tavily_search, shell_exec, read_file, write_file"
```

---

### Task 7: Agent Event Types

**Files:**
- Create: `agent/message.go`

- [ ] **Step 1: Define agent event types**

Create `agent/message.go`:

```go
package agent

type EventType int

const (
	EventThought    EventType = iota
	EventToolCall
	EventToolResult
	EventAnswer
	EventError
	EventDelta
)

func (e EventType) String() string {
	switch e {
	case EventThought:
		return "Thought"
	case EventToolCall:
		return "Action"
	case EventToolResult:
		return "Observation"
	case EventAnswer:
		return "Answer"
	case EventError:
		return "Error"
	case EventDelta:
		return "Delta"
	default:
		return "Unknown"
	}
}

type AgentEvent struct {
	Type     EventType
	Content  string
	ToolName string
	ToolID   string
	Params   map[string]any
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go build ./agent/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add agent/
git commit -m "feat: define AgentEvent types for ReAct loop events"
```

---

### Task 8: ReAct Agent Core Loop

**Files:**
- Create: `agent/agent.go`
- Create: `agent/agent_test.go`

- [ ] **Step 1: Write agent test**

Create `agent/agent_test.go`:

```go
package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/tools"
)

type mockLLMClient struct {
	responses []mockResponse
	callIndex int
}

type mockResponse struct {
	content   string
	toolCalls []client.ToolCall
}

func (m *mockLLMClient) ChatCompletion(_ context.Context, _ client.ChatRequest) (<-chan client.StreamChunk, error) {
	ch := make(chan client.StreamChunk, 2)
	resp := m.responses[m.callIndex]
	m.callIndex++

	go func() {
		defer close(ch)
		if resp.content != "" {
			ch <- client.StreamChunk{Delta: resp.content, FinishReason: "stop"}
		}
		if len(resp.toolCalls) > 0 {
			ch <- client.StreamChunk{ToolCalls: resp.toolCalls, FinishReason: "tool_calls"}
		}
	}()
	return ch, nil
}

type constTool struct {
	name   string
	result string
}

func (c *constTool) Name() string        { return c.name }
func (c *constTool) Description() string { return "test tool" }
func (c *constTool) Schema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"input": map[string]any{"type": "string"}},
	}
}
func (c *constTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return c.result, nil
}

func collectEvents(ch <-chan AgentEvent) []AgentEvent {
	var events []AgentEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestAgentDirectAnswer(t *testing.T) {
	llm := &mockLLMClient{
		responses: []mockResponse{
			{content: "Hello!"},
		},
	}
	reg := tools.NewRegistry()
	a := New(llm, reg, AgentConfig{MaxIterations: 10, SystemPrompt: "You are helpful."})

	events := collectEvents(a.Run(context.Background(), "hi"))

	var hasAnswer bool
	for _, e := range events {
		if e.Type == EventAnswer {
			hasAnswer = true
			if e.Content != "Hello!" {
				t.Errorf("answer = %q", e.Content)
			}
		}
	}
	if !hasAnswer {
		t.Error("no EventAnswer received")
	}
}

func TestAgentToolCallLoop(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"input": "test"})
	llm := &mockLLMClient{
		responses: []mockResponse{
			{toolCalls: []client.ToolCall{{ID: "call_1", Name: "search", Arguments: string(args)}}},
			{content: "Found result."},
		},
	}
	reg := tools.NewRegistry()
	reg.Register(&constTool{name: "search", result: "search result here"})
	a := New(llm, reg, AgentConfig{MaxIterations: 10, SystemPrompt: "You are helpful."})

	events := collectEvents(a.Run(context.Background(), "search something"))

	var hasToolCall, hasToolResult, hasAnswer bool
	for _, e := range events {
		switch e.Type {
		case EventToolCall:
			hasToolCall = true
			if e.ToolName != "search" {
				t.Errorf("tool name = %q", e.ToolName)
			}
		case EventToolResult:
			hasToolResult = true
			if e.Content != "search result here" {
				t.Errorf("tool result = %q", e.Content)
			}
		case EventAnswer:
			hasAnswer = true
		}
	}
	if !hasToolCall || !hasToolResult || !hasAnswer {
		t.Errorf("hasToolCall=%v hasToolResult=%v hasAnswer=%v", hasToolCall, hasToolResult, hasAnswer)
	}
}

func TestAgentMaxIterations(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"input": "x"})
	llm := &mockLLMClient{
		responses: make([]mockResponse, 20),
	}
	for i := range llm.responses {
		llm.responses[i] = mockResponse{
			toolCalls: []client.ToolCall{{ID: "call_1", Name: "loop", Arguments: string(args)}},
		}
	}
	reg := tools.NewRegistry()
	reg.Register(&constTool{name: "loop", result: "looping"})
	a := New(llm, reg, AgentConfig{MaxIterations: 3, SystemPrompt: "You are helpful."})

	events := collectEvents(a.Run(context.Background(), "loop forever"))

	var hasError bool
	for _, e := range events {
		if e.Type == EventError {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected EventError for max iterations")
	}
}

func TestAgentContextCancel(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"input": "x"})
	llm := &mockLLMClient{
		responses: make([]mockResponse, 20),
	}
	for i := range llm.responses {
		llm.responses[i] = mockResponse{
			toolCalls: []client.ToolCall{{ID: "call_1", Name: "slow", Arguments: string(args)}},
		}
	}
	reg := tools.NewRegistry()
	reg.Register(&constTool{name: "slow", result: "done"})
	a := New(llm, reg, AgentConfig{MaxIterations: 20, SystemPrompt: "You are helpful."})

	ctx, cancel := context.WithCancel(context.Background())
	ch := a.Run(ctx, "do something")

	<-ch
	cancel()

	events := collectEvents(ch)
	if len(events) > 10 {
		t.Errorf("received %d events after cancel, expected few", len(events))
	}
}

func TestAgentMultiTurn(t *testing.T) {
	llm := &mockLLMClient{
		responses: []mockResponse{
			{content: "I'm an AI assistant."},
			{content: "You asked who I am. Now you ask about weather."},
		},
	}
	reg := tools.NewRegistry()
	a := New(llm, reg, AgentConfig{MaxIterations: 10, SystemPrompt: "You are helpful."})

	events1 := collectEvents(a.Run(context.Background(), "who are you?"))
	var answer1 string
	for _, e := range events1 {
		if e.Type == EventAnswer {
			answer1 = e.Content
		}
	}
	if answer1 == "" {
		t.Fatal("no answer for turn 1")
	}

	events2 := collectEvents(a.Run(context.Background(), "what about weather?"))
	var answer2 string
	for _, e := range events2 {
		if e.Type == EventAnswer {
			answer2 = e.Content
		}
	}
	if answer2 == "" {
		t.Fatal("no answer for turn 2")
	}

	if len(a.History()) < 4 {
		t.Errorf("history len = %d, want >= 4 (2 user + 2 assistant)", len(a.History()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./agent/ -v
```

Expected: compilation error — `New`, `AgentConfig`, etc. not defined.

- [ ] **Step 3: Implement agent core loop**

Create `agent/agent.go`:

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/tools"
	"golang.org/x/sync/errgroup"
)

type AgentConfig struct {
	MaxIterations int
	SystemPrompt  string
	Model         string
}

type Agent struct {
	llm      client.LLMClient
	registry *tools.Registry
	history  []client.Message
	config   AgentConfig
}

func New(llm client.LLMClient, registry *tools.Registry, config AgentConfig) *Agent {
	if config.MaxIterations <= 0 {
		config.MaxIterations = 10
	}
	return &Agent{
		llm:      llm,
		registry: registry,
		config:   config,
	}
}

func (a *Agent) History() []client.Message {
	return a.history
}

func (a *Agent) ClearHistory() {
	a.history = nil
}

func (a *Agent) Run(ctx context.Context, input string) <-chan AgentEvent {
	ch := make(chan AgentEvent, 32)

	a.history = append(a.history, client.Message{
		Role:    client.RoleUser,
		Content: input,
	})

	go func() {
		defer close(ch)
		a.runLoop(ctx, ch)
	}()

	return ch
}

func (a *Agent) runLoop(ctx context.Context, ch chan<- AgentEvent) {
	for i := 0; i < a.config.MaxIterations; i++ {
		if ctx.Err() != nil {
			return
		}

		messages := a.buildMessages()
		req := client.ChatRequest{
			Model:    a.config.Model,
			Messages: messages,
			Tools:    a.registry.ToolSchemas(),
		}

		streamCh, err := a.llm.ChatCompletion(ctx, req)
		if err != nil {
			ch <- AgentEvent{Type: EventError, Content: fmt.Sprintf("LLM error: %v", err)}
			return
		}

		var fullContent string
		var toolCalls []client.ToolCall

		for chunk := range streamCh {
			if ctx.Err() != nil {
				return
			}
			if chunk.Err != nil {
				ch <- AgentEvent{Type: EventError, Content: fmt.Sprintf("Stream error: %v", chunk.Err)}
				return
			}
			if chunk.Delta != "" {
				fullContent += chunk.Delta
				ch <- AgentEvent{Type: EventDelta, Content: chunk.Delta}
			}
			if len(chunk.ToolCalls) > 0 {
				toolCalls = chunk.ToolCalls
			}
		}

		if len(toolCalls) == 0 {
			a.history = append(a.history, client.Message{
				Role:    client.RoleAssistant,
				Content: fullContent,
			})
			ch <- AgentEvent{Type: EventAnswer, Content: fullContent}
			return
		}

		assistantMsg := client.Message{
			Role:      client.RoleAssistant,
			Content:   fullContent,
			ToolCalls: toolCalls,
		}
		a.history = append(a.history, assistantMsg)

		for _, tc := range toolCalls {
			var params map[string]any
			json.Unmarshal([]byte(tc.Arguments), &params)
			ch <- AgentEvent{
				Type:     EventToolCall,
				ToolName: tc.Name,
				ToolID:   tc.ID,
				Params:   params,
			}
		}

		results := a.executeTools(ctx, toolCalls, ch)
		for _, r := range results {
			a.history = append(a.history, client.Message{
				Role:       client.RoleTool,
				Content:    r.content,
				ToolCallID: r.toolCallID,
			})
		}
	}

	ch <- AgentEvent{Type: EventError, Content: "Agent reached maximum iterations"}
}

type toolResult struct {
	toolCallID string
	content    string
}

func (a *Agent) executeTools(ctx context.Context, toolCalls []client.ToolCall, ch chan<- AgentEvent) []toolResult {
	results := make([]toolResult, len(toolCalls))

	g, gctx := errgroup.WithContext(ctx)
	for i, tc := range toolCalls {
		i, tc := i, tc
		g.Go(func() error {
			tool, ok := a.registry.Get(tc.Name)
			if !ok {
				results[i] = toolResult{
					toolCallID: tc.ID,
					content:    fmt.Sprintf("Unknown tool: %s", tc.Name),
				}
				return nil
			}

			var params map[string]any
			json.Unmarshal([]byte(tc.Arguments), &params)

			output, err := tool.Execute(gctx, params)
			if err != nil {
				output = fmt.Sprintf("Tool error: %v", err)
			}

			results[i] = toolResult{
				toolCallID: tc.ID,
				content:    output,
			}
			return nil
		})
	}
	g.Wait()

	for _, r := range results {
		ch <- AgentEvent{
			Type:     EventToolResult,
			Content:  r.content,
			ToolName: "",
			ToolID:   r.toolCallID,
		}
	}

	return results
}

func (a *Agent) buildMessages() []client.Message {
	var msgs []client.Message
	if a.config.SystemPrompt != "" {
		msgs = append(msgs, client.Message{
			Role:    client.RoleSystem,
			Content: a.config.SystemPrompt,
		})
	}
	msgs = append(msgs, a.history...)
	return msgs
}
```

- [ ] **Step 4: Get errgroup dependency and run tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go get golang.org/x/sync && go test ./agent/ -v -count=1
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add agent/ go.mod go.sum
git commit -m "feat: ReAct agent core loop with multi-turn, tool execution, max iterations"
```

---

### Task 9: TUI Styles

**Files:**
- Create: `tui/styles.go`

- [ ] **Step 1: Define styles**

Create `tui/styles.go`:

```go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	appNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EEEEEE")).
			Background(lipgloss.Color("#333333")).
			Padding(0, 1)

	statusReadyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#22C55E")).
				Bold(true)

	statusThinkingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EAB308")).
				Bold(true)

	statusExecutingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#3B82F6")).
				Bold(true)

	thoughtStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#60A5FA")).
			Bold(true)

	actionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24")).
			Bold(true)

	observationStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#34D399")).
				Bold(true)

	answerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F9FAFB"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Bold(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444"))

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C3AED")).
				Bold(true)
)
```

- [ ] **Step 2: Get lipgloss dependency and verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go get github.com/charmbracelet/lipgloss && go build ./tui/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tui/styles.go go.mod go.sum
git commit -m "feat: TUI lipgloss style definitions"
```

---

### Task 10: TUI Status Bar

**Files:**
- Create: `tui/statusbar.go`

- [ ] **Step 1: Implement status bar**

Create `tui/statusbar.go`:

```go
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type AgentState int

const (
	StateReady AgentState = iota
	StateThinking
	StateExecuting
)

func (s AgentState) String() string {
	switch s {
	case StateReady:
		return "Ready"
	case StateThinking:
		return "Thinking"
	case StateExecuting:
		return "Executing"
	default:
		return "Unknown"
	}
}

func renderStatusBar(width int, provider, model string, state AgentState, elapsed time.Duration) string {
	title := appNameStyle.Render("Go-Agent v0.1")

	modelInfo := dimStyle.Render(fmt.Sprintf("%s/%s", provider, model))

	var stateStr string
	switch state {
	case StateReady:
		stateStr = statusReadyStyle.Render("● " + state.String())
	case StateThinking:
		stateStr = statusThinkingStyle.Render("◐ " + state.String())
	case StateExecuting:
		stateStr = statusExecutingStyle.Render("⚡ " + state.String())
	}

	var elapsedStr string
	if elapsed > 0 {
		elapsedStr = dimStyle.Render(fmt.Sprintf("⏱ %.1fs", elapsed.Seconds()))
	}

	left := fmt.Sprintf(" %s  │  %s", title, modelInfo)
	right := fmt.Sprintf("%s  %s ", stateStr, elapsedStr)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 0 {
		gap = 1
	}
	padding := ""
	for i := 0; i < gap; i++ {
		padding += " "
	}

	return statusBarStyle.Width(width).Render(left + padding + right)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go build ./tui/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tui/statusbar.go
git commit -m "feat: TUI status bar with state indicators"
```

---

### Task 11: TUI Viewport (Collapsible Panels)

**Files:**
- Create: `tui/viewport.go`

- [ ] **Step 1: Implement viewport with collapsible panels**

Create `tui/viewport.go`:

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type PanelType int

const (
	PanelUser PanelType = iota
	PanelThought
	PanelAction
	PanelObservation
	PanelAnswer
	PanelError
)

type Panel struct {
	Type      PanelType
	Title     string
	Content   string
	Collapsed bool
}

type MessageView struct {
	panels []Panel
}

func NewMessageView() *MessageView {
	return &MessageView{}
}

func (m *MessageView) AddPanel(p Panel) {
	m.panels = append(m.panels, p)
}

func (m *MessageView) LastPanel() *Panel {
	if len(m.panels) == 0 {
		return nil
	}
	return &m.panels[len(m.panels)-1]
}

func (m *MessageView) UpdateLastContent(content string) {
	if len(m.panels) > 0 {
		m.panels[len(m.panels)-1].Content = content
	}
}

func (m *MessageView) AppendToLast(delta string) {
	if len(m.panels) > 0 {
		m.panels[len(m.panels)-1].Content += delta
	}
}

func (m *MessageView) ToggleCollapse(index int) {
	if index >= 0 && index < len(m.panels) {
		p := &m.panels[index]
		if p.Type != PanelUser && p.Type != PanelAnswer {
			p.Collapsed = !p.Collapsed
		}
	}
}

func (m *MessageView) Render(width int) string {
	var sb strings.Builder
	for _, p := range m.panels {
		sb.WriteString(renderPanel(p, width))
		sb.WriteString("\n")
	}
	return sb.String()
}

func (m *MessageView) PanelCount() int {
	return len(m.panels)
}

func (m *MessageView) Clear() {
	m.panels = nil
}

func renderPanel(p Panel, width int) string {
	contentWidth := width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	switch p.Type {
	case PanelUser:
		label := userStyle.Render("You")
		return fmt.Sprintf("\n  %s: %s", label, p.Content)

	case PanelThought:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		header := thoughtStyle.Render(fmt.Sprintf("%s Thought", arrow))
		line := dimStyle.Render(strings.Repeat("─", maxInt(contentWidth-lipgloss.Width(header), 2)))
		if p.Collapsed {
			return fmt.Sprintf("  %s %s", header, line)
		}
		content := wrapText(p.Content, contentWidth)
		return fmt.Sprintf("  %s %s\n%s", header, line, indent(content, "    "))

	case PanelAction:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		header := actionStyle.Render(fmt.Sprintf("%s Action: %s", arrow, p.Title))
		line := dimStyle.Render(strings.Repeat("─", maxInt(contentWidth-lipgloss.Width(header), 2)))
		if p.Collapsed {
			return fmt.Sprintf("  %s %s", header, line)
		}
		content := wrapText(p.Content, contentWidth)
		return fmt.Sprintf("  %s %s\n%s", header, line, indent(content, "    "))

	case PanelObservation:
		arrow := "▾"
		if p.Collapsed {
			arrow = "▸"
		}
		header := observationStyle.Render(fmt.Sprintf("%s Observation", arrow))
		line := dimStyle.Render(strings.Repeat("─", maxInt(contentWidth-lipgloss.Width(header), 2)))
		if p.Collapsed {
			return fmt.Sprintf("  %s %s", header, line)
		}
		content := wrapText(p.Content, contentWidth)
		return fmt.Sprintf("  %s %s\n%s", header, line, indent(content, "    "))

	case PanelAnswer:
		label := answerStyle.Render("Assistant")
		content := wrapText(p.Content, contentWidth)
		return fmt.Sprintf("\n  %s:\n%s", label, indent(content, "    "))

	case PanelError:
		label := errorStyle.Render("Error")
		return fmt.Sprintf("\n  %s: %s", label, p.Content)
	}
	return ""
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		for len(line) > width {
			lines = append(lines, line[:width])
			line = line[width:]
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go build ./tui/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tui/viewport.go
git commit -m "feat: TUI viewport with collapsible thought/action/observation panels"
```

---

### Task 12: TUI Input Component

**Files:**
- Create: `tui/input.go`

- [ ] **Step 1: Implement input component**

Create `tui/input.go`:

```go
package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

func newInputArea(width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Ask something... (Ctrl+C cancel | ESC quit)"
	ta.Prompt = inputPromptStyle.Render("> ")
	ta.CharLimit = 4096
	ta.SetWidth(width - 4)
	ta.SetHeight(1)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.Focus()
	return ta
}
```

- [ ] **Step 2: Get bubbles dependency and verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go get github.com/charmbracelet/bubbles && go build ./tui/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tui/input.go go.mod go.sum
git commit -m "feat: TUI textarea input component"
```

---

### Task 13: TUI Main App (bubbletea Model)

**Files:**
- Create: `tui/app.go`

- [ ] **Step 1: Implement main bubbletea Model**

Create `tui/app.go`:

```go
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fengxuan/go-agent/agent"
)

type RunAgentFunc func(ctx context.Context, input string) <-chan agent.AgentEvent
type ClearHistoryFunc func()
type SwitchProviderFunc func(provider string) error

type AppConfig struct {
	Provider string
	Model    string
	RunAgent RunAgentFunc
	ClearHistory ClearHistoryFunc
	SwitchProvider SwitchProviderFunc
}

type agentEventMsg agent.AgentEvent
type agentDoneMsg struct{}
type tickMsg time.Time

type Model struct {
	config      AppConfig
	viewport    viewport.Model
	textarea    textarea.Model
	spinner     spinner.Model
	messageView *MessageView

	state       AgentState
	startTime   time.Time
	elapsed     time.Duration
	width       int
	height      int

	agentCancel context.CancelFunc
	agentCh     <-chan agent.AgentEvent

	answerBuf   string

	ready       bool
}

func NewModel(config AppConfig) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	return Model{
		config:      config,
		spinner:     s,
		messageView: NewMessageView(),
		state:       StateReady,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 1
		inputHeight := 3
		vpHeight := m.height - headerHeight - inputHeight - 2

		if !m.ready {
			m.viewport = viewport.New(m.width, vpHeight)
			m.textarea = newInputArea(m.width)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpHeight
			m.textarea.SetWidth(m.width - 4)
		}

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.state != StateReady {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.state = StateReady
				m.messageView.AddPanel(Panel{
					Type:    PanelError,
					Content: "Interrupted by user",
				})
				m.updateViewport()
				m.textarea.Focus()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEsc:
			if m.state != StateReady {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.state = StateReady
				m.textarea.Focus()
				return m, nil
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state == StateReady {
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m, nil
				}
				m.textarea.Reset()

				if strings.HasPrefix(input, "/") {
					return m.handleSlashCommand(input)
				}

				m.messageView.AddPanel(Panel{
					Type:    PanelUser,
					Content: input,
				})

				m.state = StateThinking
				m.startTime = time.Now()

				ctx, cancel := context.WithCancel(context.Background())
				m.agentCancel = cancel
				m.agentCh = m.config.RunAgent(ctx, input)
				m.answerBuf = ""

				m.textarea.Blur()
				m.updateViewport()
				return m, tea.Batch(waitForEvent(m.agentCh), tickCmd())
			}
		}

	case agentEventMsg:
		e := agent.AgentEvent(msg)
		switch e.Type {
		case agent.EventDelta:
			if m.answerBuf == "" {
				m.messageView.AddPanel(Panel{
					Type: PanelAnswer,
				})
			}
			m.answerBuf += e.Content
			m.messageView.UpdateLastContent(m.answerBuf)

		case agent.EventThought:
			m.messageView.AddPanel(Panel{
				Type:    PanelThought,
				Content: e.Content,
			})

		case agent.EventToolCall:
			m.state = StateExecuting
			params, _ := json.MarshalIndent(e.Params, "", "  ")
			m.messageView.AddPanel(Panel{
				Type:    PanelAction,
				Title:   e.ToolName,
				Content: string(params),
			})

		case agent.EventToolResult:
			m.state = StateThinking
			content := e.Content
			if len(content) > 500 {
				content = content[:500] + "\n... (truncated)"
			}
			m.messageView.AddPanel(Panel{
				Type:    PanelObservation,
				Content: content,
			})

		case agent.EventAnswer:
			if m.answerBuf == "" {
				m.messageView.AddPanel(Panel{
					Type:    PanelAnswer,
					Content: e.Content,
				})
			}

		case agent.EventError:
			m.messageView.AddPanel(Panel{
				Type:    PanelError,
				Content: e.Content,
			})
		}

		m.updateViewport()
		if m.agentCh != nil {
			cmds = append(cmds, waitForEvent(m.agentCh))
		}

	case agentDoneMsg:
		m.state = StateReady
		m.elapsed = time.Since(m.startTime)
		m.agentCh = nil
		m.agentCancel = nil
		m.answerBuf = ""
		m.textarea.Focus()
		m.updateViewport()

	case tickMsg:
		if m.state != StateReady {
			m.elapsed = time.Since(m.startTime)
			cmds = append(cmds, tickCmd())
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.state == StateReady {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	statusBar := renderStatusBar(m.width, m.config.Provider, m.config.Model, m.state, m.elapsed)
	content := m.viewport.View()

	inputBorder := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Width(m.width - 2)

	input := inputBorder.Render(m.textarea.View())

	return fmt.Sprintf("%s\n%s\n%s", statusBar, content, input)
}

func (m *Model) updateViewport() {
	m.viewport.SetContent(m.messageView.Render(m.width))
	m.viewport.GotoBottom()
}

func (m Model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/clear":
		m.messageView.Clear()
		if m.config.ClearHistory != nil {
			m.config.ClearHistory()
		}
		m.updateViewport()
		return m, nil

	case "/model":
		if len(parts) < 2 {
			m.messageView.AddPanel(Panel{
				Type:    PanelError,
				Content: "Usage: /model <provider>",
			})
		} else {
			provider := parts[1]
			if m.config.SwitchProvider != nil {
				if err := m.config.SwitchProvider(provider); err != nil {
					m.messageView.AddPanel(Panel{
						Type:    PanelError,
						Content: fmt.Sprintf("Failed to switch: %v", err),
					})
				} else {
					m.config.Provider = provider
					m.messageView.AddPanel(Panel{
						Type:    PanelAnswer,
						Content: fmt.Sprintf("Switched to provider: %s", provider),
					})
				}
			}
		}
		m.updateViewport()
		return m, nil

	case "/quit", "/exit":
		return m, tea.Quit

	default:
		m.messageView.AddPanel(Panel{
			Type:    PanelError,
			Content: fmt.Sprintf("Unknown command: %s", cmd),
		})
		m.updateViewport()
		return m, nil
	}
}

func waitForEvent(ch <-chan agent.AgentEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg(event)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
```

- [ ] **Step 2: Get bubbletea dependency and verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go get github.com/charmbracelet/bubbletea && go build ./tui/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add tui/app.go go.mod go.sum
git commit -m "feat: TUI main bubbletea Model with event bridge and key handling"
```

---

### Task 14: Main Entry Point

**Files:**
- Create: `main.go`

- [ ] **Step 1: Implement main.go**

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/tools"
	"github.com/fengxuan/go-agent/tui"
)

func main() {
	cfgPath := filepath.Join(os.Getenv("HOME"), ".go-agent", "config.yaml")
	if envPath := os.Getenv("GO_AGENT_CONFIG"); envPath != "" {
		cfgPath = envPath
	}

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config from %s: %v\n", cfgPath, err)
		fmt.Fprintf(os.Stderr, "Create a config file or set GO_AGENT_CONFIG env var.\n")
		os.Exit(1)
	}
	config.ApplyEnvOverrides(cfg)

	providerCfg, ok := cfg.Providers[cfg.DefaultProvider]
	if !ok {
		fmt.Fprintf(os.Stderr, "Provider %q not found in config\n", cfg.DefaultProvider)
		os.Exit(1)
	}

	llmClient := createClient(cfg.DefaultProvider, providerCfg)

	registry := tools.NewRegistry()
	registry.Register(tools.NewTavilySearch(cfg.Tools.Tavily.APIKey, ""))
	registry.Register(tools.NewShellExec(cfg.Tools.Shell.BlockedCommands))
	registry.Register(tools.NewReadFile(cfg.Tools.File.MaxReadSize))
	registry.Register(tools.NewWriteFile())

	systemPrompt := `You are a helpful AI assistant with access to tools. 
When you need to find information, use the tavily_search tool.
When you need to run commands, use the shell_exec tool.
When you need to read files, use the read_file tool.
When you need to write files, use the write_file tool.
Always explain your reasoning before using tools.`

	ag := agent.New(llmClient, registry, agent.AgentConfig{
		MaxIterations: cfg.MaxIterations,
		SystemPrompt:  systemPrompt,
		Model:         providerCfg.Model,
	})

	appCfg := tui.AppConfig{
		Provider: cfg.DefaultProvider,
		Model:    providerCfg.Model,
		RunAgent: func(ctx context.Context, input string) <-chan agent.AgentEvent {
			return ag.Run(ctx, input)
		},
		ClearHistory: func() {
			ag.ClearHistory()
		},
		SwitchProvider: func(provider string) error {
			pc, ok := cfg.Providers[provider]
			if !ok {
				return fmt.Errorf("provider %q not configured", provider)
			}
			newClient := createClient(provider, pc)
			*ag = *agent.New(newClient, registry, agent.AgentConfig{
				MaxIterations: cfg.MaxIterations,
				SystemPrompt:  systemPrompt,
				Model:         pc.Model,
			})
			return nil
		},
	}

	p := tea.NewProgram(
		tui.NewModel(appCfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func createClient(provider string, cfg config.ProviderConfig) client.LLMClient {
	switch provider {
	case "anthropic":
		return client.NewAnthropicClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	default:
		return client.NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/fengxuan/Desktop/go-agent && go build -o go-agent .
```

Expected: binary `go-agent` built with no errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add main.go
git commit -m "feat: main entry point wiring config, client, tools, agent, and TUI"
```

---

### Task 15: Default Config + Manual Testing

**Files:**
- Create: `config.example.yaml`

- [ ] **Step 1: Create example config**

Create `config.example.yaml`:

```yaml
default_provider: openai
max_iterations: 10

providers:
  openai:
    api_key: sk-your-key-here
    base_url: https://api.openai.com/v1
    model: gpt-4o

  deepseek:
    api_key: sk-your-key-here
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat

  anthropic:
    api_key: sk-ant-your-key-here
    base_url: https://api.anthropic.com
    model: claude-sonnet-4-20250514

tools:
  tavily:
    api_key: tvly-your-key-here
  shell:
    allowed_commands: []
    blocked_commands:
      - "rm -rf /"
      - "sudo"
      - "mkfs"
  file:
    max_read_size: 1048576
```

- [ ] **Step 2: Create gitignore**

Create `.gitignore`:

```
go-agent
*.exe
.DS_Store
```

- [ ] **Step 3: Set up config and run manual test**

```bash
mkdir -p ~/.go-agent
cp /Users/fengxuan/Desktop/go-agent/config.example.yaml ~/.go-agent/config.yaml
# Edit ~/.go-agent/config.yaml with real API keys
# Then run:
cd /Users/fengxuan/Desktop/go-agent && go run .
```

Manual test checklist:
1. App starts with full-screen TUI, status bar shows "Ready"
2. Type a simple question → agent answers directly, status cycles Thinking → Ready
3. Ask "search for golang news" → agent calls tavily_search, shows Thought → Action → Observation → Answer panels
4. Press `Tab` on a panel → it collapses/expands
5. While agent is thinking, press `Ctrl+C` → agent stops, returns to input
6. Type `/clear` → conversation clears
7. Type `/model deepseek` → provider switches (if configured)
8. Press `ESC` when idle → app exits cleanly

- [ ] **Step 4: Commit**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add config.example.yaml .gitignore
git commit -m "feat: example config and gitignore"
```

---

### Task 16: Run Full Test Suite

- [ ] **Step 1: Run all tests**

```bash
cd /Users/fengxuan/Desktop/go-agent && go test ./... -v -count=1
```

Expected: all tests PASS across config/, client/, tools/, agent/ packages.

- [ ] **Step 2: Run vet and check for issues**

```bash
cd /Users/fengxuan/Desktop/go-agent && go vet ./...
```

Expected: no issues.

- [ ] **Step 3: Final commit if any fixes were needed**

```bash
cd /Users/fengxuan/Desktop/go-agent
git add -A
git status
# Only commit if there are changes
git diff --cached --stat && git commit -m "fix: address issues found in full test suite"
```
