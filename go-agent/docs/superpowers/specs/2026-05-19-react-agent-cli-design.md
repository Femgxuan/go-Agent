# Go-Agent: ReAct AI Agent CLI - Design Spec

## Overview

A terminal-based AI Agent application written in Go, implementing the ReAct (Reason-Act-Observe) loop with a polished TUI inspired by hermes/OpenClaw. Supports multi-provider LLM backends, multi-turn conversations, and an extensible tool system.

**Project path**: `/Users/fengxuan/Desktop/go-agent`

## Project Structure

```
go-agent/
├── main.go                  # Entry point: parse config, init deps, start bubbletea
├── config/
│   └── config.go            # Config loading: YAML file + env var override
├── client/
│   ├── provider.go          # LLMClient interface definition
│   ├── openai.go            # OpenAI-compatible implementation (custom base URL)
│   └── anthropic.go         # Claude API implementation
├── agent/
│   ├── agent.go             # ReAct core loop
│   └── message.go           # Message types (system/user/assistant/tool)
├── tools/
│   ├── registry.go          # Tool registry + JSON Schema generation
│   ├── tavily.go            # Tavily search
│   ├── shell.go             # Shell command execution
│   ├── readfile.go          # File reading
│   └── writefile.go         # File writing
├── tui/
│   ├── app.go               # bubbletea Model (Init/Update/View)
│   ├── input.go             # Input component
│   ├── viewport.go          # Message display area (collapsible panels)
│   ├── statusbar.go         # Bottom status bar
│   └── styles.go            # lipgloss style definitions
└── go.mod
```

## 1. ReAct Core Loop

### Flow

```
User Input
   │
   ▼
┌─────────────────────────────────────────┐
│  Build messages: system prompt +        │
│  history + user msg + tools schema      │
└────────────────┬────────────────────────┘
                 │
                 ▼
         ┌──────────────┐
         │  Call LLM API │◄─────────────┐
         └──────┬───────┘              │
                │                      │
                ▼                      │
        ┌───────────────┐              │
        │ Parse response │              │
        └───┬───────┬───┘              │
            │       │                  │
   has tool_calls  no tool_calls       │
            │       │                  │
            ▼       ▼                  │
   ┌────────────┐  ┌──────────┐       │
   │ Execute     │  │ Final    │       │
   │ tools       │  │ answer   │       │
   │ (concurrent)│  │ → done   │       │
   └─────┬──────┘  └──────────┘       │
         │                            │
         ▼                            │
   ┌──────────────────┐               │
   │ Append tool       │               │
   │ results to history├───────────────┘
   └──────────────────┘
        (next iteration, up to max_iterations)
```

### Agent Interface

```go
type AgentEvent struct {
    Type    EventType  // EventThought, EventToolCall, EventToolResult, EventAnswer, EventError
    Content string
    ToolName string    // only for EventToolCall/EventToolResult
    Params   map[string]any // only for EventToolCall
}

type Agent struct {
    client   client.LLMClient
    registry *tools.Registry
    history  []Message        // multi-turn conversation history
    config   AgentConfig
}

func (a *Agent) Run(ctx context.Context, input string) <-chan AgentEvent
func (a *Agent) ClearHistory()
```

### Key Design Decisions

- **`Run()` returns a read-only channel**: TUI consumes events via `tea.Cmd` that listens on this channel. Decouples agent logic from UI completely.
- **Context propagation**: `ctx` flows from TUI layer. User pressing `Ctrl+C`/`ESC` triggers `cancel()`, agent checks `ctx.Err()` at each iteration boundary.
- **Concurrent tool execution**: When LLM returns multiple tool_calls, execute them concurrently via `errgroup`, reassemble results in original order.
- **Max iterations**: Default 10, configurable. Prevents infinite loops.

### Multi-turn Conversation

- Agent maintains `history []Message` across turns. Each user input appends a user message; LLM replies append assistant messages (including tool_calls); tool results append tool messages.
- Full history is sent with each LLM request to maintain context.
- **Context window management**: When history token count approaches model limit, retain system prompt + most recent N turns, discard earliest middle turns. Token count estimated as `len(content) / 4` (1 token ≈ 4 chars); no tiktoken dependency in v1.
- **`/clear` command**: Clears history and viewport, starts fresh.

## 2. Multi-Provider Client

### Interface

```go
type LLMClient interface {
    ChatCompletion(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)
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
```

### Implementations

- **OpenAI-compatible** (`openai.go`): Covers OpenAI, DeepSeek, Qwen, Ollama, vLLM — any service implementing `/v1/chat/completions`. Configurable via `base_url`.
- **Anthropic** (`anthropic.go`): Adapts to `/v1/messages` API. Internal message format conversion between OpenAI tool_calls and Anthropic tool_use blocks.

### Unified Streaming

All responses use channels internally, even for non-streaming scenarios (single chunk). Keeps upper-layer code uniform.

## 3. TUI Design

### Layout (hermes/OpenClaw style)

