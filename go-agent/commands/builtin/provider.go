package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

// ProviderCommand shows or switches the current LLM provider.
type ProviderCommand struct{}

func NewProvider() *ProviderCommand { return &ProviderCommand{} }

func (c *ProviderCommand) Name() string        { return "provider" }
func (c *ProviderCommand) Aliases() []string   { return []string{"p"} }
func (c *ProviderCommand) Description() string { return "Show or switch the current LLM provider" }
func (c *ProviderCommand) Usage() string       { return "/provider [<name>]" }

func (c *ProviderCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if len(ctx.Args) == 0 {
		current := ctx.Runtime.CurrentProvider()
		available := ctx.Runtime.AvailableProviders()
		msg := fmt.Sprintf("Current provider: %s\nAvailable: %s", current, strings.Join(available, ", "))
		if ctx.Output != nil {
			ctx.Output(msg)
		}
		return commands.CommandResult{Message: msg}, nil
	}

	name := ctx.Args[0]
	if err := ctx.Runtime.SwitchProvider(name); err != nil {
		return commands.CommandResult{}, err
	}
	msg := fmt.Sprintf("Switched provider to: %s", name)
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
