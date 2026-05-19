package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRulesFile(t *testing.T) {
	dir := t.TempDir()

	content := "# My Rules\n\n- rule one\n"
	if err := os.WriteFile(filepath.Join(dir, "RULES.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write RULES.md: %v", err)
	}

	loader := NewLoader(dir, "")
	got := loader.Load()

	if len(got) != 1 {
		t.Fatalf("expected 1 RuleSet, got %d", len(got))
	}
	if got[0].Source != "user" {
		t.Errorf("expected source %q, got %q", "user", got[0].Source)
	}
	if got[0].Content != content {
		t.Errorf("expected content %q, got %q", content, got[0].Content)
	}
	if got[0].Path != filepath.Join(dir, "RULES.md") {
		t.Errorf("unexpected path %q", got[0].Path)
	}
}

func TestLoadRulesBothLayers(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	userContent := "# User Rules\n"
	projectContent := "# Project Rules\n"

	if err := os.WriteFile(filepath.Join(userDir, "RULES.md"), []byte(userContent), 0644); err != nil {
		t.Fatalf("failed to write user RULES.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "RULES.md"), []byte(projectContent), 0644); err != nil {
		t.Fatalf("failed to write project RULES.md: %v", err)
	}

	loader := NewLoader(userDir, projectDir)
	got := loader.Load()

	if len(got) != 2 {
		t.Fatalf("expected 2 RuleSets, got %d", len(got))
	}

	// User must come first.
	if got[0].Source != "user" {
		t.Errorf("expected first source %q, got %q", "user", got[0].Source)
	}
	if got[0].Content != userContent {
		t.Errorf("unexpected user content: %q", got[0].Content)
	}

	if got[1].Source != "project" {
		t.Errorf("expected second source %q, got %q", "project", got[1].Source)
	}
	if got[1].Content != projectContent {
		t.Errorf("unexpected project content: %q", got[1].Content)
	}
}

func TestLoadRulesNoFile(t *testing.T) {
	// Nonexistent directories — both missing.
	loader := NewLoader("/nonexistent/user/dir", "/nonexistent/project/dir")
	got := loader.Load()

	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(got))
	}
}
