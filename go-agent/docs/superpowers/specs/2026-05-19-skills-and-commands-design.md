# Skills System & Slash Commands Design Spec

## Overview

Phase 2 of go-agent: add an extensible Skills system, a Rules system, a Slash command system with TUI autocomplete, a PromptBuilder with source tracking, and a Runtime coordinator. Goal is a long-term evolvable agent runtime kernel, not a demo.

## Constraints

- No embedding / vector DB / reranker
- No heavyweight dependencies unless absolutely necessary
- No complex plugin system
- No hardcoded skill content in the binary
- No opaque prompt concatenation — all sources must be traceable
- Do not break existing chat and tool call flow
- First phase: get the kernel skeleton right, not pile on features

---

## 1. Skill Storage Structure

### Two Forms, Unified Interface

**Single file (minimum viable):**

```
~/.go-agent/skills/code-review.md
```

**Directory (engineering-grade):**

```
~/.go-agent/skills/code-review/
├── SKILL.md           # Entry point: frontmatter + body
├── scripts/           # Private scripts (NOT auto-registered as tools)
│   └── analyze.go
├── templates/         # Output templates
│   └── review-report.md
├── references/        # Reference documents
│   └── style-guide.md
└── assets/            # Other resources
```

### Loading Priority (low → high)

1. Built-in skills (compiled via `embed.FS`)
2. User-level: `~/.go-agent/skills/`
3. Project-level: `./.go-agent/skills/`

Same-name skill at higher priority overrides lower.

### Frontmatter (v1 Fields)

```yaml
---
name: code-review
description: "Structured code review process"
version: "1.0"
tags: [review, quality, golang]
triggers:
  keywords: ["review", "code review"]
  patterns: ["help me review", "look at this code"]
# --- Extended fields (optional) ---
scope: project          # user | project | global
priority: 10            # higher number = higher priority
tools: [read_file, shell_exec]   # suggested tools (advisory, not enforced)
scripts:
  - name: analyze
    path: scripts/analyze.go
    description: "Static analysis helper"
inputs:
  - name: file_path
    type: string
    required: true
outputs:
  - name: review_report
    type: markdown
---
```

### Body Sections (write as needed)

```markdown
## Purpose
What this skill does in one sentence.

## When to use
Applicable scenarios.

## When NOT to use
Inapplicable scenarios — prevents false matches.

## Inputs
What the user is expected to provide.

## Workflow
Step-by-step execution flow.

## Constraints
Restrictions and caveats during execution.

## Output format
Expected output format.

## Examples
Input → output examples.
```

### Core Principle

**Scripts are skill-private resources, not auto-registered tools.** A skill may reference scripts in its Workflow section; the runtime loads and executes them on demand. Scripts never appear in the global tool list.

---

## 2. Three-Layer Architecture

```
┌─────────────────────────────────────┐
│  Skill Layer (task capability packs) │
│  SKILL.md + resources                │
├──────────────────────────────────────┤
│  Script Layer (skill-private logic)  │
│  scripts/*.go (Yaegi interpreted)    │
├──────────────────────────────────────┤
│  Tool Layer (global tools, registry) │
│  read_file, shell_exec, tavily...    │
└──────────────────────────────────────┘
```

### Three Invocation Paths

| Path | Description | Example |
|------|-------------|---------|
| **Prompt-only** | Skill injects prompt text only, LLM reasons on its own | code-review skill injects review criteria, LLM reads files itself |
| **Tool-guiding** | Skill suggests which registered tools to use in Workflow | "Use read_file to read the target, run go vet via shell_exec" |
| **Scripted** | Skill declares private scripts, runtime executes on demand | scripts/analyze.go does custom analysis, result goes back to LLM |

Phase 1 implements prompt-only and tool-guiding. Scripted path: interface reserved, implementation via Yaegi later.

---

## 3. Skill Loading & Matching

### SkillIndex

```go
type SkillIndex struct {
    skills []SkillMeta
}

type SkillMeta struct {
    Name        string
    Description string
    Tags        []string
    Triggers    TriggerConfig
    Scope       string       // "user" | "project" | "global"
    Priority    int
    Source      string       // "builtin" | "user" | "project"
    BasePath    string
}

type TriggerConfig struct {
    Keywords []string
    Patterns []string        // regex patterns
}
```

### Matching Rules (v1, deterministic)

1. Exact name match: `/skill code-review` → direct hit
2. Keyword match: user input contains a word from `triggers.keywords`
3. Pattern match: user input matches a regex from `triggers.patterns`
4. Tag match: set intersection with query tokens
5. Description substring match: fallback

Results sorted by priority, return top-N (default 3).

### Two Injection Modes

- **Auto-match**: on every user input, SkillIndex runs matching; hits are injected into the current prompt
- **Explicit**: `/skill <name>` forces injection of a specific skill for the next request

