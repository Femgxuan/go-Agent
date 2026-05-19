package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type SkillRunCommand struct {
	name        string
	description string
}

func NewSkillRun(name, description string) *SkillRunCommand {
	return &SkillRunCommand{name: name, description: description}
}

func (c *SkillRunCommand) Name() string        { return c.name }
func (c *SkillRunCommand) Aliases() []string    { return nil }
func (c *SkillRunCommand) Description() string  { return c.description }
func (c *SkillRunCommand) Usage() string        { return fmt.Sprintf("/%s <input>", c.name) }

func (c *SkillRunCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if len(ctx.Args) == 0 {
		return commands.CommandResult{}, fmt.Errorf("usage: /%s <input>", c.name)
	}

	if err := ctx.Runtime.ActivateSkill(c.name); err != nil {
		return commands.CommandResult{}, err
	}

	input := strings.Join(ctx.Args, " ")
	return commands.CommandResult{
		Action: commands.ActionRunAgent,
		Input:  input,
	}, nil
}
