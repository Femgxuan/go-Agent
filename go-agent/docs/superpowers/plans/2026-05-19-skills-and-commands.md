# Skills System & Slash Commands Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an extensible Skills system, Rules system, Slash command system with TUI autocomplete, PromptBuilder with source tracking, and Runtime coordinator to the go-agent project.

**Architecture:** Five new packages (`skills/`, `rules/`, `prompt/`, `commands/`, `runtime/`) coordinate through a central Runtime struct. TUI delegates all command/input handling to Runtime. Agent receives per-turn system prompts built by PromptBuilder. Existing packages receive minimal modifications.

**Tech Stack:** Go 1.26, bubbletea/bubbles/lipgloss (TUI), gopkg.in/yaml.v3 (frontmatter parsing), standard library regexp (skill matching).

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `skills/skill.go` | `Skill` struct, `TriggerConfig`, `ScriptRef` data types |
| `skills/frontmatter.go` | Parse YAML frontmatter from markdown, return meta + body |
| `skills/loader.go` | Load skills from filesystem (single file + directory forms) |
| `skills/index.go` | `Index` struct: store loaded skills, match against user input |
| `skills/loader_test.go` | Tests for loader |
| `skills/index_test.go` | Tests for index matching |
| `rules/loader.go` | `Loader` struct: load RULES.md from user + project dirs |
| `rules/loader_test.go` | Tests for rules loader |
| `prompt/source.go` | `Source` struct (Name, Type, Content) |
| `prompt/builder.go` | `Builder`: assemble sources in order, produce final prompt string |
| `prompt/trace.go` | `Trace` struct for observability |
| `prompt/builder_test.go` | Tests for builder |
| `commands/command.go` | `Command` interface, `CommandContext`, `CommandResult`, `CommandAction` |
| `commands/registry.go` | `Registry` struct: register/lookup/list commands |
| `commands/registry_test.go` | Tests for registry |
| `commands/builtin/help.go` | `/help` command |
| `commands/builtin/clear.go` | `/clear` command |
| `commands/builtin/provider.go` | `/provider` command |
| `commands/builtin/model.go` | `/model` command |
| `commands/builtin/skills_cmd.go` | `/skills` command |
| `commands/builtin/skill_cmd.go` | `/skill` command |
| `commands/builtin/reload.go` | `/reload` command |
| `commands/builtin/prompt_cmd.go` | `/prompt` command |
| `commands/builtin/tools_cmd.go` | `/tools` command |
| `commands/builtin/quit.go` | `/quit` command |
| `commands/builtin/builtin_test.go` | Tests for built-in commands |
| `runtime/runtime.go` | `Runtime` struct: coordinate all subsystems |
| `runtime/runtime_test.go` | Tests for runtime |
| `tui/completion.go` | `CompletionState`, filtering, rendering |

### Modified files

| File | Change |
|------|--------|
| `agent/agent.go` | Add `SetSystemPrompt()` method |
| `agent/message.go` | Add `EventPromptTrace` event type |
| `tui/app.go` | Replace `AppConfig` callbacks with `*runtime.Runtime`, add completion integration |
| `tui/styles.go` | Add completion panel styles |
| `main.go` | Use `runtime.New()` instead of manual wiring |

---

### Task 1: Skills data types and frontmatter parser

**Files:**
- Create: `skills/skill.go`
- Create: `skills/frontmatter.go`

- [ ] **Step 1: Create `skills/skill.go` with data types**

```go
package skills

type TriggerConfig struct {
	Keywords []string `yaml:"keywords"`
	Patterns []string `yaml:"patterns"`
}

type ScriptRef struct {
	Name        string `yaml:"name"`
	Path        string `yaml:"path"`
	Description string `yaml:"description"`
}

type InputDef struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

type OutputDef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type Skill struct {
	// v1 required fields
	Name        string        `yaml:"name"`
	Description string        `yaml:"description"`
	Version     string        `yaml:"version"`
	Tags        []string      `yaml:"tags"`
	Triggers    TriggerConfig `yaml:"triggers"`

	// Extended fields
	Scope    string      `yaml:"scope"`    // "user" | "project" | "global"
	Priority int         `yaml:"priority"`
	Tools    []string    `yaml:"tools"`    // advisory tool names
	Scripts  []ScriptRef `yaml:"scripts"`
	Inputs   []InputDef  `yaml:"inputs"`
	Outputs  []OutputDef `yaml:"outputs"`

	// Populated by loader, not from YAML
	Body     string `yaml:"-"`
	Source   string `yaml:"-"` // "builtin" | "user" | "project"
	BasePath string `yaml:"-"`
}
```

- [ ] **Step 2: Create `skills/frontmatter.go` with parser**

```go
package skills

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseSkillFile(content string) (*Skill, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(meta), &skill); err != nil {
		return nil, fmt.Errorf("invalid skill frontmatter: %w", err)
	}
	if skill.Name == "" {
		return nil, fmt.Errorf("skill missing required field: name")
	}
	skill.Body = strings.TrimSpace(body)
	return &skill, nil
}

func splitFrontmatter(content string) (meta string, body string, err error) {
	const sep = "---"
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, sep) {
		return "", trimmed, fmt.Errorf("no frontmatter found: file must start with ---")
	}

	rest := trimmed[len(sep):]
	idx := strings.Index(rest, "\n"+sep)
	if idx < 0 {
		return "", trimmed, fmt.Errorf("no closing --- found for frontmatter")
	}

	meta = strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+len("\n"+sep):])
	return meta, body, nil
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build ./skills/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add skills/skill.go skills/frontmatter.go
git commit -m "feat(skills): add skill data types and frontmatter parser"
```

---

### Task 2: Skills loader

**Files:**
- Create: `skills/loader.go`
- Create: `skills/loader_test.go`

- [ ] **Step 1: Write the test file `skills/loader_test.go`**

```go
package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSingleFileSkill(t *testing.T) {
	dir := t.TempDir()
	content := `---
name: test-skill
description: "A test skill"
tags: [test]
triggers:
  keywords: ["test"]
---

## Purpose
This is a test skill.
`
	path := filepath.Join(dir, "test-skill.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", skills[0].Name)
	}
	if skills[0].Body == "" {
		t.Error("expected non-empty body")
	}
}

func TestLoadDirectorySkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: my-skill
description: "Directory skill"
tags: [dir]
triggers:
  keywords: ["directory"]
---

## Workflow
Step 1: do something.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "my-skill" {
		t.Errorf("expected name 'my-skill', got %q", skills[0].Name)
	}
	if skills[0].BasePath != skillDir {
		t.Errorf("expected BasePath %q, got %q", skillDir, skills[0].BasePath)
	}
}

func TestLoadDirNotExist(t *testing.T) {
	skills, err := LoadDir("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}

func TestLoadMultiLayer(t *testing.T) {
	userDir := t.TempDir()
	projDir := t.TempDir()

	// User-level skill
	userContent := `---
name: shared-skill
description: "User version"
tags: [user]
triggers:
  keywords: ["shared"]
priority: 1
---
User body.
`
	os.WriteFile(filepath.Join(userDir, "shared-skill.md"), []byte(userContent), 0644)

	// Project-level skill with same name
	projContent := `---
name: shared-skill
description: "Project version"
tags: [project]
triggers:
  keywords: ["shared"]
priority: 5
---
Project body.
`
	os.WriteFile(filepath.Join(projDir, "shared-skill.md"), []byte(projContent), 0644)

	skills, err := LoadMultiLayer(userDir, projDir)
	if err != nil {
		t.Fatalf("LoadMultiLayer failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (project overrides user), got %d", len(skills))
	}
	if skills[0].Source != "project" {
		t.Errorf("expected source 'project', got %q", skills[0].Source)
	}
	if skills[0].Description != "Project version" {
		t.Errorf("expected project description, got %q", skills[0].Description)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./skills/ -v -run TestLoad`
Expected: compilation error — `LoadDir` and `LoadMultiLayer` undefined

- [ ] **Step 3: Write `skills/loader.go`**

```go
package skills

import (
	"os"
	"path/filepath"
	"strings"
)

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
		path := filepath.Join(dir, entry.Name())

		if entry.IsDir() {
			skill, err := loadDirSkill(path)
			if err != nil {
				continue
			}
			skills = append(skills, skill)
		} else if strings.HasSuffix(entry.Name(), ".md") {
			skill, err := loadFileSkill(path)
			if err != nil {
				continue
			}
			skill.BasePath = dir
			skills = append(skills, skill)
		}
	}
	return skills, nil
}

func loadFileSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSkillFile(string(data))
}

func loadDirSkill(dir string) (*Skill, error) {
	skillFile := filepath.Join(dir, "SKILL.md")
	skill, err := loadFileSkill(skillFile)
	if err != nil {
		return nil, err
	}
	skill.BasePath = dir
	return skill, nil
}

func LoadMultiLayer(userDir, projectDir string) ([]*Skill, error) {
	result := make(map[string]*Skill)

	userSkills, err := LoadDir(userDir)
	if err != nil {
		return nil, err
	}
	for _, s := range userSkills {
		s.Source = "user"
		result[s.Name] = s
	}

	projSkills, err := LoadDir(projectDir)
	if err != nil {
		return nil, err
	}
	for _, s := range projSkills {
		s.Source = "project"
		result[s.Name] = s
	}

	skills := make([]*Skill, 0, len(result))
	for _, s := range result {
		skills = append(skills, s)
	}
	return skills, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./skills/ -v -run TestLoad`
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add skills/loader.go skills/loader_test.go
git commit -m "feat(skills): add filesystem loader with multi-layer support"
```

---

### Task 3: Skills index and matching

**Files:**
- Create: `skills/index.go`
- Create: `skills/index_test.go`

- [ ] **Step 1: Write the test file `skills/index_test.go`**

```go
package skills