---

## 4. Rules System

```
~/.go-agent/RULES.md        # User-level rules (personal preferences)
./.go-agent/RULES.md        # Project-level rules (team conventions)
```

### Rules vs Skills

| | Rules | Skills |
|---|---|---|
| Nature | Stable behavioral constraints | Task-specific capability packs |
| Lifecycle | Injected on every request | Matched and injected on demand |
| Content | "Always use gofmt", "Never delete tests" | "How to do code review", "Migration script workflow" |
| File | RULES.md (single file) | SKILL.md + resource directory |

Rules content is unconditionally injected on every request. No matching needed.

---

## 5. PromptBuilder

```go
type Builder struct {
    sources []Source
}

type Source struct {
    Name     string   // "base_system", "user_rules", "project_rules", "skill:code-review"
    Type     string   // "system" | "rules" | "skill" | "user"
    Content  string
    Priority int
}
```

### Assembly Order (top → bottom)

```
┌─────────────────────────────┐
│ 1. Base System Prompt       │  ← Hardcoded role definition
├─────────────────────────────┤
│ 2. User Rules               │  ← ~/.go-agent/RULES.md
├─────────────────────────────┤
│ 3. Project Rules            │  ← ./.go-agent/RULES.md
├─────────────────────────────┤
│ 4. Matched Skills           │  ← Auto-matched + explicitly activated skill content
├─────────────────────────────┤
│ 5. Tool Descriptions        │  ← Registered tool schemas
├─────────────────────────────┤
│ 6. User Input               │  ← Current user message
└─────────────────────────────┘
```

### Key Properties

- Every source carries a Name for traceability
- Skill injection is per-turn only — does not pollute conversation history
- `/prompt` command shows the full assembly result with source attribution

---

## 6. Observability

Each request produces a `PromptTrace`:

```go
type Trace struct {
    Timestamp     time.Time
    RulesLoaded   []string   // ["user_rules", "project_rules"]
    SkillsMatched []string   // ["code-review (auto)", "golang-best-practices (explicit)"]
    Sources       []string   // All injected source names
    TotalTokens   int        // Estimated prompt token count
}
```

- New agent event type `EventPromptTrace` — TUI can optionally display
- `/prompt` outputs the most recent full trace
- `/skills` lists all loaded skills with source and priority

---

## 7. Slash Command System

### CommandRegistry

```go
type Command interface {
    Name() string
    Aliases() []string
    Description() string
    Usage() string
    Execute(ctx CommandContext) (CommandResult, error)
}

type CommandContext struct {
    Args    []string
    Runtime *Runtime
    Output  func(string)
}

type CommandResult struct {
    Message string
    Action  CommandAction  // None | Quit | ClearScreen
}

type CommandRegistry struct {
    commands map[string]Command
    aliases  map[string]string
}
```

### Design Principles

- Commands registered during Runtime init, not in TUI layer
- TUI only: detect `/` prefix → call `Runtime.ExecuteCommand()` → render result
- Adding a command = implement `Command` interface + one Register call. Zero TUI changes.

### v1 Built-in Commands

| Command | Aliases | Function |
|---------|---------|----------|
| `/help` | `/h`, `/?` | List all commands with usage |
| `/clear` | `/c` | Clear conversation history and panels |
| `/provider <name>` | `/p` | Switch LLM provider |
| `/model <name>` | `/m` | Switch model within provider |
| `/skills` | `/ss` | List all loaded skills |
| `/skill <name>` | `/s` | Activate a skill for the next request |
| `/reload` | `/r` | Reload skills and rules from disk |
| `/prompt` | | Show most recent prompt assembly trace |
| `/tools` | `/t` | List all registered tools |
| `/quit` | `/q`, `/exit` | Exit the program |

---

## 8. Slash Command Autocomplete

### Interaction Flow

```
User types "/"
  │
  └─→ Completion panel appears (floating above input)
       ├─ Shows all commands + short description
       ├─ First item highlighted
       │
       ├─ Continue typing → real-time prefix filter
       │   "/sk" → shows only /skills, /skill
       │
       ├─ ↑↓ arrows → move highlight
       ├─ Tab → complete highlighted item into input
       ├─ Enter → execute (no-arg commands) or complete + wait for args
       └─ Esc → close panel, return to normal input
```

### Panel Layout

```
  ┌──────────────────────────────┐
  │  /skills     List all skills │  ← highlighted
  │  /skill      Activate skill  │
  └──────────────────────────────┘
  ╭──────────────────────────────╮
  │ /sk▊                         │
  ╰──────────────────────────────╯
```

### Implementation

