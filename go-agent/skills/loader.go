package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadDir scans dir for skill files:
// - .md files are treated as single-file skills
// - subdirectories containing SKILL.md are treated as directory skills
// Returns nil slice (not error) for nonexistent directories.
// Entries that fail to parse are silently skipped.
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
		if entry.IsDir() {
			skillMD := filepath.Join(dir, entry.Name(), "SKILL.md")
			if _, statErr := os.Stat(skillMD); statErr == nil {
				skill, loadErr := loadDirSkill(filepath.Join(dir, entry.Name()))
				if loadErr == nil {
					skills = append(skills, skill)
				}
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			skill, loadErr := loadFileSkill(filepath.Join(dir, entry.Name()))
			if loadErr == nil {
				skills = append(skills, skill)
			}
		}
	}

	return skills, nil
}

// loadFileSkill loads and parses a single .md skill file.
func loadFileSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	skill, err := ParseSkillFile(string(data))
	if err != nil {
		return nil, err
	}

	skill.BasePath = filepath.Dir(path)
	return skill, nil
}

// loadDirSkill loads SKILL.md from inside a directory.
func loadDirSkill(dir string) (*Skill, error) {
	skillPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return nil, err
	}

	skill, err := ParseSkillFile(string(data))
	if err != nil {
		return nil, err
	}

	skill.BasePath = dir
	return skill, nil
}

// LoadMultiLayer loads skills from both userDir and projectDir.
// Project-level skills override user-level skills by name.
// Source field is set to "user" or "project".
func LoadMultiLayer(userDir, projectDir string) ([]*Skill, error) {
	userSkills, err := LoadDir(userDir)
	if err != nil {
		return nil, err
	}

	projectSkills, err := LoadDir(projectDir)
	if err != nil {
		return nil, err
	}

	// Build a map for deduplication — project overrides user
	merged := make(map[string]*Skill)

	for _, s := range userSkills {
		s.Source = "user"
		merged[s.Name] = s
	}

	for _, s := range projectSkills {
		s.Source = "project"
		merged[s.Name] = s
	}

	result := make([]*Skill, 0, len(merged))
	for _, s := range merged {
		result = append(result, s)
	}

	return result, nil
}
