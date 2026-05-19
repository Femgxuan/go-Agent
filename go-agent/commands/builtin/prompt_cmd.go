package builtin

import "github.com/fengxuan/go-agent/commands"

// PromptCommand shows the last prompt trace.
type PromptCommand struct{}

func NewPrompt() *PromptCommand { return &PromptCommand{} }

func (c *PromptCommand) Name() string        { return "prompt" }
func (c *PromptCommand) Aliases() []string   { return []string{"pt"} }
func (c *PromptCommand) Description() string { return "Show the last prompt trace" }
func (c *PromptCommand) Usage() string       { return "/prompt" }

func (c *PromptCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	trace := ctx.Runtime.LastPromptTrace()
	msg := trace
	if msg == "" {
		msg = "No prompt trace available yet."
	}
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
