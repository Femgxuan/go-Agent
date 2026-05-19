package prompt

import (
	"strings"
	"testing"
)

func TestBuilderAssemblyOrder(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "sys", Type: TypeSystem, Content: "system content"})
	b.AddSource(Source{Name: "rule1", Type: TypeRules, Content: "rules content"})
	b.AddSource(Source{Name: "skill1", Type: TypeSkill, Content: "skill content"})
	b.AddSource(Source{Name: "user1", Type: TypeUser, Content: "user content"})

	result, trace := b.Build()

	// Verify output order
	parts := strings.Split(result, "\n\n")
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	if parts[0] != "system content" {
		t.Errorf("expected parts[0]='system content', got %q", parts[0])
	}
	if parts[1] != "rules content" {
		t.Errorf("expected parts[1]='rules content', got %q", parts[1])
	}
	if parts[2] != "skill content" {
		t.Errorf("expected parts[2]='skill content', got %q", parts[2])
	}
	if parts[3] != "user content" {
		t.Errorf("expected parts[3]='user content', got %q", parts[3])
	}

	// Verify trace has 4 sources
	if len(trace.Sources) != 4 {
		t.Errorf("expected 4 trace sources, got %d", len(trace.Sources))
	}
	if len(trace.RulesLoaded) != 1 || trace.RulesLoaded[0] != "rule1" {
		t.Errorf("expected RulesLoaded=[rule1], got %v", trace.RulesLoaded)
	}
	if len(trace.SkillsMatched) != 1 || trace.SkillsMatched[0] != "skill1" {
		t.Errorf("expected SkillsMatched=[skill1], got %v", trace.SkillsMatched)
	}
}

func TestBuilderEmpty(t *testing.T) {
	b := NewBuilder()
	result, trace := b.Build()

	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if len(trace.Sources) != 0 {
		t.Errorf("expected 0 trace sources, got %d", len(trace.Sources))
	}
	if trace.TotalChars != 0 {
		t.Errorf("expected TotalChars=0, got %d", trace.TotalChars)
	}
}

func TestBuilderReset(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "sys", Type: TypeSystem, Content: "system content"})
	b.Reset()
	result, trace := b.Build()

	if result != "" {
		t.Errorf("expected empty result after reset, got %q", result)
	}
	if len(trace.Sources) != 0 {
		t.Errorf("expected 0 sources after reset, got %d", len(trace.Sources))
	}
}

func TestBuilderTraceSourceNames(t *testing.T) {
	b := NewBuilder()
	b.AddSource(Source{Name: "alpha", Type: TypeSystem, Content: "alpha content"})
	b.AddSource(Source{Name: "beta", Type: TypeRules, Content: "beta content"})
	b.AddSource(Source{Name: "empty-source", Type: TypeUser, Content: ""}) // should be skipped
	b.AddSource(Source{Name: "gamma", Type: TypeSkill, Content: "gamma content"})

	_, trace := b.Build()

	expected := []string{"alpha", "beta", "gamma"}
	if len(trace.Sources) != len(expected) {
		t.Fatalf("expected %d sources, got %d: %v", len(expected), len(trace.Sources), trace.Sources)
	}
	for i, name := range expected {
		if trace.Sources[i] != name {
			t.Errorf("trace.Sources[%d]: expected %q, got %q", i, name, trace.Sources[i])
		}
	}
}
