# Skill System Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add category layer to skill directory structure, add AgentCallbacks progress system, fix two bugs (Registry duplicates + CompletionState stale), and update frontmatter format.

**Architecture:** Bottom-up — start with the Skill data model, then update scanning/loading to support `skills/<category>/<skill-name>/SKILL.md`, fix the command registry bug, add optional AgentCallbacks to the Agent execution loop, and finally wire TUI refresh.

**Tech Stack:** Go 1.21+, no new dependencies.

---

### Task 1: Add Category field to Skill struct

**Files:**
- Modify: `skills/skill.go`

- [ ] **Step 1: Add Category field to Skill struct**

```go
type Skill struct {
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Version     string        `yaml:"version"`
	Tags        []string      `yaml:"tags"`
	Triggers    TriggerConfig `yaml:"triggers"`
	Scope       string        `yaml:"scope"`
	Priority    int           `yaml:"priority"`
	Tools       []string      `yaml:"tools"`
	Scripts     []ScriptRef   `yaml:"scripts"`
	Inputs      []InputDef    `yaml:"inputs"`
	Outputs     []OutputDef   `yaml:"outputs"`
	Body        string        `yaml:"-"`
	Source      string        `yaml:"-"`
	BasePath    string        `yaml:"-"`
	Category    string        `yaml:"-"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd go-agent && go build ./skills/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add go-agent/skills/skill.go
git commit -m "feat: add Category field to Skill struct"
```

---

### Task 2: Add Category to SkillDraft and update RenderMarkdown

**Files:**
- Modify: `skills/draft.go`
- Modify: `skills/draft_test.go`

- [ ] **Step 1: Add Category field to SkillDraft**

```go
type SkillDraft struct {
	Name        string
	Description string
	Tags        []string
	Keywords    []string
	Patterns    []string
	Priority    int
	Body        string
	Category    string
}
```

- [ ] **Step 2: Update RenderMarkdown to output metadata.go-agent block**

In `RenderMarkdown()`, after the priority block and before `---\n`, add:

```go
// After the priority block (around line 60):
if d.Category != "" {
	b.WriteString("metadata:\n")
	b.WriteString("  go-agent:\n")
	b.WriteString(fmt.Sprintf("    category: %s\n", d.Category))
}
```

Insert this right before the line:
```go
b.WriteString("---\n")
b.WriteString("\n")
b.WriteString(d.Body)
```

So the full ending becomes:

```go
	if d.Category != "" {
		b.WriteString("metadata:\n")
		b.WriteString("  go-agent:\n")
		b.WriteString(fmt.Sprintf("    category: %s\n", d.Category))
	}

	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString(d.Body)
	b.WriteString("\n")

	return b.String()
```

- [ ] **Step 3: Add test for metadata.go-agent in rendered output**

Add a new test case in `TestSkillDraft_RenderMarkdown` in `skills/draft_test.go`:

```go
t.Run("draft with category renders metadata", func(t *testing.T) {
	draft := SkillDraft{
		Name:        "cat-skill",
		Description: "With category",
		Category:    "core",
		Body:        "Body.",
	}

	got := draft.RenderMarkdown()

	if !strings.Contains(got, "metadata:\n") {
		t.Error("should contain metadata block")
	}
	if !strings.Contains(got, "  go-agent:\n") {
		t.Error("should contain go-agent key")
	}
	if !strings.Contains(got, "    category: core\n") {
		t.Error("should contain category: core")
	}
})

