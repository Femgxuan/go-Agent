package builtin

import "github.com/fengxuan/go-agent/commands"

// QuitCommand exits the application.
type QuitCommand struct{}

func NewQuit() *QuitCommand { return &QuitCommand{} }

func (c *QuitCommand) Name() string        { return "quit" }
func (c *QuitCommand) Aliases() []string   { return []string{"q", "exit"} }
func (c *QuitCommand) Description() string { return "Quit the application" }
func (c *QuitCommand) Usage() string       { return "/quit" }

func (c *QuitCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	return commands.CommandResult{
		Message: "Goodbye!",
		Action:  commands.ActionQuit,
	}, nil
}
