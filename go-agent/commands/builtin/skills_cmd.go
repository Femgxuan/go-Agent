package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

// SkillsCommand lists all available skills.
type SkillsCommand struct{}

func NewSkills() *SkillsCommand { return &SkillsCommand{} }

func (c *SkillsCommand) Name() string        { return "skills" }
func (c *SkillsCommand) Aliases() []string   { return []string{"ss"} }
func (c *SkillsCommand) Description() string { return "List all available skills" }
func (c *SkillsCommand) Usage() string       { return "/skills" }

func (c *SkillsCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	skills := ctx.Runtime.ListSkills()
	if len(skills) == 0 {
		msg := "No skills available."
		if ctx.Output != nil {
			ctx.Output(msg)
		}
		return commands.CommandResult{Message: msg}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Skills (%d):\n", len(skills)))
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("  [%d] %s — %s\n", s.Priority, s.Name, s.Description))
		if len(s.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("       tags: %s\n", strings.Join(s.Tags, ", ")))
		}
	}
	msg := sb.String()
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
