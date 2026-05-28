# Memory Markdown Classifier Design

**Date**: 2026-05-29
**Status**: Draft
**Replaces**: Parts of `2026-05-28-hermes-memory-system-design.md` (Classifier and markdown write sections)

## Problem

The current `Classifier` (`memory/classifier.go`) decides whether to store a fact in PostgreSQL with structured categories (preference/environment/correction/norm/milestone/explicit). But per the Memory Management Protocol, PG auto-archives all conversations — the Classifier's real job should be deciding **which markdown file** to write to (SOUL.md / MEMORY.md / USER.md) for curated hot memory.

## Architecture

### Memory Tiers

| Tier | Storage | Temperature | Loading | Purpose |
|------|---------|-------------|---------|---------|
| Hot | SOUL.md, MEMORY.md, USER.md | Hot | Every session, injected into system prompt | Curated knowledge, slow-changing |
| Cold | PostgreSQL (facts + episodes) | Cold | On-demand via `session_search` / `Retrieve()` | Full conversation history, semantic search |

### Classification Decision

```
User input → extractMemoryFact() [rule pre-filter]
  │
  ├─ No trigger keywords → skip
  │
  └─ Trigger detected → Classify() [LLM]
       │
       ├─ target=soul → write ~/.go-agent/SOUL.md
       ├─ target=memory → write ~/.go-agent/MEMORY.md
       ├─ target=user → write ~/.go-agent/USER.md
       └─ target=none → skip

PG auto-archive path (unchanged):
  storeInteraction() → Store() → PG (every interaction, no classification)
```

## Changes

### 1. Classifier Output Format Change

**Current** (`classifier.go`):
```go
type classificationResponse struct {
    ShouldStore bool   `json:"should_store"`
    Category    string `json:"category"`    // preference/environment/correction/norm/milestone/explicit
    Key         string `json:"key"`
    Content     string `json:"content"`
    Reason      string `json:"reason"`
}
```

**New**:
```go
type ClassificationResult struct {
    Target  string `json:"target"`   // "soul" | "memory" | "user" | "none"
    Content string `json:"content"`  // curated fact with § prefix
    Reason  string `json:"reason"`   // why this target
}
```

The `Classify()` method returns `(ClassificationResult, bool)` where the bool indicates whether to write to a markdown file.

### 2. Classifier Prompt

Based on the Memory Management Protocol:

```
You are a memory classifier. Given a user message and an assistant reply,
determine if the message contains information worth saving to a markdown file.

Categories:
- soul: Agent personality, communication style, behavior boundaries, values
  ("be concise", "don't use emojis", "ask before destructive ops")
- memory: Project context, tech stack, environment, work conventions
  ("we use PostgreSQL", "code style is Google", "repo at ~/code/proj")
- user: User personal facts, preferences, habits, constraints
  ("my name is Alice", "I prefer Go", "I have a toddler")
- none: Trivial info, temporary tasks, easily re-rediscoverable facts

Rules:
- DO NOT store: greetings, questions, tool requests, temporary info
- DO store: stable facts that remain true across sessions
- Curate: distill to 1-2 sentences, add § prefix

Respond with ONLY JSON:
{"target": "soul"|"memory"|"user"|"none", "content": "§ curated fact", "reason": "why"}
```

### 3. extractMemoryFact() Simplification

Current function returns a `Fact` with PG categories. Simplify to return a trigger signal:

```go
type MarkdownCandidate struct {
    ShouldClassify bool   // whether to invoke LLM classifier
    Hint           string // matched keyword, passed to classifier for context
}

func extractMemoryFact(input string) MarkdownCandidate
```

Rule pre-filter only checks for trigger keywords ("记住", "我喜欢", "服务器是", etc.). It no longer assigns categories — that's the LLM's job.

### 4. Markdown Write Format

Each entry appended to the target file:

```
<!-- written 2026-05-29 -->
§ User prefers TypeScript over JavaScript, avoid recommending JS solutions
```

- `<!-- written YYYY-MM-DD -->` timestamp comment
- `§` prefix on the curated fact
- Append-only, never overwrite existing entries
- Single entry max 3 sentences

### 5. auto_write Config

```go
// memory/config.go
AutoWrite bool `yaml:"auto_write" env:"MEMORY_AUTO_WRITE"`
// Default: false (suggest mode, wait for user confirmation)
```

- `auto_write=true`: Classifier result writes directly to markdown file
- `auto_write=false`: Return suggestion to TUI for user confirmation

### 6. memory_append Tool: Add "soul" Target

Current `MemoryAppendTool` only supports `target: "memory" | "user"`. Add `"soul"`:

```go
// Schema enum: ["soul", "memory", "user"]
// getFilePath(): "soul" → SOUL.md
// getLimit(): "soul" → no limit (SOUL.md is small and stable)
```

Same for `MemoryReplaceTool` and `MemoryDeleteTool`.

### 7. HermesBuilder Wired into Runtime

**Current**: `runtime.go` uses `prompt.NewBuilder()`, doesn't read markdown files.

**New flow in `runtime.go RunUserInput()`**:

