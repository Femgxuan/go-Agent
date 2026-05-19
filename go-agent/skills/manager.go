package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager provides CRUD operations on skill files.
type Manager struct {
	userDir    string // e.g. ~/.go-agent/skills/
	projectDir string // e.g. ./.go-agent/skills/
	index      *Index
}

// NewManager creates a new Manager.
func NewManager(userDir, projectDir string, index *Index) *Manager {
	return &Manager{
		userDir:    userDir,
		projectDir: projectDir,
		index:      index,
	}
}

// Create creates a new user-level skill from the given draft.
func (m *Manager) Create(draft SkillDraft) (*Skill, error) {
	return m.createIn(draft, m.userDir)
}

// CreateInProject creates a new project-level skill from the given draft.
func (m *Manager) CreateInProject(draft SkillDraft) (*Skill, error) {
	return m.createIn(draft, m.projectDir)
}

func (m *Manager) createIn(draft SkillDraft, dir string) (*Skill, error) {
	if err := draft.Validate(); err != nil {
		return nil, err
	}

	slug := Slugify(draft.Name)

	// Collect existing slugs (directory names) to avoid conflicts.
	existingNames := m.existingSlugs()
	slug = UniqueSlug(draft.Name, existingNames)

	// Create directory and write SKILL.md.
	skillDir := filepath.Join(dir, slug)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return nil, fmt.Errorf("create skill directory: %w", err)
	}

	content := draft.RenderMarkdown()
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write skill file: %w", err)
	}

	// Reload index from disk.
	if err := m.Reload(); err != nil {
		return nil, fmt.Errorf("reload after create: %w", err)
	}

	// Find and return the created skill.
	matches := m.index.MatchByName(draft.Name)
	if len(matches) == 0 {
		return nil, fmt.Errorf("skill %q was written but not found after reload", draft.Name)
	}
	return matches[0], nil
}

// existingSlugs returns the directory/file base names from both user and project dirs.
func (m *Manager) existingSlugs() []string {
	var names []string
	for _, dir := range []string{m.userDir, m.projectDir} {
		entries, err := os.ReadDir(dir)
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
	return names
}

// Get returns a skill by exact name match.
func (m *Manager) Get(name string) (*Skill, bool) {
	matches := m.index.MatchByName(name)
	if len(matches) == 0 {
		return nil, false
	}
	return matches[0], true
}

// List returns all skills in the index.
func (m *Manager) List() []*Skill {
	return m.index.List()
}

// Delete removes a user-level skill by name from disk and reloads the index.
func (m *Manager) Delete(name string) error {
	skill, found := m.Get(name)
	if !found {
		return fmt.Errorf("skill %q not found", name)
	}

	if skill.Source != "user" {
		return fmt.Errorf("cannot delete %s-level skill %q; only user-level skills can be deleted", skill.Source, name)
	}

	if err := os.RemoveAll(skill.BasePath); err != nil {
		return fmt.Errorf("remove skill directory: %w", err)
	}

	return m.Reload()
}

// Update overwrites an existing skill's SKILL.md with new content from draft,
// then reloads the index.
func (m *Manager) Update(name string, draft SkillDraft) (*Skill, error) {
	skill, found := m.Get(name)
	if !found {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	if err := draft.Validate(); err != nil {
		return nil, err
	}

	content := draft.RenderMarkdown()
	skillFile := filepath.Join(skill.BasePath, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("write updated skill file: %w", err)
	}

	if err := m.Reload(); err != nil {
		return nil, fmt.Errorf("reload after update: %w", err)
	}

	matches := m.index.MatchByName(draft.Name)
	if len(matches) == 0 {
		return nil, errors.New("skill not found after update reload")
	}
	return matches[0], nil
}

// Reload reloads all skills from both user and project directories and updates the index.
func (m *Manager) Reload() error {
	skills, err := LoadMultiLayer(m.userDir, m.projectDir)
	if err != nil {
		return fmt.Errorf("reload skills: %w", err)
	}
	m.index.Reload(skills)
	return nil
}
