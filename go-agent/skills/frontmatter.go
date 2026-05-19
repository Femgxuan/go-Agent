package skills

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseSkillFile parses a skill markdown file with YAML frontmatter.
func ParseSkillFile(content string) (*Skill, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var skill Skill
	if err := yaml.Unmarshal([]byte(meta), &skill); err != nil {
		return nil, err
	}

	if skill.Name == "" {
		return nil, errors.New("skill frontmatter missing required field: name")
	}

	skill.Body = strings.TrimSpace(body)
	return &skill, nil
}

// splitFrontmatter splits content into frontmatter YAML and body.
// Content must start with "---" and have a closing "---" delimiter.
func splitFrontmatter(content string) (meta, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", errors.New("no frontmatter found: content does not start with ---")
	}

	// Remove the opening ---
	rest := content[3:]

	// Find the closing ---
	idx := strings.Index(rest, "\n---")
	if idx == -1 {
		return "", "", errors.New("no frontmatter found: missing closing ---")
	}

	meta = strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+4:]) // skip "\n---"

	return meta, body, nil
}
