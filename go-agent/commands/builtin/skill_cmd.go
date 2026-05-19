package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

// SkillCommand activates a specific skill by name.
type SkillCommand struct{}

func NewSkill() *SkillCommand { return &SkillCommand{} }

func (c *SkillCommand) Name() string        { return "skill" }
func (c *SkillCommand) Aliases() []string   { return []string{"s"} }
func (c *SkillCommand) Description() string { return "Activate a skill by name" }
func (c *SkillCommand) Usage() string       { return "/skill <name>" }

func (c *SkillCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if len(ctx.Args) == 0 {
		return commands.CommandResult{}, fmt.Errorf("skill name required. Usage: %s", c.Usage())
	}

	name := ctx.Args[0]
	if err := ctx.Runtime.ActivateSkill(name); err != nil {
		return commands.CommandResult{}, err
	}
	msg := fmt.Sprintf("Skill '%s' activated.", name)
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