import (
	"testing"
)

func makeTestSkills() []*Skill {
	return []*Skill{
		{
			Name:        "code-review",
			Description: "Structured code review process",
			Tags:        []string{"review", "quality"},
			Triggers:    TriggerConfig{Keywords: []string{"review", "code review"}, Patterns: []string{"review this"}},
			Priority:    10,
		},
		{
			Name:        "golang-best-practices",
			Description: "Best practices for Go development",
			Tags:        []string{"golang", "best-practices"},
			Triggers:    TriggerConfig{Keywords: []string{"best practice", "golang tips"}},
			Priority:    5,
		},
		{
			Name:        "migration-helper",
			Description: "Database migration workflow",
			Tags:        []string{"database", "migration"},
			Triggers:    TriggerConfig{Keywords: []string{"migrate", "migration"}, Patterns: []string{`create.*migration`}},
			Priority:    8,
		},
	}
}

func TestIndexMatchByName(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	results := idx.MatchByName("code-review")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "code-review" {
		t.Errorf("expected 'code-review', got %q", results[0].Name)
	}
}

func TestIndexMatchByKeyword(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	results := idx.Match("please review my code")
	found := false
	for _, r := range results {
		if r.Name == "code-review" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'code-review' to match keyword 'review'")
	}
}

func TestIndexMatchByPattern(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	results := idx.Match("create a new migration for users table")
	found := false
	for _, r := range results {
		if r.Name == "migration-helper" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'migration-helper' to match pattern 'create.*migration'")
	}
}

func TestIndexMatchSortedByPriority(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	// "review" matches code-review; inject a second skill with lower priority that also matches
	results := idx.Match("review best practice tips")
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	// code-review (priority 10) should come before golang-best-practices (priority 5)
	if results[0].Priority < results[1].Priority {
		t.Errorf("results not sorted by priority: %d < %d", results[0].Priority, results[1].Priority)
	}
}

func TestIndexMatchMaxResults(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	idx.MaxResults = 1
	results := idx.Match("review best practice migration")
	if len(results) > 1 {
		t.Errorf("expected at most 1 result, got %d", len(results))
	}
}

func TestIndexList(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	all := idx.List()
	if len(all) != 3 {
		t.Errorf("expected 3 skills, got %d", len(all))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./skills/ -v -run TestIndex`
Expected: compilation error — `NewIndex`, `Index` undefined

- [ ] **Step 3: Write `skills/index.go`**

```go
package skills

import (
	"regexp"
	"sort"
	"strings"
)

type Index struct {
	skills     []*Skill
	MaxResults int
}

func NewIndex(skills []*Skill) *Index {
	return &Index{
		skills:     skills,
		MaxResults: 3,
	}
}

func (idx *Index) Reload(skills []*Skill) {
	idx.skills = skills
}

func (idx *Index) List() []*Skill {
	cp := make([]*Skill, len(idx.skills))
	copy(cp, idx.skills)
	return cp
}

func (idx *Index) MatchByName(name string) []*Skill {
	for _, s := range idx.skills {
		if s.Name == name {
			return []*Skill{s}
		}
	}
	return nil
}

type matchResult struct {
	skill *Skill
	score int
}

func (idx *Index) Match(input string) []*Skill {
	lower := strings.ToLower(input)
	var results []matchResult

	seen := make(map[string]bool)
	for _, s := range idx.skills {
		score := idx.scoreSkill(s, lower)
		if score > 0 && !seen[s.Name] {
			results = append(results, matchResult{skill: s, score: score})
			seen[s.Name] = true
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].skill.Priority > results[j].skill.Priority
	})

	max := idx.MaxResults
	if max <= 0 {
		max = 3
	}
	if len(results) > max {
		results = results[:max]
	}

	skills := make([]*Skill, len(results))
	for i, r := range results {
		skills[i] = r.skill
	}
	return skills
}

func (idx *Index) scoreSkill(s *Skill, lowerInput string) int {
	score := 0

	// Keyword match (highest weight)
	for _, kw := range s.Triggers.Keywords {
		if strings.Contains(lowerInput, strings.ToLower(kw)) {
			score += 10
		}
	}

	// Pattern match
	for _, pat := range s.Triggers.Patterns {
		re, err := regexp.Compile("(?i)" + pat)
		if err != nil {
			continue
		}
		if re.MatchString(lowerInput) {
			score += 8
		}
	}

	// Tag match
	tokens := strings.Fields(lowerInput)
	for _, tag := range s.Tags {
		for _, tok := range tokens {
			if strings.EqualFold(tag, tok) {
				score += 3
			}
		}
	}

	// Description substring match (lowest weight)
	if strings.Contains(strings.ToLower(s.Description), lowerInput) {
		score += 1
	}
	for _, tok := range tokens {
		if len(tok) >= 3 && strings.Contains(strings.ToLower(s.Description), tok) {
			score += 1
		}
	}

	return score
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./skills/ -v -run TestIndex`
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add skills/index.go skills/index_test.go
git commit -m "feat(skills): add index with deterministic matching"
```

---

### Task 4: Rules loader

**Files:**
- Create: `rules/loader.go`
- Create: `rules/loader_test.go`

- [ ] **Step 1: Write the test file `rules/loader_test.go`**

```go
package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRulesFile(t *testing.T) {
	dir := t.TempDir()
	content := "# Rules\n\n- Always use gofmt\n- Never delete tests\n"
	path := filepath.Join(dir, "RULES.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(dir, "")
	rules := loader.Load()

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule set, got %d", len(rules))
	}
	if rules[0].Source != "user" {
		t.Errorf("expected source 'user', got %q", rules[0].Source)
	}
	if rules[0].Content != content {
		t.Errorf("unexpected content: %q", rules[0].Content)
	}
}

func TestLoadRulesBothLayers(t *testing.T) {
	userDir := t.TempDir()
	projDir := t.TempDir()

	os.WriteFile(filepath.Join(userDir, "RULES.md"), []byte("user rules"), 0644)
	os.WriteFile(filepath.Join(projDir, "RULES.md"), []byte("project rules"), 0644)

	loader := NewLoader(userDir, projDir)
	rules := loader.Load()

	if len(rules) != 2 {
		t.Fatalf("expected 2 rule sets, got %d", len(rules))
	}
	if rules[0].Source != "user" {
		t.Errorf("first should be user, got %q", rules[0].Source)
	}
	if rules[1].Source != "project" {
		t.Errorf("second should be project, got %q", rules[1].Source)
	}
}

func TestLoadRulesNoFile(t *testing.T) {
	loader := NewLoader("/nonexistent", "/also-nonexistent")
	rules := loader.Load()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./rules/ -v`
Expected: compilation error — `NewLoader` undefined

- [ ] **Step 3: Write `rules/loader.go`**

```go
package rules

import (
	"os"
	"path/filepath"
)

type RuleSet struct {
	Source  string // "user" | "project"
	Path   string
	Content string
}

type Loader struct {
	userDir    string
	projectDir string
}

func NewLoader(userDir, projectDir string) *Loader {
	return &Loader{
		userDir:    userDir,
		projectDir: projectDir,
	}
}

func (l *Loader) Load() []RuleSet {
	var rules []RuleSet

	if r, ok := loadRulesFile(l.userDir, "user"); ok {
		rules = append(rules, r)
	}
	if r, ok := loadRulesFile(l.projectDir, "project"); ok {
		rules = append(rules, r)
	}

	return rules
}

func loadRulesFile(dir, source string) (RuleSet, bool) {
	if dir == "" {
		return RuleSet{}, false
	}
	path := filepath.Join(dir, "RULES.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return RuleSet{}, false
	}
	return RuleSet{
		Source:  source,
		Path:   path,
		Content: string(data),
	}, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./rules/ -v`
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add rules/loader.go rules/loader_test.go
git commit -m "feat(rules): add rules loader with user/project layers"
```

---

### Task 5: PromptBuilder with source tracking

**Files:**
- Create: `prompt/source.go`
- Create: `prompt/builder.go`
- Create: `prompt/trace.go`
- Create: `prompt/builder_test.go`

- [ ] **Step 1: Write the test file `prompt/builder_test.go`**

```go
package prompt

import (
	"strings"
	"testing"
)

func TestBuilderAssemblyOrder(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "base_system", Type: TypeSystem, Content: "You are helpful."})
	b.AddSource(Source{Name: "user_rules", Type: TypeRules, Content: "Always use gofmt."})
	b.AddSource(Source{Name: "project_rules", Type: TypeRules, Content: "No globals."})
	b.AddSource(Source{Name: "skill:code-review", Type: TypeSkill, Content: "Review workflow."})

	result, trace := b.Build()

	// Verify order: system → rules → skill
	sysIdx := strings.Index(result, "You are helpful.")
	rulesIdx := strings.Index(result, "Always use gofmt.")
	skillIdx := strings.Index(result, "Review workflow.")

	if sysIdx >= rulesIdx || rulesIdx >= skillIdx {
		t.Errorf("wrong order: sys=%d, rules=%d, skill=%d", sysIdx, rulesIdx, skillIdx)
	}

	if len(trace.Sources) != 4 {
		t.Errorf("expected 4 sources in trace, got %d", len(trace.Sources))
	}
}

