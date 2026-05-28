package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillStore manages agent skill files stored on the filesystem.
type SkillStore interface {
	// Scan returns Level-0 index (name + description) for all skills.
	Scan(ctx context.Context) ([]SkillIndex, error)

	// Read returns the full SKILL.md content for a skill.
	Read(ctx context.Context, name string) (string, error)

	// ReadFile returns the content of a file within a skill directory.
	ReadFile(ctx context.Context, name string, path string) ([]byte, error)

	// Manage performs CRUD operations on skills.
	Manage(ctx context.Context, action SkillAction, name string, content string) error

	// Search searches skills by keyword.
	Search(ctx context.Context, query string, limit int) ([]SkillIndex, error)
}

// FileSkillStore implements SkillStore using the local filesystem.
type FileSkillStore struct {
	skillsDir string
	maxIndex  int
}

// NewFileSkillStore creates a new filesystem-based skill store.
func NewFileSkillStore(skillsDir string, maxIndex int) *FileSkillStore {
	if maxIndex <= 0 {
		maxIndex = 20
	}
	return &FileSkillStore{skillsDir: skillsDir, maxIndex: maxIndex}
}

// Scan reads all SKILL.md files and returns their frontmatter as SkillIndex entries.
func (s *FileSkillStore) Scan(ctx context.Context) ([]SkillIndex, error) {
	var indices []SkillIndex

	entries, err := os.ReadDir(s.skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		catPath := filepath.Join(s.skillsDir, entry.Name())
		catEntries, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}

		for _, catEntry := range catEntries {
			if !catEntry.IsDir() {
				continue
			}

			skillFile := filepath.Join(catPath, catEntry.Name(), "SKILL.md")
			idx, err := parseSkillFrontmatter(skillFile)
			if err != nil {
				continue
			}
			indices = append(indices, *idx)

			if len(indices) >= s.maxIndex {
				return indices, nil
			}
		}
	}

	return indices, nil
}

// Read returns the full content of a skill's SKILL.md.
func (s *FileSkillStore) Read(ctx context.Context, name string) (string, error) {
	skillPath := s.findSkillPath(name)
	if skillPath == "" {
		return "", fmt.Errorf("skill %q not found", name)
	}

	data, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("read skill: %w", err)
	}

	return string(data), nil
}

// ReadFile reads a file from within a skill directory.
func (s *FileSkillStore) ReadFile(ctx context.Context, name string, path string) ([]byte, error) {
	skillDir := s.findSkillDir(name)
	if skillDir == "" {
		return nil, fmt.Errorf("skill %q not found", name)
	}

	fullPath := filepath.Join(skillDir, path)
	if !strings.HasPrefix(fullPath, skillDir) {
		return nil, fmt.Errorf("path traversal detected")
	}

	return os.ReadFile(fullPath)
}

// Manage performs CRUD on skills.
func (s *FileSkillStore) Manage(ctx context.Context, action SkillAction, name string, content string) error {
	switch action {
	case SkillCreate:
		return s.createSkill(name, content)
	case SkillEdit, SkillPatch:
		return s.editSkill(name, content)
	case SkillDelete:
		return s.deleteSkill(name)
	default:
		return fmt.Errorf("unknown skill action: %s", action)
	}
}

// Search searches skills by keyword (simple substring match on name + description).
func (s *FileSkillStore) Search(ctx context.Context, query string, limit int) ([]SkillIndex, error) {
	all, err := s.Scan(ctx)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var results []SkillIndex
	for _, idx := range all {
		if strings.Contains(strings.ToLower(idx.Name), query) ||
			strings.Contains(strings.ToLower(idx.Description), query) {
			results = append(results, idx)
		}
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GenerateIndex generates the Level-0 skill index string for system prompt injection.
func (s *FileSkillStore) GenerateIndex(ctx context.Context) (string, error) {
	indices, err := s.Scan(ctx)
	if err != nil {
		return "", err
	}
	if len(indices) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Available Skills\n")
	for _, idx := range indices {
		tags := ""
		if len(idx.Tags) > 0 {
			tags = " [" + strings.Join(idx.Tags, ", ") + "]"
		}
		version := ""
		if idx.Version != "" {
			version = " (" + idx.Version + ")"
		}
		sb.WriteString(fmt.Sprintf("- %s: %s%s%s\n", idx.Name, idx.Description, version, tags))
	}
	if len(indices) >= s.maxIndex {
		sb.WriteString(fmt.Sprintf("\n(更多技能请使用 skill_manage 工具搜索)\n"))
	} else {
		sb.WriteString("\n使用 read_skill 工具读取完整技能内容。鼓励在完成复杂任务后创建新技能。\n")
	}

	return sb.String(), nil
}

// findSkillPath finds the SKILL.md path for a skill by name.
func (s *FileSkillStore) findSkillPath(name string) string {
	dir := s.findSkillDir(name)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "SKILL.md")
}

// findSkillDir finds the directory containing a skill by name.
func (s *FileSkillStore) findSkillDir(name string) string {
	entries, err := os.ReadDir(s.skillsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		catPath := filepath.Join(s.skillsDir, entry.Name())
		catEntries, err := os.ReadDir(catPath)
		if err != nil {
			continue
		}
		for _, catEntry := range catEntries {
			if !catEntry.IsDir() {
				continue
			}
			if catEntry.Name() == name {
				return filepath.Join(catPath, catEntry.Name())
			}
		}
	}
	return ""
}

func (s *FileSkillStore) createSkill(name, content string) error {
	skillDir := filepath.Join(s.skillsDir, "custom", name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}
	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
}

func (s *FileSkillStore) editSkill(name, content string) error {
	skillPath := s.findSkillPath(name)
	if skillPath == "" {
		return fmt.Errorf("skill %q not found", name)
	}
	return os.WriteFile(skillPath, []byte(content), 0o644)
}

func (s *FileSkillStore) deleteSkill(name string) error {
	skillDir := s.findSkillDir(name)
	if skillDir == "" {
		return fmt.Errorf("skill %q not found", name)
	}
	return os.RemoveAll(skillDir)
}

// parseSkillFrontmatter parses YAML frontmatter from a SKILL.md file.
func parseSkillFrontmatter(path string) (*SkillIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("no frontmatter")
	}

	parts := strings.SplitN(content[3:], "---", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("unclosed frontmatter")
	}

	var idx SkillIndex
	if err := yaml.Unmarshal([]byte(parts[0]), &idx); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	if idx.Name == "" {
		return nil, fmt.Errorf("missing name in frontmatter")
	}

	return &idx, nil
}
