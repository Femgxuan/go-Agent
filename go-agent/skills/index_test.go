package skills

import (
	"testing"
)

func makeTestSkills() []*Skill {
	return []*Skill{
		{
			Name:        "log-analyzer",
			Description: "Analyze application logs for errors",
			Tags:        []string{"logs", "monitoring"},
			Priority:    5,
			Triggers: TriggerConfig{
				Keywords: []string{"查日志", "查看日志"},
				Patterns: []string{`log\s+query`},
			},
		},
		{
			Name:        "code-reviewer",
			Description: "Review code changes and suggest improvements",
			Tags:        []string{"code", "review"},
			Priority:    10,
			Triggers: TriggerConfig{
				Keywords: []string{"review", "code review"},
				Patterns: []string{`review\s+pr`},
			},
		},
		{
			Name:        "deploy-helper",
			Description: "Help with deployment and release processes",
			Tags:        []string{"deploy", "release"},
			Priority:    3,
			Triggers: TriggerConfig{
				Keywords: []string{"deploy", "release"},
				Patterns: []string{},
			},
		},
	}
}

func TestIndexMatchByName(t *testing.T) {
	idx := NewIndex(makeTestSkills())

	results := idx.MatchByName("log-analyzer")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "log-analyzer" {
		t.Errorf("expected 'log-analyzer', got %q", results[0].Name)
	}

	results = idx.MatchByName("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results for nonexistent name, got %d", len(results))
	}
}

func TestIndexMatchByKeyword(t *testing.T) {
	idx := NewIndex(makeTestSkills())

	results := idx.Match("我想查日志")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for keyword match")
	}
	if results[0].Name != "log-analyzer" {
		t.Errorf("expected 'log-analyzer' as top result, got %q", results[0].Name)
	}
}

func TestIndexMatchByPattern(t *testing.T) {
	idx := NewIndex(makeTestSkills())

	results := idx.Match("log query for errors")
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for pattern match")
	}
	if results[0].Name != "log-analyzer" {
		t.Errorf("expected 'log-analyzer' as top result, got %q", results[0].Name)
	}
}

func TestIndexMatchSortedByPriority(t *testing.T) {
	// Both "code-reviewer" (priority 10) and "deploy-helper" (priority 3)
	// should match "deploy review" but code-reviewer has higher priority.
	skills := []*Skill{
		{
			Name:        "skill-low",
			Description: "deployment helper tool",
			Priority:    1,
			Tags:        []string{"deploy"},
			Triggers:    TriggerConfig{Keywords: []string{"deploy"}},
		},
		{
			Name:        "skill-high",
			Description: "deployment management tool",
			Priority:    20,
			Tags:        []string{"deploy"},
			Triggers:    TriggerConfig{Keywords: []string{"deploy"}},
		},
	}
	idx := NewIndex(skills)

	results := idx.Match("deploy")
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "skill-high" {
		t.Errorf("expected 'skill-high' first (higher priority), got %q", results[0].Name)
	}
}

func TestIndexMatchMaxResults(t *testing.T) {
	idx := NewIndex(makeTestSkills())
	idx.MaxResults = 2

	// "deploy" will match deploy-helper by keyword,
	// but we also add a broad query that could match multiple skills
	skills := []*Skill{
		{Name: "s1", Description: "foo bar", Priority: 1, Triggers: TriggerConfig{Keywords: []string{"test"}}},
		{Name: "s2", Description: "foo baz", Priority: 2, Triggers: TriggerConfig{Keywords: []string{"test"}}},
		{Name: "s3", Description: "foo qux", Priority: 3, Triggers: TriggerConfig{Keywords: []string{"test"}}},
	}
	idx2 := NewIndex(skills)
	idx2.MaxResults = 2

	results := idx2.Match("test")
	if len(results) > 2 {
		t.Errorf("expected at most 2 results (MaxResults=2), got %d", len(results))
	}
}

func TestIndexList(t *testing.T) {
	testSkills := makeTestSkills()
	idx := NewIndex(testSkills)

	all := idx.List()
	if len(all) != len(testSkills) {
		t.Errorf("expected %d skills, got %d", len(testSkills), len(all))
	}

	// Verify it's a copy (modifying returned slice doesn't affect index)
	all[0] = nil
	if idx.skills[0] == nil {
		t.Error("List() should return a copy, not a reference to internal slice")
	}
}
