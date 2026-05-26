package skills

import (
	"errors"
	"strings"

	"gopkg.in/yaml.v3"
)

// goAgentMetadata mirrors the metadata.go-agent block in frontmatter.
type goAgentMetadata struct {
	Category string `yaml:"category"`
}

type skillFrontmatter struct {
	Skill    `yaml:",inline"`
	Metadata struct {
		GoAgent goAgentMetadata `yaml:"go-agent"`
	} `yaml:"metadata"`
}

// ParseSkillFile parses a skill markdown file with YAML frontmatter.
func ParseSkillFile(content string) (*Skill, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(meta), &fm); err != nil {
		return nil, err
	}

	skill := &fm.Skill
	if skill.Name == "" {
		return nil, errors.New("skill frontmatter missing required field: name")
	}

	skill.Category = fm.Metadata.GoAgent.Category
	skill.Body = strings.TrimSpace(body)
	return skill, nil
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