func TestBuilderEmpty(t *testing.T) {
	b := NewBuilder()
	result, trace := b.Build()
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if len(trace.Sources) != 0 {
		t.Errorf("expected 0 sources, got %d", len(trace.Sources))
	}
}

func TestBuilderReset(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "test", Type: TypeSystem, Content: "hello"})
	b.Reset()
	result, trace := b.Build()
	if result != "" {
		t.Errorf("expected empty after reset, got %q", result)
	}
	if len(trace.Sources) != 0 {
		t.Errorf("expected 0 sources after reset, got %d", len(trace.Sources))
	}
}

func TestBuilderTraceSourceNames(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "base_system", Type: TypeSystem, Content: "sys"})
	b.AddSource(Source{Name: "skill:review", Type: TypeSkill, Content: "review"})

	_, trace := b.Build()
	if trace.Sources[0] != "base_system" {
		t.Errorf("expected 'base_system', got %q", trace.Sources[0])
	}
	if trace.Sources[1] != "skill:review" {
		t.Errorf("expected 'skill:review', got %q", trace.Sources[1])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./prompt/ -v`
Expected: compilation error

- [ ] **Step 3: Write `prompt/source.go`**

```go
package prompt

const (
	TypeSystem = "system"
	TypeRules  = "rules"
	TypeSkill  = "skill"
	TypeUser   = "user"
)

type Source struct {
	Name    string
	Type    string
	Content string
}
```

- [ ] **Step 4: Write `prompt/trace.go`**

```go
package prompt

import (
	"fmt"
	"strings"
	"time"
)

type Trace struct {
	Timestamp     time.Time
	RulesLoaded   []string
	SkillsMatched []string
	Sources       []string
	TotalChars    int
}

func (t *Trace) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Prompt Trace (%s)\n", t.Timestamp.Format("15:04:05")))
	sb.WriteString(fmt.Sprintf("  Rules loaded:   %s\n", strings.Join(t.RulesLoaded, ", ")))
	sb.WriteString(fmt.Sprintf("  Skills matched: %s\n", strings.Join(t.SkillsMatched, ", ")))
	sb.WriteString(fmt.Sprintf("  Sources:        %s\n", strings.Join(t.Sources, " → ")))
	sb.WriteString(fmt.Sprintf("  Total chars:    %d\n", t.TotalChars))
	return sb.String()
}
```

- [ ] **Step 5: Write `prompt/builder.go`**

```go
package prompt

import (
	"strings"
	"time"
)

type Builder struct {
	sources []Source
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) AddSource(s Source) {
	b.sources = append(b.sources, s)
}

func (b *Builder) Reset() {
	b.sources = nil
}

