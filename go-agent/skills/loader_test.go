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
