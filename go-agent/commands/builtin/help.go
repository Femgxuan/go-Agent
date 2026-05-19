package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

// HelpCommand lists all available commands.
type HelpCommand struct {
	registry *commands.Registry
}

func NewHelp(registry *commands.Registry) *HelpCommand {
	return &HelpCommand{registry: registry}
}

func (c *HelpCommand) Name() string        { return "help" }
func (c *HelpCommand) Aliases() []string   { return []string{"h"} }
func (c *HelpCommand) Description() string { return "Show available commands" }
func (c *HelpCommand) Usage() string       { return "/help" }

func (c *HelpCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	list := c.registry.List()
	var sb strings.Builder
	sb.WriteString("Available commands:\n")
	for _, info := range list {
		aliases := ""
		if len(info.Aliases) > 0 {
			aliases = fmt.Sprintf(" (aliases: %s)", strings.Join(info.Aliases, ", "))
		}
		sb.WriteString(fmt.Sprintf("  /%s%s — %s\n", info.Name, aliases, info.Description))
	}
	msg := sb.String()
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
