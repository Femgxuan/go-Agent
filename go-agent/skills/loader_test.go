package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempSkill(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp skill file: %v", err)
	}
}

const sampleSkillMD = `---
name: test-skill
description: A test skill
version: "1.0"
tags:
  - test
triggers:
  keywords:
    - "hello"
---
This is the skill body.
`

func TestLoadSingleFileSkill(t *testing.T) {
	dir := t.TempDir()
	writeTempSkill(t, dir, "test-skill.md", sampleSkillMD)

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if s.Category != "core" {
		t.Errorf("expected category 'core', got %q", s.Category)
	}
	if s.Body != "This is the skill body." {
		t.Errorf("unexpected body: %q", s.Body)
	}
}

func TestLoadDirectorySkill(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "my-dir-skill")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTempSkill(t, subDir, "SKILL.md", sampleSkillMD)

	skills, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if s.BasePath != subDir {
		t.Errorf("expected BasePath %q, got %q", subDir, s.BasePath)
	}
	if s.Category != "core" {
		t.Errorf("expected category 'core', got %q", s.Category)
	}
}

func TestLoadDirNotExist(t *testing.T) {
	skills, err := LoadDir("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil slice for nonexistent dir, got %v", skills)
	}
}

func TestLoadMultiLayer(t *testing.T) {
	userDir := t.TempDir()
	projectDir := t.TempDir()

	userSkill := `---
name: shared-skill
description: User version
version: "1.0"
---
User body.
`
	projectSkill := `---
name: shared-skill
description: Project version
version: "2.0"
---
Project body.
`

	writeTempSkill(t, userDir, "shared-skill.md", userSkill)
	writeTempSkill(t, projectDir, "shared-skill.md", projectSkill)

	skills, err := LoadMultiLayer(userDir, projectDir)
	if err != nil {
		t.Fatalf("LoadMultiLayer error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 merged skill, got %d", len(skills))
	}

	s := skills[0]
	if s.Source != "project" {
		t.Errorf("expected source 'project', got %q", s.Source)
	}
	if s.Description != "Project version" {
		t.Errorf("expected project description, got %q", s.Description)
	}
}

func TestLoadCategoryDir(t *testing.T) {
	dir := t.TempDir()
	coreDir := filepath.Join(dir, "core", "my-skill")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: cat-skill\ndescription: A categorized skill\nmetadata:\n  go-agent:\n    category: core\n---\nCategorized body.\n"
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
	coreDir := filepath.Join(dir, "code", "my-skill")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: fallback-skill\ndescription: No metadata category\n---\nFallback body.\n"
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
	legacyContent := "---\nname: legacy-skill\ndescription: Old flat skill\n---\nLegacy body.\n"
	writeTempSkill(t, dir, "legacy-skill.md", legacyContent)

	// New category structure
	coreDir := filepath.Join(dir, "core", "categorized")
	if err := os.MkdirAll(coreDir, 0755); err != nil {
		t.Fatal(err)
	}
	catContent := "---\nname: categorized-skill\ndescription: New style\n---\nCat body.\n"
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