func (b *Builder) Build() (string, *Trace) {
	trace := &Trace{
		Timestamp: time.Now(),
	}

	var parts []string
	for _, s := range b.sources {
		if s.Content == "" {
			continue
		}
		parts = append(parts, s.Content)
		trace.Sources = append(trace.Sources, s.Name)

		switch s.Type {
		case TypeRules:
			trace.RulesLoaded = append(trace.RulesLoaded, s.Name)
		case TypeSkill:
			trace.SkillsMatched = append(trace.SkillsMatched, s.Name)
		}
	}

	result := strings.Join(parts, "\n\n")
	trace.TotalChars = len(result)
	return result, trace
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./prompt/ -v`
Expected: all 4 tests PASS

- [ ] **Step 7: Commit**

```bash
git add prompt/source.go prompt/builder.go prompt/trace.go prompt/builder_test.go
git commit -m "feat(prompt): add PromptBuilder with source tracking and trace"
```

---

### Task 6: Command interface and registry

**Files:**
- Create: `commands/command.go`
- Create: `commands/registry.go`
- Create: `commands/registry_test.go`

- [ ] **Step 1: Write the test file `commands/registry_test.go`**

```go
package commands

import (
	"testing"
)

type testCommand struct {
	name    string
	aliases []string
	desc    string
	usage   string
}

func (c *testCommand) Name() string        { return c.name }
func (c *testCommand) Aliases() []string    { return c.aliases }
func (c *testCommand) Description() string  { return c.desc }
func (c *testCommand) Usage() string        { return c.usage }
func (c *testCommand) Execute(ctx CommandContext) (CommandResult, error) {
	return CommandResult{Message: "executed " + c.name}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	cmd := &testCommand{name: "help", aliases: []string{"h", "?"}, desc: "Show help", usage: "/help"}
	reg.Register(cmd)

	got, ok := reg.Get("help")
	if !ok {
		t.Fatal("expected to find 'help'")
	}
	if got.Name() != "help" {
		t.Errorf("expected name 'help', got %q", got.Name())
	}
}

func TestRegistryAlias(t *testing.T) {
	reg := NewRegistry()
	cmd := &testCommand{name: "help", aliases: []string{"h", "?"}, desc: "Show help", usage: "/help"}
	reg.Register(cmd)

	got, ok := reg.Get("h")
	if !ok {
		t.Fatal("expected to find alias 'h'")
	}
	if got.Name() != "help" {
		t.Errorf("expected name 'help' via alias, got %q", got.Name())
	}

	got2, ok := reg.Get("?")
	if !ok {
		t.Fatal("expected to find alias '?'")
	}
	if got2.Name() != "help" {
		t.Errorf("expected name 'help' via alias '?', got %q", got2.Name())
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testCommand{name: "help", aliases: []string{"h"}, desc: "Show help", usage: "/help"})
	reg.Register(&testCommand{name: "clear", aliases: []string{"c"}, desc: "Clear", usage: "/clear"})

	items := reg.List()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestRegistryParseAndExecute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testCommand{name: "help", aliases: []string{"h"}, desc: "Show help", usage: "/help"})

	result, err := reg.Execute("/help", CommandContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "executed help" {
		t.Errorf("expected 'executed help', got %q", result.Message)
	}
}

func TestRegistryExecuteWithAlias(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&testCommand{name: "help", aliases: []string{"h"}, desc: "Show help", usage: "/help"})

	result, err := reg.Execute("/h", CommandContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "executed help" {
		t.Errorf("expected 'executed help', got %q", result.Message)
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute("/unknown", CommandContext{})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./commands/ -v`
Expected: compilation error

- [ ] **Step 3: Write `commands/command.go`**

```go
package commands

type CommandAction int

const (
	ActionNone CommandAction = iota
	ActionQuit
	ActionClearScreen
)

type CommandContext struct {
	Args   []string
	Output func(string)
}

type CommandResult struct {
	Message string
	Action  CommandAction
}

type Command interface {
	Name() string
	Aliases() []string
	Description() string
	Usage() string
	Execute(ctx CommandContext) (CommandResult, error)
}
```

Note: `CommandContext` does NOT hold `*Runtime` yet — that creates a circular import (`commands` → `runtime` → `commands`). Instead, each built-in command will receive what it needs through closures or a `RuntimeAccessor` interface. We'll add this in Task 9 when we wire up Runtime.

- [ ] **Step 4: Write `commands/registry.go`**

```go
package commands

import (
	"fmt"
	"strings"
)

type CommandInfo struct {
	Name        string
	Aliases     []string
	Description string
	HasArgs     bool
}

type Registry struct {
	commands map[string]Command
	aliases  map[string]string
	order    []string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}
}

func (r *Registry) Register(cmd Command) {
	name := cmd.Name()
	r.commands[name] = cmd
	r.order = append(r.order, name)
	for _, alias := range cmd.Aliases() {
		r.aliases[alias] = name
	}
}

func (r *Registry) Get(nameOrAlias string) (Command, bool) {
	if cmd, ok := r.commands[nameOrAlias]; ok {
		return cmd, true
	}
	if realName, ok := r.aliases[nameOrAlias]; ok {
		return r.commands[realName], true
	}
	return nil, false
}

func (r *Registry) List() []CommandInfo {
	var items []CommandInfo
	for _, name := range r.order {
		cmd := r.commands[name]
		hasArgs := strings.Contains(cmd.Usage(), "<")
		items = append(items, CommandInfo{
			Name:        cmd.Name(),
			Aliases:     cmd.Aliases(),
			Description: cmd.Description(),
			HasArgs:     hasArgs,
		})
	}
	return items
}

func (r *Registry) Execute(input string, ctx CommandContext) (CommandResult, error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return CommandResult{}, fmt.Errorf("empty command")
	}

	name := strings.TrimPrefix(parts[0], "/")
	cmd, ok := r.Get(name)
	if !ok {
		return CommandResult{}, fmt.Errorf("unknown command: /%s", name)
	}

	ctx.Args = parts[1:]
	return cmd.Execute(ctx)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./commands/ -v`
Expected: all 6 tests PASS

- [ ] **Step 6: Commit**

```bash
git add commands/command.go commands/registry.go commands/registry_test.go
git commit -m "feat(commands): add Command interface and Registry"
```

---

### Task 7: Built-in commands

**Files:**
- Create: `commands/builtin/help.go`
- Create: `commands/builtin/clear.go`
- Create: `commands/builtin/quit.go`
- Create: `commands/builtin/provider.go`
- Create: `commands/builtin/model.go`
- Create: `commands/builtin/skills_cmd.go`
- Create: `commands/builtin/skill_cmd.go`
- Create: `commands/builtin/reload.go`
- Create: `commands/builtin/prompt_cmd.go`
- Create: `commands/builtin/tools_cmd.go`
- Create: `commands/builtin/builtin_test.go`

Built-in commands need access to Runtime state (skills index, prompt trace, tool registry, etc). To avoid circular imports (`commands/builtin` → `runtime` → `commands`), we define a `RuntimeAccessor` interface in the `commands` package that Runtime will implement.

- [ ] **Step 1: Add `RuntimeAccessor` interface to `commands/command.go`**

Append to `commands/command.go`:

```go
type RuntimeAccessor interface {
	ListSkills() []SkillInfo
	ActivateSkill(name string) error
	ReloadSkillsAndRules() error
	LastPromptTrace() string
	ListTools() []ToolInfo
	SwitchProvider(name string) error
	SwitchModel(name string) error
	CurrentProvider() string
	CurrentModel() string
	AvailableProviders() []string
	ClearHistory()
}

type SkillInfo struct {
	Name        string
	Description string
	Source      string
	Priority    int
	Tags        []string
}

type ToolInfo struct {
	Name        string
	Description string
}
```

Update `CommandContext` to include accessor:

```go
type CommandContext struct {
	Args     []string
	Output   func(string)
	Runtime  RuntimeAccessor
}
```

- [ ] **Step 2: Write `commands/builtin/help.go`**

```go
package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type HelpCommand struct {
	registry *commands.Registry
}

func NewHelp(registry *commands.Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Aliases() []string    { return []string{"h", "?"} }
func (c *HelpCommand) Description() string  { return "List all available commands" }
func (c *HelpCommand) Usage() string        { return "/help" }

func (c *HelpCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	var sb strings.Builder
	sb.WriteString("Available commands:\n\n")
	for _, info := range c.registry.List() {
		aliases := ""
		if len(info.Aliases) > 0 {
			aliases = fmt.Sprintf(" (/%s)", strings.Join(info.Aliases, ", /"))
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s%s\n", "/"+info.Name, info.Description, aliases))
	}
	return commands.CommandResult{Message: sb.String()}, nil
}
```

- [ ] **Step 3: Write `commands/builtin/clear.go`**

```go
package builtin

import "github.com/fengxuan/go-agent/commands"

type ClearCommand struct{}

func NewClear() *ClearCommand { return &ClearCommand{} }

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Aliases() []string    { return []string{"c"} }
func (c *ClearCommand) Description() string  { return "Clear conversation history" }
func (c *ClearCommand) Usage() string        { return "/clear" }

func (c *ClearCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime != nil {
		ctx.Runtime.ClearHistory()
	}
	return commands.CommandResult{Message: "Conversation cleared.", Action: commands.ActionClearScreen}, nil
}
```

- [ ] **Step 4: Write `commands/builtin/quit.go`**

```go
package builtin

import "github.com/fengxuan/go-agent/commands"

type QuitCommand struct{}

func NewQuit() *QuitCommand { return &QuitCommand{} }

func (c *QuitCommand) Name() string        { return "quit" }
func (c *QuitCommand) Aliases() []string    { return []string{"q", "exit"} }
func (c *QuitCommand) Description() string  { return "Exit the program" }
func (c *QuitCommand) Usage() string        { return "/quit" }

func (c *QuitCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	return commands.CommandResult{Message: "Goodbye!", Action: commands.ActionQuit}, nil
}
```

- [ ] **Step 5: Write `commands/builtin/provider.go`**

```go
package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type ProviderCommand struct{}

func NewProvider() *ProviderCommand { return &ProviderCommand{} }

func (c *ProviderCommand) Name() string        { return "provider" }
func (c *ProviderCommand) Aliases() []string    { return []string{"p"} }
func (c *ProviderCommand) Description() string  { return "Switch LLM provider" }
func (c *ProviderCommand) Usage() string        { return "/provider <name>" }

func (c *ProviderCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	if len(ctx.Args) == 0 {
		current := ctx.Runtime.CurrentProvider()
		available := ctx.Runtime.AvailableProviders()
		msg := fmt.Sprintf("Current provider: %s\nAvailable: %s", current, strings.Join(available, ", "))
		return commands.CommandResult{Message: msg}, nil
	}
	if err := ctx.Runtime.SwitchProvider(ctx.Args[0]); err != nil {
		return commands.CommandResult{}, err
	}
	return commands.CommandResult{Message: fmt.Sprintf("Switched to provider: %s", ctx.Args[0])}, nil
}
```

- [ ] **Step 6: Write `commands/builtin/model.go`**

```go
package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

type ModelCommand struct{}

func NewModel() *ModelCommand { return &ModelCommand{} }

func (c *ModelCommand) Name() string        { return "model" }
func (c *ModelCommand) Aliases() []string    { return []string{"m"} }
func (c *ModelCommand) Description() string  { return "Switch model" }
func (c *ModelCommand) Usage() string        { return "/model <name>" }

func (c *ModelCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	if len(ctx.Args) == 0 {
		msg := fmt.Sprintf("Current model: %s", ctx.Runtime.CurrentModel())
		return commands.CommandResult{Message: msg}, nil
	}
	if err := ctx.Runtime.SwitchModel(ctx.Args[0]); err != nil {
		return commands.CommandResult{}, err
	}
	return commands.CommandResult{Message: fmt.Sprintf("Switched to model: %s", ctx.Args[0])}, nil
}
```

- [ ] **Step 7: Write `commands/builtin/skills_cmd.go`**

```go
package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type SkillsCommand struct{}

func NewSkills() *SkillsCommand { return &SkillsCommand{} }

func (c *SkillsCommand) Name() string        { return "skills" }
func (c *SkillsCommand) Aliases() []string    { return []string{"ss"} }
func (c *SkillsCommand) Description() string  { return "List all loaded skills" }
func (c *SkillsCommand) Usage() string        { return "/skills" }

func (c *SkillsCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	skills := ctx.Runtime.ListSkills()
	if len(skills) == 0 {
		return commands.CommandResult{Message: "No skills loaded."}, nil
	}

	var sb strings.Builder
	sb.WriteString("Loaded skills:\n\n")
	for _, s := range skills {
		tags := ""
		if len(s.Tags) > 0 {
			tags = fmt.Sprintf(" [%s]", strings.Join(s.Tags, ", "))
		}
		sb.WriteString(fmt.Sprintf("  %-25s %s (source: %s, priority: %d)%s\n",
			s.Name, s.Description, s.Source, s.Priority, tags))
	}
	return commands.CommandResult{Message: sb.String()}, nil
}
```

- [ ] **Step 8: Write `commands/builtin/skill_cmd.go`**

```go
package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

type SkillCommand struct{}

func NewSkill() *SkillCommand { return &SkillCommand{} }

func (c *SkillCommand) Name() string        { return "skill" }
func (c *SkillCommand) Aliases() []string    { return []string{"s"} }
func (c *SkillCommand) Description() string  { return "Activate a skill for the next request" }
func (c *SkillCommand) Usage() string        { return "/skill <name>" }

func (c *SkillCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	if len(ctx.Args) == 0 {
		return commands.CommandResult{}, fmt.Errorf("usage: /skill <name>")
	}
	name := ctx.Args[0]
	if err := ctx.Runtime.ActivateSkill(name); err != nil {
		return commands.CommandResult{}, err
	}
	return commands.CommandResult{Message: fmt.Sprintf("Skill '%s' activated for next request.", name)}, nil
}
```

- [ ] **Step 9: Write `commands/builtin/reload.go`**

```go
package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

type ReloadCommand struct{}

func NewReload() *ReloadCommand { return &ReloadCommand{} }

func (c *ReloadCommand) Name() string        { return "reload" }
func (c *ReloadCommand) Aliases() []string    { return []string{"r"} }
func (c *ReloadCommand) Description() string  { return "Reload skills and rules" }
func (c *ReloadCommand) Usage() string        { return "/reload" }

func (c *ReloadCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	if err := ctx.Runtime.ReloadSkillsAndRules(); err != nil {
		return commands.CommandResult{}, err
	}
	return commands.CommandResult{Message: "Skills and rules reloaded."}, nil
}
```

- [ ] **Step 10: Write `commands/builtin/prompt_cmd.go`**

```go
package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

type PromptCommand struct{}

func NewPrompt() *PromptCommand { return &PromptCommand{} }

func (c *PromptCommand) Name() string        { return "prompt" }
func (c *PromptCommand) Aliases() []string    { return nil }
func (c *PromptCommand) Description() string  { return "Show last prompt assembly trace" }
func (c *PromptCommand) Usage() string        { return "/prompt" }

func (c *PromptCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	trace := ctx.Runtime.LastPromptTrace()
	if trace == "" {
		return commands.CommandResult{Message: "No prompt trace available yet. Send a message first."}, nil
	}
	return commands.CommandResult{Message: trace}, nil
}
```

- [ ] **Step 11: Write `commands/builtin/tools_cmd.go`**

```go
package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type ToolsCommand struct{}

func NewTools() *ToolsCommand { return &ToolsCommand{} }

func (c *ToolsCommand) Name() string        { return "tools" }
func (c *ToolsCommand) Aliases() []string    { return []string{"t"} }
func (c *ToolsCommand) Description() string  { return "List all registered tools" }
func (c *ToolsCommand) Usage() string        { return "/tools" }

func (c *ToolsCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime == nil {
		return commands.CommandResult{}, fmt.Errorf("runtime not available")
	}
	tools := ctx.Runtime.ListTools()
	if len(tools) == 0 {
		return commands.CommandResult{Message: "No tools registered."}, nil
	}

	var sb strings.Builder
	sb.WriteString("Registered tools:\n\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", t.Name, t.Description))
	}
	return commands.CommandResult{Message: sb.String()}, nil
}
```

- [ ] **Step 12: Write `commands/builtin/builtin_test.go`**

```go
package builtin

import (
	"testing"

	"github.com/fengxuan/go-agent/commands"
)

type mockRuntime struct {
	skills    []commands.SkillInfo
	tools     []commands.ToolInfo
	provider  string
	model     string
	providers []string
	activated string
	reloaded  bool
	cleared   bool
}

func (m *mockRuntime) ListSkills() []commands.SkillInfo     { return m.skills }
func (m *mockRuntime) ActivateSkill(name string) error      { m.activated = name; return nil }
func (m *mockRuntime) ReloadSkillsAndRules() error          { m.reloaded = true; return nil }
func (m *mockRuntime) LastPromptTrace() string               { return "trace output" }
func (m *mockRuntime) ListTools() []commands.ToolInfo        { return m.tools }
func (m *mockRuntime) SwitchProvider(name string) error      { m.provider = name; return nil }
func (m *mockRuntime) SwitchModel(name string) error         { m.model = name; return nil }
func (m *mockRuntime) CurrentProvider() string               { return m.provider }
func (m *mockRuntime) CurrentModel() string                  { return m.model }
func (m *mockRuntime) AvailableProviders() []string          { return m.providers }
func (m *mockRuntime) ClearHistory()                         { m.cleared = true }

func TestHelpCommand(t *testing.T) {
	reg := commands.NewRegistry()
	help := NewHelp(reg)
	reg.Register(help)
	reg.Register(NewClear())

	result, err := help.Execute(commands.CommandContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty help output")
	}
}

func TestClearCommand(t *testing.T) {
	rt := &mockRuntime{}
	cmd := NewClear()
	result, err := cmd.Execute(commands.CommandContext{Runtime: rt})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != commands.ActionClearScreen {
		t.Error("expected ActionClearScreen")
	}
	if !rt.cleared {
		t.Error("expected ClearHistory to be called")
	}
}

func TestQuitCommand(t *testing.T) {
	cmd := NewQuit()
	result, err := cmd.Execute(commands.CommandContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Action != commands.ActionQuit {
		t.Error("expected ActionQuit")
	}
}

func TestSkillActivation(t *testing.T) {
	rt := &mockRuntime{}
	cmd := NewSkill()
	_, err := cmd.Execute(commands.CommandContext{Runtime: rt, Args: []string{"code-review"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.activated != "code-review" {
		t.Errorf("expected activated 'code-review', got %q", rt.activated)
	}
}

func TestReloadCommand(t *testing.T) {
	rt := &mockRuntime{}
	cmd := NewReload()
	_, err := cmd.Execute(commands.CommandContext{Runtime: rt})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rt.reloaded {
		t.Error("expected ReloadSkillsAndRules to be called")
	}
}

func TestPromptTraceCommand(t *testing.T) {
	rt := &mockRuntime{}
	cmd := NewPrompt()
	result, err := cmd.Execute(commands.CommandContext{Runtime: rt})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "trace output" {
		t.Errorf("expected trace output, got %q", result.Message)
	}
}

func TestToolsCommand(t *testing.T) {
	rt := &mockRuntime{
		tools: []commands.ToolInfo{
			{Name: "read_file", Description: "Read a file"},
		},
	}
	cmd := NewTools()
	result, err := cmd.Execute(commands.CommandContext{Runtime: rt})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty tools output")
	}
}

func TestProviderSwitchCommand(t *testing.T) {
	rt := &mockRuntime{provider: "openai", providers: []string{"openai", "anthropic"}}
	cmd := NewProvider()
	_, err := cmd.Execute(commands.CommandContext{Runtime: rt, Args: []string{"anthropic"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", rt.provider)
	}
}
```

- [ ] **Step 13: Run all tests**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./commands/... -v`
Expected: all tests PASS

- [ ] **Step 14: Commit**

```bash
git add commands/
git commit -m "feat(commands): add 10 built-in slash commands with RuntimeAccessor"
```

---

### Task 8: Agent modifications

**Files:**
- Modify: `agent/agent.go` — add `SetSystemPrompt()` method
- Modify: `agent/message.go` — add `EventPromptTrace`

- [ ] **Step 1: Add `EventPromptTrace` to `agent/message.go`**

Add after the `EventDelta` constant:

```go
EventPromptTrace
```

Add to the `String()` switch:

```go
case EventPromptTrace:
    return "PromptTrace"
```

- [ ] **Step 2: Add `SetSystemPrompt` to `agent/agent.go`**

Add after the `ClearHistory` method:

```go
func (a *Agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.SystemPrompt = prompt
}
```

- [ ] **Step 3: Run existing agent tests to verify no regressions**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./agent/ -v`
Expected: all 5 existing tests PASS

- [ ] **Step 4: Commit**

```bash
git add agent/agent.go agent/message.go
git commit -m "feat(agent): add SetSystemPrompt and EventPromptTrace"
```

---

### Task 9: Runtime coordinator

**Files:**
- Create: `runtime/runtime.go`
- Create: `runtime/runtime_test.go`

- [ ] **Step 1: Write `runtime/runtime.go`**

```go
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/commands"
	"github.com/fengxuan/go-agent/commands/builtin"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/prompt"
	"github.com/fengxuan/go-agent/rules"
	"github.com/fengxuan/go-agent/skills"
	"github.com/fengxuan/go-agent/tools"
)

const baseSystemPrompt = `You are a helpful AI assistant with access to tools.
When you need to find information, use the tavily_search tool.
When you need to run commands, use the shell_exec tool.
When you need to read files, use the read_file tool.
When you need to write files, use the write_file tool.
Always explain your reasoning before using tools.`

type Runtime struct {
	Config          *config.Config
	Agent           *agent.Agent
	ToolRegistry    *tools.Registry
	SkillIndex      *skills.Index
	RulesLoader     *rules.Loader
	CommandRegistry *commands.Registry

	mu           sync.Mutex
	activeSkills []string
	lastTrace    *prompt.Trace
	provider     string
	model        string
	userDir      string
	projectDir   string
	createClient func(provider string, cfg config.ProviderConfig) client.LLMClient
}

type Config struct {
	AppConfig    *config.Config
	ToolRegistry *tools.Registry
	CreateClient func(provider string, cfg config.ProviderConfig) client.LLMClient
}

func New(cfg Config) (*Runtime, error) {
	home, _ := os.UserHomeDir()
	userDir := filepath.Join(home, ".go-agent")
	projectDir := ".go-agent"

	providerName := cfg.AppConfig.DefaultProvider
	if providerName == "" {
		providerName = "openai"
	}
	providerCfg, ok := cfg.AppConfig.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in config", providerName)
	}

	llmClient := cfg.CreateClient(providerName, providerCfg)

	ag := agent.New(llmClient, cfg.ToolRegistry, agent.AgentConfig{
		SystemPrompt:  baseSystemPrompt,
		MaxIterations: cfg.AppConfig.MaxIterations,
		Model:         providerCfg.Model,
	})

	rulesLoader := rules.NewLoader(userDir, projectDir)

	loadedSkills, _ := skills.LoadMultiLayer(
		filepath.Join(userDir, "skills"),
		filepath.Join(projectDir, "skills"),
	)
	skillIndex := skills.NewIndex(loadedSkills)

	cmdRegistry := commands.NewRegistry()

	rt := &Runtime{
		Config:          cfg.AppConfig,
		Agent:           ag,
		ToolRegistry:    cfg.ToolRegistry,
		SkillIndex:      skillIndex,
		RulesLoader:     rulesLoader,
		CommandRegistry: cmdRegistry,
		provider:        providerName,
		model:           providerCfg.Model,
		userDir:         userDir,
		projectDir:      projectDir,
		createClient:    cfg.CreateClient,
	}

	rt.registerBuiltinCommands()

	return rt, nil
}

func (rt *Runtime) registerBuiltinCommands() {
	rt.CommandRegistry.Register(builtin.NewHelp(rt.CommandRegistry))
	rt.CommandRegistry.Register(builtin.NewClear())
	rt.CommandRegistry.Register(builtin.NewProvider())
	rt.CommandRegistry.Register(builtin.NewModel())
	rt.CommandRegistry.Register(builtin.NewSkills())
	rt.CommandRegistry.Register(builtin.NewSkill())
	rt.CommandRegistry.Register(builtin.NewReload())
	rt.CommandRegistry.Register(builtin.NewPrompt())
	rt.CommandRegistry.Register(builtin.NewTools())
	rt.CommandRegistry.Register(builtin.NewQuit())
}

func (rt *Runtime) RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent {
	builder := prompt.NewBuilder()

	builder.AddSource(prompt.Source{Name: "base_system", Type: prompt.TypeSystem, Content: baseSystemPrompt})

	ruleSets := rt.RulesLoader.Load()
	for _, r := range ruleSets {
		builder.AddSource(prompt.Source{
			Name:    r.Source + "_rules",
			Type:    prompt.TypeRules,
			Content: r.Content,
		})
	}

	matched := rt.SkillIndex.Match(input)
	for _, s := range matched {
		builder.AddSource(prompt.Source{
			Name:    "skill:" + s.Name + " (auto)",
			Type:    prompt.TypeSkill,
			Content: s.Body,
		})
	}

	rt.mu.Lock()
	for _, name := range rt.activeSkills {
		results := rt.SkillIndex.MatchByName(name)
		for _, s := range results {
			builder.AddSource(prompt.Source{
				Name:    "skill:" + s.Name + " (explicit)",
				Type:    prompt.TypeSkill,
				Content: s.Body,
			})
		}
	}
	rt.activeSkills = nil
	rt.mu.Unlock()

	builtPrompt, trace := builder.Build()
	rt.mu.Lock()
	rt.lastTrace = trace
	rt.mu.Unlock()

	rt.Agent.SetSystemPrompt(builtPrompt)

	ch := rt.Agent.Run(ctx, input)

	wrappedCh := make(chan agent.AgentEvent, 32)
	go func() {
		defer close(wrappedCh)
		// Send trace event first
		select {
		case wrappedCh <- agent.AgentEvent{
			Type:    agent.EventPromptTrace,
			Content: trace.String(),
		}:
		case <-ctx.Done():
			return
		}
		for ev := range ch {
			select {
			case wrappedCh <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	return wrappedCh
}

func (rt *Runtime) ExecuteCommand(input string) (commands.CommandResult, error) {
	ctx := commands.CommandContext{
		Runtime: rt,
	}
	return rt.CommandRegistry.Execute(input, ctx)
}

// --- RuntimeAccessor implementation ---

func (rt *Runtime) ListSkills() []commands.SkillInfo {
	var result []commands.SkillInfo
	for _, s := range rt.SkillIndex.List() {
		result = append(result, commands.SkillInfo{
			Name:        s.Name,
			Description: s.Description,
			Source:      s.Source,
			Priority:    s.Priority,
			Tags:        s.Tags,
		})
	}
	return result
}

func (rt *Runtime) ActivateSkill(name string) error {
	results := rt.SkillIndex.MatchByName(name)
	if len(results) == 0 {
		return fmt.Errorf("skill %q not found", name)
	}
	rt.mu.Lock()
	rt.activeSkills = append(rt.activeSkills, name)
	rt.mu.Unlock()
	return nil
}

func (rt *Runtime) ReloadSkillsAndRules() error {
	loadedSkills, err := skills.LoadMultiLayer(
		filepath.Join(rt.userDir, "skills"),
		filepath.Join(rt.projectDir, "skills"),
	)
	if err != nil {
		return err
	}
	rt.SkillIndex.Reload(loadedSkills)
	return nil
}

func (rt *Runtime) LastPromptTrace() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.lastTrace == nil {
		return ""
	}
	return rt.lastTrace.String()
}

func (rt *Runtime) ListTools() []commands.ToolInfo {
	var result []commands.ToolInfo
	for _, schema := range rt.ToolRegistry.ToolSchemas() {
		result = append(result, commands.ToolInfo{
			Name:        schema.Function.Name,
			Description: schema.Function.Description,
		})
	}
	return result
}

func (rt *Runtime) SwitchProvider(name string) error {
	provCfg, ok := rt.Config.Providers[name]
	if !ok {
		return fmt.Errorf("provider %q not found in config", name)
	}
	newClient := rt.createClient(name, provCfg)
	rt.Agent.Reset(newClient, agent.AgentConfig{
		SystemPrompt:  baseSystemPrompt,
		MaxIterations: rt.Config.MaxIterations,
		Model:         provCfg.Model,
	})
	rt.mu.Lock()
	rt.provider = name
	rt.model = provCfg.Model
	rt.mu.Unlock()
	return nil
}

func (rt *Runtime) SwitchModel(name string) error {
	rt.Agent.SetSystemPrompt(baseSystemPrompt)
	rt.mu.Lock()
	rt.model = name
	rt.mu.Unlock()
	return nil
}

func (rt *Runtime) CurrentProvider() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.provider
}

func (rt *Runtime) CurrentModel() string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.model
}

func (rt *Runtime) AvailableProviders() []string {
	var providers []string
	for name := range rt.Config.Providers {
		providers = append(providers, name)
	}
	return providers
}

func (rt *Runtime) ClearHistory() {
	rt.Agent.ClearHistory()
}
```

- [ ] **Step 2: Write `runtime/runtime_test.go`**

```go
package runtime

import (
	"context"
	"testing"

	"github.com/fengxuan/go-agent/agent"
	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/tools"
)

type mockLLMClient struct{}

func (m *mockLLMClient) ChatCompletion(ctx context.Context, req client.ChatRequest) (<-chan client.StreamChunk, error) {
	ch := make(chan client.StreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- client.StreamChunk{Delta: "test response"}
	}()
	return ch, nil
}

func makeTestConfig() *config.Config {
	return &config.Config{
		DefaultProvider: "openai",
		MaxIterations:   5,
		Providers: map[string]config.ProviderConfig{
			"openai": {APIKey: "test", Model: "gpt-4"},
		},
	}
}

func TestRuntimeNew(t *testing.T) {
	cfg := makeTestConfig()
	registry := tools.NewRegistry()

	rt, err := New(Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: func(provider string, cfg config.ProviderConfig) client.LLMClient {
			return &mockLLMClient{}
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt.CurrentProvider() != "openai" {
		t.Errorf("expected provider 'openai', got %q", rt.CurrentProvider())
	}
}

func TestRuntimeExecuteCommand(t *testing.T) {
	cfg := makeTestConfig()
	registry := tools.NewRegistry()

	rt, err := New(Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: func(provider string, cfg config.ProviderConfig) client.LLMClient {
			return &mockLLMClient{}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := rt.ExecuteCommand("/help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message == "" {
		t.Error("expected non-empty help message")
	}
}

func TestRuntimeRunUserInput(t *testing.T) {
	cfg := makeTestConfig()
	registry := tools.NewRegistry()

	rt, err := New(Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: func(provider string, cfg config.ProviderConfig) client.LLMClient {
			return &mockLLMClient{}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	ch := rt.RunUserInput(ctx, "hello")

	var events []agent.AgentEvent
	for ev := range ch {
		events = append(events, ev)
	}

	hasTrace := false
	for _, ev := range events {
		if ev.Type == agent.EventPromptTrace {
			hasTrace = true
		}
	}
	if !hasTrace {
		t.Error("expected EventPromptTrace event")
	}
}

func TestRuntimeActivateSkill(t *testing.T) {
	cfg := makeTestConfig()
	registry := tools.NewRegistry()

	rt, err := New(Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: func(provider string, cfg config.ProviderConfig) client.LLMClient {
			return &mockLLMClient{}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = rt.ActivateSkill("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./runtime/ -v`
Expected: all 4 tests PASS

- [ ] **Step 4: Commit**

```bash
git add runtime/runtime.go runtime/runtime_test.go
git commit -m "feat(runtime): add Runtime coordinator with subsystem wiring"
```

---

### Task 10: TUI completion panel

**Files:**
- Create: `tui/completion.go`
- Modify: `tui/styles.go` — add completion styles

- [ ] **Step 1: Add completion styles to `tui/styles.go`**

Append:

```go
	completionBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Padding(0, 1)

	completionItemStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	completionSelectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Bold(true)

	completionDescStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))
```

- [ ] **Step 2: Write `tui/completion.go`**

```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fengxuan/go-agent/commands"
)

type CompletionState struct {
	Active   bool
	Items    []CompletionItem
	Filtered []CompletionItem
	Selected int
	Prefix   string
}

type CompletionItem struct {
	Name        string
	Aliases     string
	Description string
	HasArgs     bool
}

func NewCompletionState(cmdInfos []commands.CommandInfo) CompletionState {
	items := make([]CompletionItem, 0, len(cmdInfos))
	for _, info := range cmdInfos {
		aliases := ""
		if len(info.Aliases) > 0 {
			aliases = strings.Join(info.Aliases, ",")
		}
		items = append(items, CompletionItem{
			Name:        info.Name,
			Aliases:     aliases,
			Description: info.Description,
			HasArgs:     info.HasArgs,
		})
	}
	return CompletionState{Items: items}
}

func (cs *CompletionState) Activate() {
	cs.Active = true
	cs.Prefix = ""
	cs.Selected = 0
	cs.filter()
}

func (cs *CompletionState) Deactivate() {
	cs.Active = false
	cs.Prefix = ""
	cs.Selected = 0
	cs.Filtered = nil
}

func (cs *CompletionState) UpdatePrefix(prefix string) {
	cs.Prefix = prefix
	cs.Selected = 0
	cs.filter()
}

func (cs *CompletionState) MoveUp() {
	if cs.Selected > 0 {
		cs.Selected--
	}
}

func (cs *CompletionState) MoveDown() {
	if cs.Selected < len(cs.Filtered)-1 {
		cs.Selected++
	}
}

func (cs *CompletionState) SelectedItem() (CompletionItem, bool) {
	if cs.Selected < 0 || cs.Selected >= len(cs.Filtered) {
		return CompletionItem{}, false
	}
	return cs.Filtered[cs.Selected], true
}

func (cs *CompletionState) filter() {
	if cs.Prefix == "" {
		cs.Filtered = make([]CompletionItem, len(cs.Items))
		copy(cs.Filtered, cs.Items)
		return
	}

	lower := strings.ToLower(cs.Prefix)
	cs.Filtered = nil
	for _, item := range cs.Items {
		if strings.HasPrefix(strings.ToLower(item.Name), lower) {
			cs.Filtered = append(cs.Filtered, item)
			continue
		}
		for _, alias := range strings.Split(item.Aliases, ",") {
			if alias != "" && strings.HasPrefix(strings.ToLower(alias), lower) {
				cs.Filtered = append(cs.Filtered, item)
				break
			}
		}
	}
}

func renderCompletion(cs CompletionState, width int) string {
	if !cs.Active || len(cs.Filtered) == 0 {
		return ""
	}

	maxVisible := 8
	items := cs.Filtered
	if len(items) > maxVisible {
		items = items[:maxVisible]
	}

	var rows []string
	for i, item := range items {
		aliasText := ""
		if item.Aliases != "" {
			aliasText = fmt.Sprintf(" (/%s)", item.Aliases)
		}
		name := fmt.Sprintf("/%-15s", item.Name)
		desc := item.Description + aliasText

		var line string
		if i == cs.Selected {
			line = completionSelectedStyle.Render(name) + " " + completionDescStyle.Render(desc)
		} else {
			line = completionItemStyle.Render(name) + " " + completionDescStyle.Render(desc)
		}
		rows = append(rows, line)
	}

	content := strings.Join(rows, "\n")

	panelWidth := width - 4
	if panelWidth < 30 {
		panelWidth = 30
	}
	return completionBorderStyle.Width(panelWidth).Render(content)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build ./tui/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add tui/completion.go tui/styles.go
git commit -m "feat(tui): add slash command completion panel"
```

---

### Task 11: Refactor TUI to use Runtime

**Files:**
- Modify: `tui/app.go` — replace callbacks with Runtime, integrate completion

- [ ] **Step 1: Rewrite `tui/app.go`**

Replace the existing `AppConfig` and related types with a Runtime-based approach. Key changes:

1. `AppConfig` takes a `Runtime` interface (to avoid import cycle) instead of individual callbacks
2. Completion state integrated into Model
3. Key handling updated for completion panel
4. Slash commands delegated to Runtime

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
	"github.com/fengxuan/go-agent/commands"
)

type RuntimeInterface interface {
	RunUserInput(ctx context.Context, input string) <-chan agent.AgentEvent
	ExecuteCommand(input string) (commands.CommandResult, error)
	CurrentProvider() string
	CurrentModel() string
	CommandRegistry() *commands.Registry
}

type AppConfig struct {
	Runtime RuntimeInterface
}

type agentEventMsg struct {
	event agent.AgentEvent
}

type agentDoneMsg struct{}

type tickMsg time.Time

type Model struct {
	config      AppConfig
	viewport    viewport.Model
	textarea    textarea.Model
	spinner     spinner.Model
	messageView *MessageView
	completion  CompletionState

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

	var completion CompletionState
	if config.Runtime != nil {
		completion = NewCompletionState(config.Runtime.CommandRegistry().List())
	}

	return Model{
		config:      config,
		spinner:     s,
		messageView: NewMessageView(),
		completion:  completion,
		state:       StateReady,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textarea.Blink,
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			vpHeight := m.height - 1 - 3 - 2
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport = viewport.New(m.width, vpHeight)
			m.viewport.SetContent("")
			m.textarea = newInputArea(m.width)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			vpHeight := m.height - 1 - 3 - 2
			if vpHeight < 1 {
				vpHeight = 1
			}
			m.viewport.Height = vpHeight
			m.textarea.SetWidth(m.width)
		}

	case tea.KeyMsg:
		if m.completion.Active {
			cmd := m.handleCompletionKey(msg)
			if cmd != nil {
				return m, cmd
			}
			// If handleCompletionKey returns nil, completion was deactivated — fall through
			if !m.completion.Active {
				var taCmd tea.Cmd
				m.textarea, taCmd = m.textarea.Update(msg)
				cmds = append(cmds, taCmd)
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.state != StateReady {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.messageView.AddPanel(Panel{
					Type:    PanelError,
					Content: "Interrupted by user",
				})
				m.state = StateReady
				m.elapsed = 0
				m.answerBuf = ""
				m.textarea.Focus()
				m.syncViewport()
				return m, textarea.Blink
			}
			return m, tea.Quit

		case tea.KeyEsc:
			if m.state != StateReady {
				if m.agentCancel != nil {
					m.agentCancel()
					m.agentCancel = nil
				}
				m.state = StateReady
				m.elapsed = 0
				m.answerBuf = ""
				m.textarea.Focus()
				m.syncViewport()
				return m, textarea.Blink
			}
			return m, tea.Quit

		case tea.KeyEnter:
			if m.state != StateReady {
				break
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				break
			}
			m.textarea.Reset()

			if strings.HasPrefix(input, "/") {
				cmd := m.handleSlashCommand(input)
				if cmd != nil {
					return m, cmd
				}
				break
			}

			m.messageView.AddPanel(Panel{
				Type:    PanelUser,
				Content: input,
			})
			m.state = StateThinking
			m.startTime = time.Now()
			m.elapsed = 0
			m.answerBuf = ""

			ctx, cancel := context.WithCancel(context.Background())
			m.agentCancel = cancel
			m.agentCh = m.config.Runtime.RunUserInput(ctx, input)

			m.syncViewport()
			return m, tea.Batch(waitForEvent(m.agentCh), tickCmd())
		}

		if m.state == StateReady {
			var taCmd tea.Cmd
			m.textarea, taCmd = m.textarea.Update(msg)
			cmds = append(cmds, taCmd)

			// Check if we should activate completion
			m.checkCompletion()
		}

	case agentEventMsg:
		cmds = append(cmds, m.handleAgentEvent(msg.event))

	case agentDoneMsg:
		if m.agentCancel != nil {
			m.agentCancel()
			m.agentCancel = nil
		}
		m.state = StateReady
		m.answerBuf = ""
		m.textarea.Focus()
		m.syncViewport()
		cmds = append(cmds, textarea.Blink)

	case tickMsg:
		if m.state != StateReady {
			m.elapsed = time.Since(m.startTime)
			cmds = append(cmds, tickCmd())
		}

	case spinner.TickMsg:
		var spinCmd tea.Cmd
		m.spinner, spinCmd = m.spinner.Update(msg)
		cmds = append(cmds, spinCmd)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) checkCompletion() {
	val := m.textarea.Value()
	if strings.HasPrefix(val, "/") && m.state == StateReady {
		prefix := strings.TrimPrefix(val, "/")
		if !m.completion.Active {
			m.completion.Activate()
		}
		m.completion.UpdatePrefix(prefix)
	} else if m.completion.Active {
		m.completion.Deactivate()
	}
}

func (m *Model) handleCompletionKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyUp:
		m.completion.MoveUp()
		return nil // return nil to indicate "handled, stay in completion mode"
	case tea.KeyDown:
		m.completion.MoveDown()
		return nil
	case tea.KeyTab:
		item, ok := m.completion.SelectedItem()
		if ok {
			m.textarea.Reset()
			if item.HasArgs {
				m.textarea.SetValue("/" + item.Name + " ")
			} else {
				m.textarea.SetValue("/" + item.Name)
			}
			m.completion.Deactivate()
		}
		return nil
	case tea.KeyEnter:
		item, ok := m.completion.SelectedItem()
		if ok {
			m.textarea.Reset()
			input := "/" + item.Name
			m.completion.Deactivate()
			if item.HasArgs {
				m.textarea.SetValue(input + " ")
				return nil
			}
			cmd := m.handleSlashCommand(input)
			if cmd != nil {
				return cmd
			}
		}
		return nil
	case tea.KeyEsc:
		m.completion.Deactivate()
		return nil
	default:
		// Let the keystroke pass through to textarea, then re-filter
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		m.checkCompletion()
		if taCmd != nil {
			return taCmd
		}
		return nil
	}
}

func (m *Model) handleAgentEvent(ev agent.AgentEvent) tea.Cmd {
	switch ev.Type {
	case agent.EventDelta:
		if m.answerBuf == "" {
			m.messageView.AddPanel(Panel{
				Type:    PanelAnswer,
				Content: ev.Content,
			})
		} else {
			m.messageView.AppendToLast(ev.Content)
		}
		m.answerBuf += ev.Content

	case agent.EventThought:
		m.state = StateThinking
		m.messageView.AddPanel(Panel{
			Type:    PanelThought,
			Title:   "Thought",
			Content: ev.Content,
		})

	case agent.EventToolCall:
		m.state = StateExecuting
		params := formatParams(ev.Params)
		m.messageView.AddPanel(Panel{
			Type:    PanelAction,
			Title:   ev.ToolName,
			Content: params,
		})

	case agent.EventToolResult:
		m.state = StateThinking
		content := ev.Content
		if len(content) > 500 {
			content = content[:500] + "... [truncated]"
		}
		m.messageView.AddPanel(Panel{
			Type:    PanelObservation,
			Content: content,
		})

	case agent.EventAnswer:
		if m.answerBuf == "" {
			m.messageView.AddPanel(Panel{
				Type:    PanelAnswer,
				Content: ev.Content,
			})
		}

	case agent.EventError:
		m.messageView.AddPanel(Panel{
			Type:    PanelError,
			Content: ev.Content,
		})

	case agent.EventPromptTrace:
		// Trace events are silently stored — viewable via /prompt
	}

	m.syncViewport()
	return waitForEvent(m.agentCh)
}

func (m *Model) handleSlashCommand(input string) tea.Cmd {
	if m.config.Runtime == nil {
		return nil
	}

	result, err := m.config.Runtime.ExecuteCommand(input)
	if err != nil {
		m.messageView.AddPanel(Panel{
			Type:    PanelError,
			Content: err.Error(),
		})
		m.syncViewport()
		return nil
	}

	if result.Message != "" {
		m.messageView.AddPanel(Panel{
			Type:    PanelObservation,
			Content: result.Message,
		})
	}

	switch result.Action {
	case commands.ActionQuit:
		return tea.Quit
	case commands.ActionClearScreen:
		m.messageView.Clear()
	}

	m.syncViewport()
	return nil
}

func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	content := m.messageView.Render(m.width)
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	provider := ""
	model := ""
	if m.config.Runtime != nil {
		provider = m.config.Runtime.CurrentProvider()
		model = m.config.Runtime.CurrentModel()
	}
	statusBar := renderStatusBar(m.width, provider, model, m.state, m.elapsed)

	vpView := m.viewport.View()

	completionView := renderCompletion(m.completion, m.width)

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7C3AED")).
		Width(m.width - 2)
	inputView := borderStyle.Render(m.textarea.View())

	if completionView != "" {
		return statusBar + "\n" + vpView + "\n" + completionView + "\n" + inputView
	}
	return statusBar + "\n" + vpView + "\n" + inputView
}

func waitForEvent(ch <-chan agent.AgentEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg{event: ev}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func formatParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	b, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", params)
	}
	return string(b)
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build ./tui/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add tui/app.go
git commit -m "refactor(tui): use RuntimeInterface, integrate completion panel"
```

---

### Task 12: Add RuntimeInterface adapter to runtime package

**Files:**
- Modify: `runtime/runtime.go` — add `CommandRegistry()` method to satisfy `tui.RuntimeInterface`

- [ ] **Step 1: Add `CommandRegistry()` method to `runtime/runtime.go`**

Add after the `ClearHistory` method:

```go
func (rt *Runtime) CommandRegistry() *commands.Registry {
	return rt.CommandRegistry
}
```

Note: This creates a name collision with the field. Rename the field to `cmdRegistry`:

Replace all occurrences of `rt.CommandRegistry` field access with `rt.cmdRegistry` in `runtime.go`, and expose via a method named `CommandRegistry()`.

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build ./runtime/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add runtime/runtime.go
git commit -m "refactor(runtime): expose CommandRegistry via method for TUI interface"
```

---

### Task 13: Refactor main.go to use Runtime

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Rewrite `main.go`**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fengxuan/go-agent/client"
	"github.com/fengxuan/go-agent/config"
	"github.com/fengxuan/go-agent/runtime"
	"github.com/fengxuan/go-agent/tools"
	"github.com/fengxuan/go-agent/tui"
)

func main() {
	cfgPath := os.Getenv("GO_AGENT_CONFIG")
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		cfgPath = filepath.Join(home, ".go-agent", "config.yaml")
	}

	cfg, err := config.LoadFromFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load config from %s: %v\n", cfgPath, err)
		os.Exit(1)
	}

	config.ApplyEnvOverrides(cfg)

	registry := buildRegistry(cfg)

	rt, err := runtime.New(runtime.Config{
		AppConfig:    cfg,
		ToolRegistry: registry,
		CreateClient: createClient,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	appCfg := tui.AppConfig{
		Runtime: rt,
	}

	model := tui.NewModel(appCfg)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func createClient(provider string, cfg config.ProviderConfig) client.LLMClient {
	if provider == "anthropic" {
		return client.NewAnthropicClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	}
	return client.NewOpenAIClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
}

func buildRegistry(cfg *config.Config) *tools.Registry {
	registry := tools.NewRegistry()
	tavilyBaseURL := "https://api.tavily.com/search"
	registry.Register(tools.NewTavilySearch(cfg.Tools.Tavily.APIKey, tavilyBaseURL))
	registry.Register(tools.NewShellExec(cfg.Tools.Shell.BlockedCommands))
	registry.Register(tools.NewReadFile(cfg.Tools.File.MaxReadSize))
	registry.Register(tools.NewWriteFile())
	return registry
}
```

- [ ] **Step 2: Run full build**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build .`
Expected: no errors

- [ ] **Step 3: Run all tests**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./... -v`
Expected: all tests PASS

- [ ] **Step 4: Run go vet**

Run: `cd /Users/fengxuan/Desktop/go-agent && go vet ./...`
Expected: no issues

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "refactor(main): use Runtime coordinator instead of manual wiring"
```

---

### Task 14: Integration test — create sample skills and verify end-to-end

**Files:**
- No new Go files. Create test skill files and run manual verification.

- [ ] **Step 1: Create user-level skills directory and sample skill**

```bash
mkdir -p ~/.go-agent/skills
```

Create `~/.go-agent/skills/code-review.md`:

```markdown
---
name: code-review
description: "Structured code review skill"
version: "1.0"
tags: [review, quality, golang]
triggers:
  keywords: ["review", "code review", "审查"]
  patterns: ["review this", "help me review"]
priority: 10
tools: [read_file, shell_exec]
---

## Purpose
Provide structured code review feedback.

## When to use
When the user asks for a code review on a file or diff.

## Workflow
1. Read the target file using read_file
2. Analyze code quality, potential bugs, and style issues
3. Run go vet and staticcheck if applicable via shell_exec
4. Provide structured feedback with specific line references

## Output format
- Summary (1-2 sentences)
- Issues found (bulleted list with severity)
- Suggestions (bulleted list)
```

- [ ] **Step 2: Create user-level RULES.md**

Create `~/.go-agent/RULES.md`:

```markdown
# User Rules

- Always respond in the same language the user uses
- Prefer concise answers unless asked for detail
- When writing Go code, always run `go vet` before considering done
```

- [ ] **Step 3: Build and run the agent**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build -o go-agent . && ./go-agent`

Verify:
1. App starts without errors
2. Type `/help` → all 10 commands listed
3. Type `/skills` → shows code-review skill
4. Type `/tools` → shows 4 tools
5. Type `/prompt` → shows "No prompt trace available yet"
6. Type `/` → completion panel appears
7. Type `/sk` → filters to /skills and /skill
8. Press Tab → completes to /skills
9. Press Ctrl+C to exit

- [ ] **Step 4: Commit test fixtures**

```bash
git add -A
git commit -m "test: add sample skill and rules for integration testing"
```

---

### Task 15: Final verification

- [ ] **Step 1: Run all tests**

Run: `cd /Users/fengxuan/Desktop/go-agent && go test ./... -count=1`
Expected: all tests PASS

- [ ] **Step 2: Run go vet**

Run: `cd /Users/fengxuan/Desktop/go-agent && go vet ./...`
Expected: no issues

- [ ] **Step 3: Verify clean build**

Run: `cd /Users/fengxuan/Desktop/go-agent && go build -o /dev/null .`
Expected: no errors
