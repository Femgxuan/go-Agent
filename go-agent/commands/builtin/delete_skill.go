package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

type DeleteSkillCommand struct{}

func NewDeleteSkill() *DeleteSkillCommand { return &DeleteSkillCommand{} }

func (c *DeleteSkillCommand) Name() string       { return "delete-skill" }
func (c *DeleteSkillCommand) Aliases() []string   { return []string{"ds"} }
func (c *DeleteSkillCommand) Description() string { return "Delete an existing skill" }
func (c *DeleteSkillCommand) Usage() string       { return "/delete-skill <name>" }

func (c *DeleteSkillCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if len(ctx.Args) == 0 {
		return commands.CommandResult{}, fmt.Errorf("missing skill name.\nUsage: %s", c.Usage())
	}

	name := ctx.Args[0]
	if err := ctx.Runtime.DeleteSkill(name); err != nil {
		return commands.CommandResult{}, err
	}

	return commands.CommandResult{
		Message: fmt.Sprintf("Skill '%s' deleted successfully.", name),
	}, nil
}
