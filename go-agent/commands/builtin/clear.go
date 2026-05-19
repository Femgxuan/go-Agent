package builtin

import "github.com/fengxuan/go-agent/commands"

// ClearCommand clears the conversation history and screen.
type ClearCommand struct{}

func NewClear() *ClearCommand { return &ClearCommand{} }

func (c *ClearCommand) Name() string        { return "clear" }
func (c *ClearCommand) Aliases() []string   { return []string{"cls"} }
func (c *ClearCommand) Description() string { return "Clear conversation history and screen" }
func (c *ClearCommand) Usage() string       { return "/clear" }

func (c *ClearCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if ctx.Runtime != nil {
		ctx.Runtime.ClearHistory()
	}
	return commands.CommandResult{
		Message: "History cleared.",
		Action:  commands.ActionClearScreen,
	}, nil
}