t.Run("draft without category omits metadata", func(t *testing.T) {
	draft := SkillDraft{
		Name:        "no-cat",
		Description: "No category",
		Body:        "Body.",
	}

	got := draft.RenderMarkdown()

	if strings.Contains(got, "metadata:") {
		t.Error("should not contain metadata when category is empty")
	}
})
```

- [ ] **Step 4: Run tests**

Run: `cd go-agent && go test ./skills/ -run TestSkillDraft -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add go-agent/skills/draft.go go-agent/skills/draft_test.go
git commit -m "feat: add Category to SkillDraft, render metadata.go-agent in frontmatter"
```

---

### Task 3: Parse metadata.go-agent.category from frontmatter

**Files:**
- Modify: `skills/frontmatter.go`

**Context:** `ParseSkillFile` uses `yaml.Unmarshal` into the `Skill` struct. Since `Category` is tagged `yaml:"-"`, it won't parse from the flat YAML. We need to parse the `metadata.go-agent.category` nested path separately.

- [ ] **Step 1: Add a metadata struct and extract category after parsing**

Add a helper to parse the nested metadata:

```go
// goAgentMetadata mirrors the metadata.go-agent block in frontmatter.
type goAgentMetadata struct {
	Category string `yaml:"category"`
}

type skillFrontmatter struct {
	Skill    `yaml:",inline"`
	Metadata struct {
		GoAgent goAgentMetadata `yaml:"go-agent"`
	} `yaml:"metadata"`
}
```

- [ ] **Step 2: Update ParseSkillFile to parse metadata**

Replace the current parsing logic in `ParseSkillFile`:

```go
func ParseSkillFile(content string) (*Skill, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return nil, err
	}

	skill := &fm.Skill
	if skill.Name == "" {
		return nil, errors.New("skill frontmatter missing required field: name")
	}

	skill.Category = fm.Metadata.GoAgent.Category
	skill.Body = strings.TrimSpace(body)
	return skill, nil
}
```

- [ ] **Step 3: Verify roundtrip test still passes**

Run: `cd go-agent && go test ./skills/ -run TestSkillDraft_RenderMarkdown_Roundtrip -v`
Expected: PASS (roundtrip parses category even if empty)

- [ ] **Step 4: Run all skills tests**

Run: `cd go-agent && go test ./skills/ -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add go-agent/skills/frontmatter.go
git commit -m "feat: parse metadata.go-agent.category from skill frontmatter"
```

---

### Task 4: Category-aware directory scanning in LoadDir

**Files:**
- Modify: `skills/loader.go`
- Modify: `skills/loader_test.go`

- [ ] **Step 1: Rewrite LoadDir to scan category subdirectories**

Replace the entire `LoadDir` function:

```go
func LoadDir(dir string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []*Skill

	for _, entry := range entries {
		if !entry.IsDir() {
			// Legacy: flat .md file at root → category "core"
			if strings.HasSuffix(entry.Name(), ".md") {
				skill, loadErr := loadFileSkill(filepath.Join(dir, entry.Name()))
				if loadErr == nil {
					if skill.Category == "" {
						skill.Category = "core"
					}
					skills = append(skills, skill)
				}
			}
			continue
		}

		// entry is a directory: it could be a category/, or a skill/ with SKILL.md
		skillMD := filepath.Join(dir, entry.Name(), "SKILL.md")
		if _, statErr := os.Stat(skillMD); statErr == nil {
			// Direct skill dir (no category layer) → category "core"
			skill, loadErr := loadDirSkill(filepath.Join(dir, entry.Name()))
			if loadErr == nil {
				if skill.Category == "" {
					skill.Category = "core"
				}
				skills = append(skills, skill)
			}
			continue
		}

		// Try as a category directory: scan subdirectories for SKILL.md
		categoryName := entry.Name()
		subEntries, subErr := os.ReadDir(filepath.Join(dir, categoryName))
		if subErr != nil {
			continue
		}
		for _, sub := range subEntries {
			if !sub.IsDir() {
				continue
			}
			subSkillMD := filepath.Join(dir, categoryName, sub.Name(), "SKILL.md")
			if _, statErr := os.Stat(subSkillMD); statErr == nil {
				skill, loadErr := loadDirSkill(filepath.Join(dir, categoryName, sub.Name()))
				if loadErr == nil {
					if skill.Category == "" {
						skill.Category = categoryName
					}
					skills = append(skills, skill)
				}
			}
		}
	}

	return skills, nil
}
```

- [ ] **Step 2: Update existing tests for new category behavior**

In `TestLoadSingleFileSkill`, add a check for category:

```go
if s.Category != "core" {
	t.Errorf("expected category 'core', got %q", s.Category)
}
```

In `TestLoadDirectorySkill`, add the same check:

```go
if s.Category != "core" {
	t.Errorf("expected category 'core', got %q", s.Category)
}
```

- [ ] **Step 3: Add test for category-aware scanning**

Add to `skills/loader_test.go`:

```go
func TestLoadCategoryDir(t *testing.T) {
	dir := t.TempDir()
	// Create skills/core/my-skill/SKILL.md
	coreDir := filepath.Join(dir, "core", "my-skill")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: cat-skill
description: A categorized skill
metadata:
  go-agent:
    category: core
---
Categorized body.
`
	if err := os.WriteFile(filepath.Join(coreDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name != "cat-skill" {
		t.Errorf("expected name 'cat-skill', got %q", s.Name)
	}
	if s.Category != "core" {
		t.Errorf("expected category 'core', got %q", s.Category)
	}
}

func TestLoadCategoryFallback(t *testing.T) {
	dir := t.TempDir()
	// Create skills/code/my-skill/SKILL.md without metadata.go-agent.category
	// Category should fall back to directory name "code"
	coreDir := filepath.Join(dir, "code", "my-skill")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: fallback-skill
description: No metadata category
---
Fallback body.
`
	if err := os.WriteFile(filepath.Join(coreDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Category != "code" {
		t.Errorf("expected category 'code' from dirname, got %q", s.Category)
	}
}

func TestLoadMixedLegacyAndCategory(t *testing.T) {
	dir := t.TempDir()

	// Legacy flat .md file
	legacyContent := `---
name: legacy-skill
description: Old flat skill
---
Legacy body.
`
	writeTempSkill(t, dir, "legacy-skill.md", legacyContent)

	// New category structure
	coreDir := filepath.Join(dir, "core", "categorized")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	catContent := `---
name: categorized-skill
description: New style
---
Cat body.
`
	if err := os.WriteFile(filepath.Join(coreDir, "SKILL.md"), []byte(catContent), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	// Collect by name
	names := make(map[string]*Skill)
	for _, s := range skills {
		names[s.Name] = s
	}

	if s, ok := names["legacy-skill"]; ok {
		if s.Category != "core" {
			t.Errorf("legacy skill should have category 'core', got %q", s.Category)
		}
	} else {
		t.Error("legacy-skill not found")
	}

	if s, ok := names["categorized-skill"]; ok {
		if s.Category != "core" {
			t.Errorf("categorized skill should have category 'core', got %q", s.Category)
		}
	} else {
		t.Error("categorized-skill not found")
	}
}
```

- [ ] **Step 4: Run loader tests**

Run: `cd go-agent && go test ./skills/ -run TestLoad -v`
Expected: all tests PASS (3 new + 3 existing)

- [ ] **Step 5: Commit**

```bash
git add go-agent/skills/loader.go go-agent/skills/loader_test.go
git commit -m "feat: category-aware directory scanning in LoadDir"
```

---

### Task 5: Update Manager for category-based paths

**Files:**
- Modify: `skills/manager.go`
- Modify: `skills/manager_test.go`

- [ ] **Step 1: Update createIn to use category in path**

Change `createIn` in `skills/manager.go`. The path changes from `dir/<slug>/SKILL.md` to `dir/<category>/<slug>/SKILL.md`:

```go
func (m *Manager) createIn(draft SkillDraft, dir string) (*Skill, error) {
	if err := draft.Validate(); err != nil {
		return nil, err
	}

	if draft.Category == "" {
		draft.Category = "core"
	}

	slug := Slugify(draft.Name)

	existingNames := m.existingSlugs()
	slug = UniqueSlug(draft.Name, existingNames)

	// Create directory with category layer: dir/category/slug/
	skillDir := filepath.Join(dir, draft.Category, slug)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}

	content := draft.RenderMarkdown()
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write skill file: %w", err)
	}

	if err := m.Reload(); err != nil {
		return nil, fmt.Errorf("reload after create: %w", err)
	}

	matches := m.index.MatchByName(draft.Name)
	if len(matches) == 0 {
		return nil, fmt.Errorf("skill %q was written but not found after reload", draft.Name)
	}
	return matches[0], nil
}
```

- [ ] **Step 2: Update existingSlugs to scan category dirs too**

```go
func (m *Manager) existingSlugs() []string {
	var names []string
	for _, dir := range []string{m.userDir, m.projectDir} {
		// Also scan one level of category subdirectories
		categories, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, cat := range categories {
			if !cat.IsDir() {
				continue
			}
			catPath := filepath.Join(dir, cat.Name())
			entries, err := os.ReadDir(catPath)
			if err != nil {
				continue
			}
			for _, e := range entries {
				name := e.Name()
				if !e.IsDir() {
					name = strings.TrimSuffix(name, filepath.Ext(name))
				}
				names = append(names, name)
			}
		}
	}
	return names
}
```

- [ ] **Step 3: Update test path assertions in manager_test.go**

In `TestManager_Create`, change:
```go
skillFile := filepath.Join(userDir, "my-skill", "SKILL.md")
```
to:
```go
skillFile := filepath.Join(userDir, "core", "my-skill", "SKILL.md")
```

In `TestManager_Create_DuplicateSlug`, change:
```go
dir2 := filepath.Join(userDir, "dup-skill-2")
```
to:
```go
dir2 := filepath.Join(userDir, "core", "dup-skill-2")
```

In `TestManager_Delete`, change:
```go
dir := filepath.Join(userDir, "delete-me")
```
to:
```go
dir := filepath.Join(userDir, "core", "delete-me")
```

In `TestManager_Update`, change:
```go
data, err := os.ReadFile(filepath.Join(userDir, "updatable", "SKILL.md"))
```
to:
```go
data, err := os.ReadFile(filepath.Join(userDir, "core", "updatable", "SKILL.md"))
```

In `TestManager_CreateInProject`, change:
```go
skillFile := filepath.Join(projectDir, "project-skill", "SKILL.md")
```
to:
```go
skillFile := filepath.Join(projectDir, "core", "project-skill", "SKILL.md")
```

In `TestManager_Reload`, change the manual skill write path from:
```go
dir := filepath.Join(userDir, slug)
```
to:
```go
dir := filepath.Join(userDir, "core", slug)
```

- [ ] **Step 4: Run manager tests**

Run: `cd go-agent && go test ./skills/ -run TestManager -v`
Expected: all tests PASS

- [ ] **Step 5: Run all skills tests**

Run: `cd go-agent && go test ./skills/ -v`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add go-agent/skills/manager.go go-agent/skills/manager_test.go
git commit -m "feat: category-based paths in skill Manager"
```

---

### Task 6: Fix Registry.Register upsert

**Files:**
- Modify: `commands/registry.go`
- Modify: `commands/registry_test.go`

- [ ] **Step 1: Change Register to upsert**

Replace the `Register` function body:

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

- [ ] **Step 2: Add test for re-registration (no duplicate in order)**

Add to `commands/registry_test.go`:

```go
func TestRegistryReRegisterNoDuplicate(t *testing.T) {
	r := NewRegistry()
	cmd1 := newTestCmd("reloadable", nil, "/reloadable")
	r.Register(cmd1)

	// Re-register with same name (simulating reload)
	cmd2 := newTestCmd("reloadable", nil, "/reloadable")
	r.Register(cmd2)

	list := r.List()
	// Count occurrences of "reloadable"
	count := 0
	for _, info := range list {
		if info.Name == "reloadable" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of 'reloadable' in list, got %d", count)
	}

	// The Get should return the second registration
	got, ok := r.Get("reloadable")
	if !ok {
		t.Fatal("expected to find command")
	}
	_ = got
}
```

- [ ] **Step 3: Run registry tests**

Run: `cd go-agent && go test ./commands/ -run TestRegistry -v`
Expected: all tests PASS including new re-registration test

- [ ] **Step 4: Commit**

```bash
git add go-agent/commands/registry.go go-agent/commands/registry_test.go
git commit -m "fix: Registry.Register uses upsert to prevent duplicate entries"
```

---

### Task 7: Add Category to CreateSkillRequest and create-skill command

**Files:**
- Modify: `commands/command.go`
- Modify: `commands/builtin/create_skill.go`

- [ ] **Step 1: Add Category field to CreateSkillRequest**

```go
type CreateSkillRequest struct {
	Name         string
	Description  string
	Tags         []string
	Keywords     []string
	Patterns     []string
	Priority     int
	Body         string
	ProjectLevel bool
	Category     string
}
```

- [ ] **Step 2: Update create_skill.go to parse --category flag**

In `Execute` method, after parsing priority:

```go
if cat := flags["category"]; cat != "" {
	req.Category = cat
}
```

- [ ] **Step 3: Update Runtime.CreateSkill to pass Category to draft**

In `runtime/runtime.go`, the `CreateSkill` method's draft construction already uses `req.Category` implicitly via the struct. Verify the draft line includes Category:

The draft construction in `Runtime.CreateSkill`:
```go
draft := skills.SkillDraft{
	Name:        req.Name,
	Description: req.Description,
	Tags:        req.Tags,
	Keywords:    req.Keywords,
	Patterns:    req.Patterns,
	Priority:    req.Priority,
	Body:        req.Body,
}
```

Add:
```go
	Category:    req.Category,
```

- [ ] **Step 4: Run tests**

Run: `cd go-agent && go test ./commands/... -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add go-agent/commands/command.go go-agent/commands/builtin/create_skill.go go-agent/runtime/runtime.go
git commit -m "feat: add Category to CreateSkillRequest and create-skill command"
```

---

### Task 8: Create AgentCallbacks interface

**Files:**
- Create: `agent/callbacks.go`

- [ ] **Step 1: Create agent/callbacks.go**

```go
package agent

import "time"

// ToolProgress tracks a tool call's execution state.
type ToolProgress struct {
	Name    string
	Args    map[string]any
	Status  string // "pending"|"running"|"done"|"error"
	Elapsed time.Duration
}

// AgentCallbacks is an optional interface for observing agent execution progress.
// It is invoked at key points in the ReAct loop: thinking, tool calls, streaming, heartbeat.
// A nil implementation means no callbacks are invoked.
type AgentCallbacks interface {
	OnThinkingStart()
	OnThinkingEnd()
	OnToolStart(tool *ToolProgress)
	OnToolEnd(tool *ToolProgress, result string)
	OnStreamDelta(delta string)
	OnHeartbeat(elapsed time.Duration)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd go-agent && go build ./agent/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add go-agent/agent/callbacks.go
git commit -m "feat: add AgentCallbacks interface for progress observation"
```

---

### Task 9: Integrate AgentCallbacks into Agent

**Files:**
- Modify: `agent/agent.go`

- [ ] **Step 1: Add callbacks field to Agent struct**

```go
type Agent struct {
	llm       client.LLMClient
	registry  *tools.Registry
	history   []client.Message
	mu        sync.Mutex
	config    AgentConfig
	callbacks AgentCallbacks
}
```

- [ ] **Step 2: Add SetCallbacks method**

```go
// SetCallbacks sets the optional callbacks for progress observation.
func (a *Agent) SetCallbacks(cb AgentCallbacks) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.callbacks = cb
}
```

- [ ] **Step 3: Invoke callbacks in runLoop**

In `runLoop`, at the start of each iteration before LLM call:

```go
a.mu.Lock()
cb := a.callbacks
a.mu.Unlock()

if cb != nil {
	cb.OnThinkingStart()
}
```

After the LLM stream completes (after the `for chunk := range streamCh` loop), add:

```go
if cb != nil {
	cb.OnThinkingEnd()
}
```

Before each tool execution (in `executeTools` call, or just before the tool result loop), add `OnToolStart` for each tool. The best place is in `executeTools` — but that method doesn't have access to callbacks. Instead, emit callbacks in `runLoop` right before `executeTools`:

```go
// Before executeTools call (around line 194)
if cb != nil {
	for _, tc := range toolCalls {
		var params map[string]any
		_ = json.Unmarshal([]byte(tc.Arguments), &params)
		cb.OnToolStart(&ToolProgress{
			Name:   tc.Name,
			Args:   params,
			Status: "running",
		})
	}
}
```

And after `executeTools` completes (around line 207), emit `OnToolEnd`:

```go
// After executeTools results (around line 207)
if cb != nil {
	for i, tc := range toolCalls {
		cb.OnToolEnd(&ToolProgress{
			Name:   tc.Name,
			Status: "done",
		}, results[i])
	}
}
```

For streaming delta, inside the stream loop where `EventDelta` is emitted (around line 139):

```go
if chunk.Delta != "" {
	fullContent += chunk.Delta
	if cb != nil {
		cb.OnStreamDelta(chunk.Delta)
	}
	select {
	case ch <- AgentEvent{Type: EventDelta, Content: chunk.Delta}:
	case <-ctx.Done():
		return
	}
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd go-agent && go build ./agent/`
Expected: PASS

- [ ] **Step 5: Run agent tests if any**

Run: `cd go-agent && go test ./agent/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-agent/agent/agent.go
git commit -m "feat: integrate AgentCallbacks into Agent execution loop"
```

---

### Task 10: Update Runtime — project path, progress messages, wire callbacks

**Files:**
- Modify: `runtime/runtime.go`

- [ ] **Step 1: Change project skills path from ./.go-agent/skills to ./skills/**

In `Runtime.New`, change:
```go
projectSkillsDir := filepath.Join(projectDir, "skills")
```
from:
```go
projectSkillsDir := filepath.Join(projectDir, "skills")
```
(was `filepath.Join(projectDir, "skills")` — just update the comment if `projectDir` already refers to `.go-agent`)

Actually, look at line 83: `projectDir := ".go-agent"`. We need to change the skills subdirectory. Currently at line 87:
```go
projectSkillsDir := filepath.Join(projectDir, "skills")
```

This is already `./.go-agent/skills/`. The spec says it should be `./skills/`. So change line 83 from:
```go
projectDir := ".go-agent"
```
Change the skills path, not the projectDir (which is still used by rules):

Replace line 87:
```go
projectSkillsDir := filepath.Join(projectDir, "skills")
```
with:
```go
projectSkillsDir := "skills"
```

- [ ] **Step 2: Update ReloadSkillsAndRules path**

In `ReloadSkillsAndRules`, change line 274 from:
```go
projectSkillsDir := filepath.Join(rt.projectDir, "skills")
```
to:
```go
projectSkillsDir := "skills"
```

- [ ] **Step 3: Add progress messages to CreateSkill**

Update the `CreateSkill` method to include category in the message:

```go
func (rt *Runtime) CreateSkill(req commands.CreateSkillRequest) (commands.CreateSkillResult, error) {
	draft := skills.SkillDraft{
		Name:        req.Name,
		Description: req.Description,
		Tags:        req.Tags,
		Keywords:    req.Keywords,
		Patterns:    req.Patterns,
		Priority:    req.Priority,
		Body:        req.Body,
		Category:    req.Category,
	}

	var skill *skills.Skill
	var err error
	if req.ProjectLevel {
		skill, err = rt.skillManager.CreateInProject(draft)
	} else {
		skill, err = rt.skillManager.Create(draft)
	}
	if err != nil {
		return commands.CreateSkillResult{}, err
	}

	rt.cmdRegistry.Register(builtin.NewSkillRun(skill.Name, skill.Description))

	msg := fmt.Sprintf(
		"[1/3] 解析完成: name=%s, category=%s\n[2/3] 已写入: %s\n[3/3] 已注册: /%s 可用",
		skill.Name, skill.Category, skill.BasePath, skill.Name,
	)

	return commands.CreateSkillResult{
		Name:    skill.Name,
		Path:    skill.BasePath,
		Message: msg,
	}, nil
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd go-agent && go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-agent/runtime/runtime.go
git commit -m "feat: migrate project skills to ./skills/, add progress messages"
```

---

### Task 11: TUI — CompletionState refresh and callback wiring

**Files:**
- Modify: `tui/app.go`
- Modify: `tui/completion.go`

- [ ] **Step 1: Add RefreshCompletion to TUI model**

Add a method to `Model`:

```go
func (m *Model) RefreshCompletion() {
	if m.config.Runtime != nil {
		m.completion = NewCompletionState(m.config.Runtime.CmdRegistry().List())
	}
}
```

- [ ] **Step 2: Add RefreshCompletion to RuntimeInterface**

In `tui/app.go`, add to `RuntimeInterface`:

```go
type RuntimeInterface interface {
	RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent
	ExecuteCommand(input string) (commands.CommandResult, error)
	CurrentProvider() string
	CurrentModel() string
	CmdRegistry() *commands.Registry
}
```

No change needed — `RuntimeInterface` already exposes `CmdRegistry()`. The `RefreshCompletion` method on Model handles the rest via the existing interface.

- [ ] **Step 3: Call RefreshCompletion after skill CRUD operations**

In `handleSlashCommand`, after a command executes successfully, refresh completion:

Add after the command result handling (around line 409, after `result.Message != ""` block):

```go
// Refresh completion in case commands were added/removed
m.RefreshCompletion()
```

- [ ] **Step 4: Verify compilation**

Run: `cd go-agent && go build ./...`
Expected: PASS

- [ ] **Step 5: Run all tests**

Run: `cd go-agent && go test ./... -v`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add go-agent/tui/app.go
git commit -m "feat: refresh TUI completion state after skill CRUD operations"
```

---

### Task 12: Migrate existing skill files to new category structure

**Files:**
- Move: `skills/novel-writing.md` → `skills/core/novel-writing/SKILL.md`
- Move: `skills/scifi-writing.md` → `skills/core/scifi-writing/SKILL.md`

- [ ] **Step 1: Create new directory structure**

```bash
mkdir -p go-agent/skills/core/novel-writing
mkdir -p go-agent/skills/core/scifi-writing
```

- [ ] **Step 2: Move files and add metadata.go-agent.category**

Move `novel-writing.md`:
```bash
mv go-agent/skills/novel-writing.md go-agent/skills/core/novel-writing/SKILL.md
```

Move `scifi-writing.md`:
```bash
mv go-agent/skills/scifi-writing.md go-agent/skills/core/scifi-writing/SKILL.md
```

- [ ] **Step 3: Add metadata.go-agent.category to each skill's frontmatter**

For `skills/core/novel-writing/SKILL.md`, add after the `tools` block and before `inputs`:

```yaml
metadata:
  go-agent:
    category: core
```

For `skills/core/scifi-writing/SKILL.md`, add at the same position.

- [ ] **Step 4: Verify skills load correctly**

Run: `cd go-agent && go test ./skills/ -v`
Expected: PASS (LoadDir should find them during tests)

- [ ] **Step 5: Commit**

```bash
git add go-agent/skills/core/
git rm go-agent/skills/novel-writing.md go-agent/skills/scifi-writing.md
git commit -m "refactor: migrate existing skills to skills/core/ category structure"
```

---

### Task 13: Final integration test

- [ ] **Step 1: Build the binary**

Run: `cd go-agent && go build -o go-agent.exe .`
Expected: build succeeds

- [ ] **Step 2: Run full test suite**

Run: `cd go-agent && go test ./... -v 2>&1 | tail -20`
Expected: all packages PASS

- [ ] **Step 3: Commit any remaining changes**

```bash
git status
git add -A
git commit -m "chore: final integration verification"
```