```go
type CompletionState struct {
    Active   bool
    Items    []CompletionItem
    Filtered []CompletionItem
    Selected int
    Prefix   string            // typed chars after "/"
}

type CompletionItem struct {
    Name        string         // "skills"
    Aliases     string         // "ss"
    Description string         // "List all skills"
    HasArgs     bool
}
```

- New file: `tui/completion.go`
- `CommandRegistry` exposes `ListCommands() []CommandInfo` for completion queries
- When `CompletionState.Active == true`, ↑↓/Tab/Enter/Esc are intercepted by the completion panel, not passed to textarea
- Matching: prefix match on both command names and aliases

---

## 9. Runtime Coordinator

### Structure

```go
type Runtime struct {
    Config          *config.Config
    Agent           *agent.Agent
    ToolRegistry    *tools.Registry
    SkillIndex      *skills.Index
    RulesLoader     *rules.Loader
    PromptBuilder   *prompt.Builder
    CommandRegistry *commands.Registry

    activeSkills    []string          // Explicitly activated skills for next request
    lastTrace       *prompt.Trace     // Most recent prompt trace
}
```

### Responsibilities

1. Hold references to all subsystems
2. Coordinate a full request: load rules → match skills → build prompt → call agent
3. Expose `RunUserInput(ctx, input) <-chan agent.AgentEvent` for TUI
4. Expose `ExecuteCommand(input) (CommandResult, error)` for TUI
5. Manage provider/model switching

### Request Flow

```
User input
  │
  ├─ Starts with "/"? ──→ Runtime.ExecuteCommand() ──→ Return result to TUI
  │
  └─ Normal message ──→ Runtime.RunUserInput()
                          │
                          ├─ 1. RulesLoader.Load()
                          ├─ 2. SkillIndex.Match(input) + activeSkills
                          ├─ 3. PromptBuilder.Build(rules, skills, input)
                          ├─ 4. Agent.Run(ctx, builtPrompt)
                          └─ 5. Return event channel to TUI
```

### TUI Changes

- `AppConfig` callbacks replaced by `*Runtime` reference
- `handleSlashCommand` becomes `runtime.ExecuteCommand(input)` — no more if-else
- All other TUI logic unchanged

---

## 10. New Package Structure

```
go-agent/
├── runtime/
│   └── runtime.go          # Runtime struct + request coordination
├── skills/
│   ├── skill.go            # Skill data structures + parsing
│   ├── loader.go           # Filesystem loading (single file + directory)
│   ├── index.go            # SkillIndex + matching logic
│   └── frontmatter.go      # YAML frontmatter parsing
├── commands/
│   ├── registry.go         # CommandRegistry
│   ├── command.go          # Command interface
│   └── builtin/            # Built-in command implementations
│       ├── help.go
│       ├── clear.go
│       ├── provider.go
│       ├── model.go
│       ├── skills_cmd.go
│       ├── skill_cmd.go
│       ├── reload.go
│       ├── prompt_cmd.go
│       ├── tools_cmd.go
│       └── quit.go
├── prompt/
│   ├── builder.go          # PromptBuilder + source tracking
│   └── trace.go            # PromptTrace
├── rules/
│   └── loader.go           # Rules file loading
├── tui/
│   └── completion.go       # Command autocomplete panel (NEW)
```

Existing packages `agent/`, `client/`, `config/`, `tools/`, `tui/` remain intact with minimal necessary modifications.

---

## 11. Agent Modifications

`agent.Agent` needs one change: `buildMessages()` currently hardcodes system prompt prepending. After refactoring:

- Add `Agent.SetSystemPrompt(prompt string)` — Runtime calls this before each `Run()` with the PromptBuilder output
- `buildMessages()` continues to prepend `config.SystemPrompt` as before — no structural change
- History still maintained by Agent; the per-turn skill/rules injection lives in SystemPrompt, not stored in history
- This keeps Agent backward-compatible: without Runtime, the hardcoded system prompt still works

---

## 12. Phase 1 Scope

What gets implemented now:

- [x] `skills/` package: Skill struct, loader, index, frontmatter parser
- [x] `rules/` package: Rules file loader
- [x] `prompt/` package: PromptBuilder with source tracking, Trace
- [x] `commands/` package: CommandRegistry + all 10 built-in commands
- [x] `runtime/` package: Runtime coordinator
- [x] `tui/completion.go`: Slash command autocomplete
- [x] Refactor `main.go` to use Runtime
- [x] Refactor `tui/app.go` to use Runtime + CommandRegistry
- [x] Skill invocation paths: prompt-only, tool-guiding
- [x] Tests for all new packages

What is deferred:

- [ ] Scripted invocation path (Yaegi integration)
- [ ] `embed.FS` built-in skills
- [ ] Skill `inputs`/`outputs` validation
- [ ] Skill templates rendering
- [ ] Skill versioning / dependency resolution
