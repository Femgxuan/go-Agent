# Skill System Improvements — Design Spec

## Problem Statement

1. When Agent creates a skill, users cannot see progress or track execution steps.
2. After creation, skills don't appear in `/skills` output or slash-command autocomplete without restart.
3. The skill system directory structure and frontmatter format don't match the target paradigm.

## Design

### 1. Directory Structure & Category System

**New layout:**

```
./skills/                        # project-level (was ./.go-agent/skills/)
  └── core/                      # default category
      └── <skill-name>/
          └── SKILL.md

~/.go-agent/skills/              # user-level (unchanged)
  └── core/
      └── <skill-name>/
          └── SKILL.md
```

**Category strategy:** Start with `core/` as the sole default category. New categories are created only when 3+ skills with clear commonality emerge.

**`LoadDir` scanning logic:**

1. Scan subdirectories first — each dir is a category; look for `SKILL.md` inside skill dirs within each category
2. Scan root `.md` files as fallback — treated as category `core` for backward compatibility
3. Category is read from `metadata.go-agent.category` in frontmatter; falls back to directory name

**`Skill` struct addition:**

```go
type Skill struct {
    // ... existing fields ...
    Category string `yaml:"-"`
}
```

**`SkillDraft` addition:**

```go
type SkillDraft struct {
    // ... existing fields ...
    Category string
}
```

Default category is always `"core"`.

### 2. Frontmatter Format

Add optional `metadata.go-agent` block. Existing fields unchanged.

```yaml
---
name: my-skill
description: 描述
version: "1.0"
tags: [tag1, tag2]
triggers:
  keywords: [...]
  patterns: [...]
priority: 10
metadata:
  go-agent:
    category: core
---
```

- `metadata.go-agent.category` is optional; defaults to directory name if absent.
- If present and different from directory name, the frontmatter value wins.
- `Draft.RenderMarkdown()` writes this block automatically.

### 3. Bug Fixes

**Bug A: Registry duplicate registration**

`Registry.Register` currently unconditionally appends to `order`. After reload, skills appear multiple times in autocomplete.

Fix: upsert semantics — update the command if it already exists, append to order only on first registration.

```go
func (r *Registry) Register(cmd Command) {
    name := cmd.Name()
    if _, exists := r.commands[name]; !exists {
        r.order = append(r.order, name)
    }
    r.commands[name] = cmd
    for _, alias := range cmd.Aliases() {
        r.aliases[alias] = name
    }
}
```

**Bug B: CompletionState never refreshed**

`CompletionState` is built once in `NewModel` and never updated after skill creation/deletion.

Fix: Add `RefreshCompletion()` to the TUI model, called after any skill CRUD operation. This rebuilds the completion items from the current command registry state.

### 4. Global Interaction Progress System

Add structured `AgentCallbacks` interface so all Agent interactions (thinking, tool calls, streaming) have visible progress.

**New file `agent/callbacks.go`:**

```go
type ToolProgress struct {
    Name    string
    Args    map[string]any
    Status  string  // "pending"|"running"|"done"|"error"
    Elapsed time.Duration
}

type AgentCallbacks interface {
    OnThinkingStart()
    OnThinkingEnd()
    OnToolStart(tool *ToolProgress)
    OnToolEnd(tool *ToolProgress, result string)
    OnStreamDelta(delta string)
    OnHeartbeat(elapsed time.Duration)
}
```

**Agent integration:** Callbacks are optional. When set, the Agent invokes them at the appropriate points in its execution loop. When nil, behavior is unchanged.

**TUI implementation:** The existing bubbletea model already has spinner and tool call panels. The CLIDisplay adapter maps callback events to the existing panel/state system.

**Skill creation progress:** `Runtime.CreateSkill` returns a multi-line message showing the 3 stages:

```
[1/3] 解析完成: name=my-skill, category=core
[2/3] 已写入: ~/.go-agent/skills/core/my-skill/SKILL.md
[3/3] 已注册: /my-skill 可用
```

### 5. Runtime Path Changes

Project-level skills directory changes from `./.go-agent/skills/` to `./skills/`.

`Runtime.New`:
```go
projectSkillsDir := filepath.Join(projectDir, "skills")  // was .go-agent/skills
```

### 6. Files Changed

| File | Change |
|------|--------|
| `agent/agent.go` | Inject optional `AgentCallbacks`, invoke in execution loop |
| `agent/callbacks.go` | **New** — `ToolProgress` struct, `AgentCallbacks` interface |
| `skills/skill.go` | Add `Category` field |
| `skills/loader.go` | Category-aware directory scanning |
| `skills/draft.go` | Add `Category` to draft, render `metadata.go-agent` in frontmatter |
| `skills/manager.go` | Default category `"core"`, paths use category layer |
| `commands/command.go` | `CreateSkillRequest` add `Category` |
| `commands/registry.go` | `Register` upsert semantics |
| `commands/builtin/create_skill.go` | Parse `--category` flag |
| `runtime/runtime.go` | Project path `./skills/`, progress messages |
| `tui/app.go` | Inject callbacks, `CompletionState` refresh |
