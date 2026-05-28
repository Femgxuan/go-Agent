# go-agent

A terminal-based AI agent CLI tool built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). It supports multiple LLM providers, tools (shell, file I/O, web search), slash commands, and a pluggable skills system.

## Prerequisites

- **Go 1.26+**
- An API key for at least one supported LLM provider (OpenAI, DeepSeek, or Anthropic)
- (Optional) A [Tavily](https://tavily.com) API key for web search

## Quick Start

```bash
# Clone and build
git clone https://github.com/fengxuan/go-agent.git
cd go-agent/go-agent
go build -o go-agent .

# Create config directory and file
mkdir -p ~/.go-agent
cp config.example.yaml ~/.go-agent/config.yaml
```

Edit `~/.go-agent/config.yaml`, replace the placeholder API keys with your own, then run:

```bash
./go-agent
```

## Configuration

The config file lives at `~/.go-agent/config.yaml` (override with `GO_AGENT_CONFIG` env var).

```yaml
default_provider: openai          # default LLM provider
max_iterations: 10                # max tool-calling rounds per user input

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
    api_key: tvly-your-key-here   # optional, for web search
  shell:
    blocked_commands:             # commands the shell tool will refuse to run
      - "rm -rf /"
      - "sudo"
      - "mkfs"
  file:
    max_read_size: 1048576        # max bytes per read_file call (1 MiB)
```

### Environment variable overrides

You can set API keys via environment variables instead of writing them in the config file:

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `DEEPSEEK_API_KEY`
- `TAVILY_API_KEY`

## Usage

Once launched, you're in a full-screen TUI. Type your prompt and press Enter — the agent reasons, calls tools, and streams responses back.

### Slash commands

| Command | Description |
|---|---|
| `/help` | Show all available commands |
| `/provider <name>` | Switch to a different LLM provider |
| `/model <name>` | Switch to a different model |
| `/skills` | List all loaded skills |
| `/skill <name>` | Activate a skill for the next prompt |
| `/create-skill` | Create a new skill |
| `/delete-skill` | Delete an existing skill |
| `/update-skill` | Update an existing skill |
| `/tools` | List all registered tools |
| `/prompt` | Show the last assembled system prompt |
| `/reload` | Reload skills and rules from disk |
| `/clear` | Clear conversation history |
| `/quit` | Exit the program |

### Skills

Skills are markdown files with YAML frontmatter that extend the agent's system prompt. They are auto-matched by keywords once you type a matching query, or you can manually activate them with `/skill <name>`.

**User skills** live in `~/.go-agent/skills/` (global, available everywhere).
**Project skills** live in `.go-agent/skills/` (local to a project).

### Rules

Rules files (`~/.go-agent/rules/` and `.go-agent/rules/`) are loaded into the system prompt automatically — similar to CLAUDE.md conventions. Use them for project-level or user-level instructions.

## Project Structure

```
go-agent/
├── main.go              # Entry point
├── config/              # YAML config parsing & env overrides
├── agent/               # Core agent loop (tool calling, conversation history)
├── client/              # LLM provider clients (OpenAI, Anthropic)
├── tools/               # Built-in tools: shell_exec, read_file, write_file, tavily_search
├── commands/            # Slash command framework & built-in commands
│   └── builtin/         # help, clear, quit, provider, model, skills, etc.
├── skills/              # Skill loading, matching, indexing & CRUD
├── rules/               # Rules file loader
├── prompt/              # System prompt builder with source tracing
├── runtime/             # Orchestrator wiring all subsystems together
└── tui/                 # Terminal UI (Bubble Tea model, input, status bar)
```

## License

MIT
