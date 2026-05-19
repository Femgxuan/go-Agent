package builtin

import (
	"fmt"

	"github.com/fengxuan/go-agent/commands"
)

// ModelCommand shows or switches the current model.
type ModelCommand struct{}

func NewModel() *ModelCommand { return &ModelCommand{} }

func (c *ModelCommand) Name() string        { return "model" }
func (c *ModelCommand) Aliases() []string   { return []string{"m"} }
func (c *ModelCommand) Description() string { return "Show or switch the current model" }
func (c *ModelCommand) Usage() string       { return "/model [<name>]" }

func (c *ModelCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if len(ctx.Args) == 0 {
		current := ctx.Runtime.CurrentModel()
		msg := fmt.Sprintf("Current model: %s", current)
		if ctx.Output != nil {
			ctx.Output(msg)
		}
		return commands.CommandResult{Message: msg}, nil
	}

	name := ctx.Args[0]
	if err := ctx.Runtime.SwitchModel(name); err != nil {
		return commands.CommandResult{}, err
	}
	msg := fmt.Sprintf("Switched model to: %s", name)
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