```
┌──────────────────────────────────────────────┐
│  Go-Agent v0.1  │ model: gpt-4o  │ ● Ready  │  ← Top status bar
├──────────────────────────────────────────────┤
│                                              │
│  You: What's the weather in Hangzhou?        │
│                                              │
│  ▸ Thought ──────────────────────────────    │  ← Collapsible (blue)
│    I need to search for weather info         │
│                                              │
│  ▸ Action: tavily_search ────────────────    │  ← Collapsible (yellow)
│    {"query": "杭州今天天气"}                    │
│    ⏱ 1.2s                                   │
│                                              │
│  ▸ Observation ──────────────────────────    │  ← Collapsible (green)
│    Hangzhou: sunny, 28°C...                  │
│                                              │
│  Assistant:                                  │  ← Answer (white)
│    Hangzhou is sunny today, around 28°C.     │
│                                              │
├──────────────────────────────────────────────┤
│  > Ask something... (Ctrl+C cancel | ESC quit)│ ← Input area
└──────────────────────────────────────────────┘
```

### Components

| Component | Library | Role |
|-----------|---------|------|
| Status bar | lipgloss | Model name, agent state (Ready/Thinking/Executing), elapsed time |
| Viewport | bubbles/viewport | Scrollable message history with collapsible panels |
| Input | bubbles/textarea | Multi-line input with prompt |
| Spinner | bubbles/spinner | Shown in panel title during LLM thinking / tool execution |

### Color Scheme

- Thought → Blue
- Action → Yellow
- Observation → Green
- Answer → White/Default
- Error → Red

### Interaction

- **Collapsible panels**: `Tab` key toggles expand/collapse on focused panel
- **Streaming render**: LLM tokens append in real-time, viewport auto-scrolls to bottom
- **Interrupt**: `Ctrl+C` during agent run → cancel current task, return to input prompt. `Ctrl+C`/`ESC` when idle → exit program.
- **Slash commands**: `/model <provider>` switch provider, `/clear` reset conversation

### Bubbletea Message Flow

```
AgentEvent channel → tea.Cmd (waitForEvent) → tea.Msg → Update() → View()
```

A `waitForEvent` Cmd continuously listens on the Agent's event channel. Each event becomes a `tea.Msg` that triggers UI re-render.

## 4. Tool System

### Interface

```go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any
    Execute(ctx context.Context, params map[string]any) (string, error)
}

type Registry struct {
    tools map[string]Tool
}

func (r *Registry) Register(t Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) ToolSchemas() []ToolSchema  // OpenAI function calling format
```

### Built-in Tools

| Tool | Params | Safety |
|------|--------|--------|
| `tavily_search` | `query: string`, `max_results: int (default 5)` | Requires `TAVILY_API_KEY` |
| `shell_exec` | `command: string`, `timeout: int (default 30s)` | User confirmation required. Blocks `rm -rf /`, `sudo`, etc. |
| `read_file` | `path: string` | Max 1MB. No `..` path traversal. |
| `write_file` | `path: string`, `content: string` | User confirmation required with diff preview. |

### Confirmation Mechanism

`shell_exec` and `write_file` are high-risk tools. When agent requests them, TUI shows a confirmation panel displaying the command/content. User presses `y` to approve or `n` to reject. Rejection returns "User denied this action" as tool result to LLM.

### Extension

Implement `Tool` interface + `registry.Register(tool)` to add new tools. No changes needed in agent or TUI.

## 5. Configuration

### File: `~/.go-agent/config.yaml`

```yaml
default_provider: openai
max_iterations: 10

providers:
  openai:
    api_key: sk-xxx              # overridden by OPENAI_API_KEY env var
    base_url: https://api.openai.com/v1
    model: gpt-4o

  deepseek:
    api_key: sk-xxx
    base_url: https://api.deepseek.com/v1
    model: deepseek-chat

  anthropic:
    api_key: sk-ant-xxx          # overridden by ANTHROPIC_API_KEY env var
    base_url: https://api.anthropic.com
    model: claude-sonnet-4-20250514

tools:
  tavily:
    api_key: tvly-xxx            # overridden by TAVILY_API_KEY env var
  shell:
    allowed_commands: []         # empty = all allowed (with confirmation)
    blocked_commands: ["rm -rf /", "sudo", "mkfs"]
  file:
    max_read_size: 1048576       # 1MB
```

### Environment Variable Override

Format: env var name takes precedence over YAML value. Standard names: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `TAVILY_API_KEY`, `OPENAI_BASE_URL`.

## 6. Error Handling

- **LLM API errors**: Network timeout / 429 rate limit → exponential backoff retry (max 3 attempts, 1s/2s/4s). Non-retryable errors (401/403) → report to TUI immediately.
- **Tool execution errors**: Error message returned as Observation to LLM, letting agent decide whether to retry differently.
- **JSON parse errors**: Invalid tool_calls JSON → display raw text as assistant reply.
- **Max iteration guard**: Forced stop with "Agent reached maximum iterations" message.

## 7. Testing Strategy

| Layer | Approach |
|-------|----------|
| `client/` | Mock HTTP server, verify request format and streaming parse |
| `agent/` | Mock LLMClient, verify ReAct loop (single answer / multi-tool / max iterations / ctx cancel) |
| `tools/` | Unit test each tool (tavily with mock server, shell/file with temp dir) |
| `tui/` | Manual testing for interaction experience |

## 8. Dependencies (go.mod)

```
github.com/charmbracelet/bubbletea     // TUI framework
github.com/charmbracelet/bubbles       // spinner, viewport, textarea components
github.com/charmbracelet/lipgloss      // Styling
gopkg.in/yaml.v3                       // Config parsing
golang.org/x/sync                      // errgroup for concurrent tools
```
