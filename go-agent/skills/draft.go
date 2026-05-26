package skills

import (
	"errors"
	"fmt"
	"strings"
)

// SkillDraft represents a draft for creating a new skill file.
type SkillDraft struct {
	Name        string
	Description string
	Tags        []string
	Keywords    []string
	Patterns    []string
	Priority    int
	Body        string
	Category    string
}

// Validate checks that required fields are present.
func (d SkillDraft) Validate() error {
	if d.Name == "" {
		return errors.New("skill draft validation: name is required")
	}
	if d.Description == "" {
		return errors.New("skill draft validation: description is required")
	}
	if d.Body == "" {
		return errors.New("skill draft validation: body is required")
	}
	return nil
}

// RenderMarkdown renders the draft as a complete skill Markdown file with YAML frontmatter.
func (d SkillDraft) RenderMarkdown() string {
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", d.Name))
	b.WriteString(fmt.Sprintf("description: %s\n", d.Description))

	if len(d.Tags) > 0 {
		b.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(d.Tags, ", ")))
	}

	hasKeywords := len(d.Keywords) > 0
	hasPatterns := len(d.Patterns) > 0
	if hasKeywords || hasPatterns {
		b.WriteString("triggers:\n")
		if hasKeywords {
			b.WriteString(fmt.Sprintf("  keywords: [%s]\n", strings.Join(d.Keywords, ", ")))
		}
		if hasPatterns {
			b.WriteString(fmt.Sprintf("  patterns: [%s]\n", strings.Join(d.Patterns, ", ")))
		}
	}

	if d.Priority > 0 {
		b.WriteString(fmt.Sprintf("priority: %d\n", d.Priority))
	}

	if d.Category != "" {
		b.WriteString("metadata:\n")
		b.WriteString("  go-agent:\n")
		b.WriteString(fmt.Sprintf("    category: %s\n", d.Category))
	}

	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString(d.Body)
	b.WriteString("\n")

	return b.String()
}

// ToSkill converts the draft to a Skill struct, setting the Source field.
func (d SkillDraft) ToSkill(source string) *Skill {
	return &Skill{
		Name:        d.Name,
		Description: d.Description,
		Tags:        d.Tags,
		Triggers: TriggerConfig{
			Keywords: d.Keywords,
			Patterns: d.Patterns,
		},
		Priority: d.Priority,
		Body:     d.Body,
		Category: d.Category,
		Source:   source,
	}
}
