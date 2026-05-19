package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestDraft(name, desc, body string) SkillDraft {
	return SkillDraft{
		Name:        name,
		Description: desc,
		Body:        body,
	}
}

func setupManager(t *testing.T) (*Manager, string, string) {
	t.Helper()
	userDir := filepath.Join(t.TempDir(), "user-skills")
	projectDir := filepath.Join(t.TempDir(), "project-skills")
	idx := NewIndex(nil)
	mgr := NewManager(userDir, projectDir, idx)
	return mgr, userDir, projectDir
}

func TestManager_Create(t *testing.T) {
	mgr, userDir, _ := setupManager(t)

	draft := newTestDraft("My Skill", "A test skill", "Do something useful.")
	skill, err := mgr.Create(draft)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if skill.Name != "My Skill" {
		t.Errorf("expected name %q, got %q", "My Skill", skill.Name)
	}

	// Verify file exists on disk.
	skillFile := filepath.Join(userDir, "my-skill", "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		t.Fatalf("expected file %s to exist", skillFile)
	}

	// Verify it's in the index.
	found, ok := mgr.Get("My Skill")
	if !ok {
		t.Fatal("skill not found in index after create")
	}
	if found.Source != "user" {
		t.Errorf("expected source %q, got %q", "user", found.Source)
	}
}

func TestManager_Create_Validation(t *testing.T) {
	mgr, _, _ := setupManager(t)

	draft := SkillDraft{Name: "", Description: "desc", Body: "body"}
	_, err := mgr.Create(draft)
	if err == nil {
		t.Fatal("expected validation error for empty name")
	}
}

func TestManager_Create_DuplicateSlug(t *testing.T) {
	mgr, userDir, _ := setupManager(t)

	draft1 := newTestDraft("Dup Skill", "First", "Body one.")
	_, err := mgr.Create(draft1)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Create a second skill that would produce the same slug.
	// We need a different name but same slug, so we write it manually
	// to the same slug directory to simulate a conflict.
	// Actually, UniqueSlug checks existing directory names, so creating
	// another draft with the same name should produce slug "dup-skill-2".
	draft2 := newTestDraft("Dup Skill", "Second", "Body two.")
	// The second draft has the same name but UniqueSlug should give it "-2".
	skill2, err := mgr.Create(draft2)
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	// Verify the second skill directory has the -2 suffix.
	dir2 := filepath.Join(userDir, "dup-skill-2")
	if _, err := os.Stat(dir2); os.IsNotExist(err) {
		t.Fatalf("expected directory %s to exist for duplicate slug", dir2)
	}

	_ = skill2
}

func TestManager_Get(t *testing.T) {
	mgr, _, _ := setupManager(t)

	draft := newTestDraft("Getter Skill", "For testing get", "Get me.")
	_, err := mgr.Create(draft)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	skill, ok := mgr.Get("Getter Skill")
	if !ok {
		t.Fatal("expected to find skill")
	}
	if skill.Name != "Getter Skill" {
		t.Errorf("expected name %q, got %q", "Getter Skill", skill.Name)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	mgr, _, _ := setupManager(t)

	_, ok := mgr.Get("Nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestManager_List(t *testing.T) {
	mgr, _, _ := setupManager(t)

	draft1 := newTestDraft("List Skill A", "First skill", "Body A.")
	draft2 := newTestDraft("List Skill B", "Second skill", "Body B.")

	if _, err := mgr.Create(draft1); err != nil {
		t.Fatalf("Create A failed: %v", err)
	}
	if _, err := mgr.Create(draft2); err != nil {
		t.Fatalf("Create B failed: %v", err)
	}

	list := mgr.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(list))
	}
}

func TestManager_Delete(t *testing.T) {
	mgr, userDir, _ := setupManager(t)

	draft := newTestDraft("Delete Me", "To be deleted", "Gone soon.")
	_, err := mgr.Create(draft)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = mgr.Delete("Delete Me")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify removed from index.
	_, ok := mgr.Get("Delete Me")
	if ok {
		t.Fatal("skill should not be in index after delete")
	}

	// Verify removed from disk.
	dir := filepath.Join(userDir, "delete-me")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected directory %s to be removed", dir)
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	mgr, _, _ := setupManager(t)

	err := mgr.Delete("Ghost Skill")
	if err == nil {
		t.Fatal("expected error when deleting nonexistent skill")
	}
}

func TestManager_Update(t *testing.T) {
	mgr, userDir, _ := setupManager(t)

	draft := newTestDraft("Updatable", "Original desc", "Original body.")
	_, err := mgr.Create(draft)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated := newTestDraft("Updatable", "Updated desc", "Updated body content.")
	skill, err := mgr.Update("Updatable", updated)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if skill.Body != "Updated body content." {
		t.Errorf("expected updated body, got %q", skill.Body)
	}

	// Verify on disk.
	data, err := os.ReadFile(filepath.Join(userDir, "updatable", "SKILL.md"))
	if err != nil {
		t.Fatalf("read updated file: %v", err)
	}
	if got := string(data); !contains(got, "Updated body content.") {
		t.Errorf("file content does not contain updated body:\n%s", got)
	}
}

func TestManager_CreateInProject(t *testing.T) {
	mgr, _, projectDir := setupManager(t)

	draft := newTestDraft("Project Skill", "A project skill", "Project body.")
	skill, err := mgr.CreateInProject(draft)
	if err != nil {
		t.Fatalf("CreateInProject failed: %v", err)
	}

	if skill.Source != "project" {
		t.Errorf("expected source %q, got %q", "project", skill.Source)
	}

	// Verify file is in project dir.
	skillFile := filepath.Join(projectDir, "project-skill", "SKILL.md")
	if _, err := os.Stat(skillFile); os.IsNotExist(err) {
		t.Fatalf("expected file %s to exist in project dir", skillFile)
	}
}

func TestManager_Reload(t *testing.T) {
	mgr, userDir, _ := setupManager(t)

	// Manually write a skill file to disk without using Create.
	slug := "manual-skill"
	dir := filepath.Join(userDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := "---\nname: Manual Skill\ndescription: Added manually\n---\n\nManual body.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Before reload, index should not have it.
	_, ok := mgr.Get("Manual Skill")
	if ok {
		t.Fatal("skill should not be in index before reload")
	}

	// Reload and verify.
	if err := mgr.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	skill, ok := mgr.Get("Manual Skill")
	if !ok {
		t.Fatal("skill should be in index after reload")
	}
	if skill.Body != "Manual body." {
		t.Errorf("expected body %q, got %q", "Manual body.", skill.Body)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