```go
// 1. Read markdown files from disk
soulContent := readFileOrDefault(filepath.Join(memDir, "SOUL.md"), "")
memoryContent := readFileOrDefault(filepath.Join(memDir, "MEMORY.md"), "")
userContent := readFileOrDefault(filepath.Join(memDir, "USER.md"), "")

// 2. Build system prompt with HermesBuilder
hb := prompt.NewHermesBuilder()
hb.SetSoul(soulContent)
hb.SetMemory(memoryContent, userContent)
hb.SetSkillsIndex(skillIndexStr)
hb.SetContextFiles(contextFiles)
hb.SetToolRules(toolRules)
hb.SetToolSchemas(toolSchemas)
systemPrompt := hb.Build() // frozen after first call

// 3. Set on agent
ag.SetSystemPrompt(systemPrompt)
```

Key constraint: `Build()` uses `sync.Once`, so the prompt is frozen for the session. Changes to markdown files take effect in the **next session**. Within the current session, relevant facts are retrieved via `Retrieve()` from PG.

### 8. Agent storeInteraction() Flow Change

```go
func (a *Agent) storeInteraction(ctx context.Context, input string) {
    // 1. Add to working memory (unchanged)
    // 2. Store to PG (unchanged, auto-archive)
    // 3. Markdown classification (NEW)
    if candidate := extractMemoryFact(input); candidate.ShouldClassify {
        go func() {
            result, ok := a.classifier.Classify(ctx, input, lastAssistant)
            if ok && result.Target != "none" {
                if a.config.AutoWrite {
                    // Direct write: uses same logic as MemoryAppendTool
                    // (security scan + capacity check + backup + append)
                    appendToMarkdown(a.memoryDir, result.Target, result.Content)
                } else {
                    // Send suggestion to TUI via callback
                    a.callbacks.OnMemorySuggestion(result.Target, result.Content, result.Reason)
                }
            }
        }()
    }
}

// appendToMarkdown writes a curated fact to the target markdown file.
// Uses the same security/backup/append logic as MemoryAppendTool.
func appendToMarkdown(memoryDir, target, content string) error {
    filePath := filepath.Join(memoryDir, map[string]string{
        "soul": "SOUL.md", "memory": "MEMORY.md", "user": "USER.md",
    }[target])
    // 1. Security scan
    if warnings := memory.ScanForInjection(content); len(warnings) > 0 {
        return fmt.Errorf("security warning: %v", warnings)
    }
    // 2. Read current, capacity check
    current, _ := os.ReadFile(filePath)
    limit := map[string]int{"memory": 2200, "user": 1375}[target] // soul: no limit
    if limit > 0 && len(current)+len(content) > limit {
        return fmt.Errorf("capacity exceeded")
    }
    // 3. Backup + append
    os.WriteFile(filePath+".bak", current, 0o644)
    f, _ := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    defer f.Close()
    if len(current) > 0 && !strings.HasSuffix(string(current), "\n") {
        f.WriteString("\n")
    }
    f.WriteString(content + "\n")
    return nil
}
```

## Files Modified

| File | Change |
|------|--------|
| `memory/classifier.go` | New prompt, new output type `ClassificationResult` |
| `agent/agent.go` | `extractMemoryFact()` simplification, `storeInteraction()` markdown write logic, `appendToMarkdown()` |
| `agent/callbacks.go` | Add `OnMemorySuggestion(target, content, reason string)` to `AgentCallbacks` interface |
| `tools/memory_tools.go` | Add "soul" target to all 3 tools |
| `runtime/runtime.go` | Wire HermesBuilder, read markdown files at startup |
| `memory/config.go` | Add `AutoWrite` field |
| `memory/types.go` | Add `MarkdownCandidate` type, `ClassificationResult` type |

## Files Unchanged

| File | Reason |
|------|--------|
| `memory/longterm.go` | PG storage unchanged, still auto-archives |
| `memory/manager.go` | `Store()` and `Retrieve()` unchanged |
| `memory/working.go` | Working memory unchanged |
| `memory/shortterm.go` | Short-term memory unchanged |
| `memory/episodic.go` | Episodic memory unchanged |

## Testing

| Test | Coverage |
|------|----------|
| `TestClassify_Targets` | Mock LLM, verify soul/memory/user/none classification |
| `TestClassify_PromptParsing` | JSON response parsing, edge cases |
| `TestExtractMemoryFact_Triggers` | Keyword detection for all 6 categories |
| `TestExtractMemoryFact_NoTrigger` | Non-trigger inputs correctly skipped |
| `TestMarkdownWrite_Format` | Timestamp, § prefix, append behavior |
| `TestMarkdownWrite_Capacity` | MEMORY.md 2200 char limit, USER.md 1375 char limit |
| `TestMarkdownWrite_Security` | Injection scan blocks dangerous content |
| `TestHermesBuilder_FileInjection` | SOUL/MEMORY/USER content appears in system prompt |
| `TestHermesBuilder_MissingFiles` | Empty files don't break prompt |
| `TestMemoryTools_SoulTarget` | memory_append/replace/delete with target="soul" |
