package builtin

import (
	"fmt"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

// ToolsCommand lists all available tools.
type ToolsCommand struct{}

func NewTools() *ToolsCommand { return &ToolsCommand{} }

func (c *ToolsCommand) Name() string        { return "tools" }
func (c *ToolsCommand) Aliases() []string   { return []string{"t"} }
func (c *ToolsCommand) Description() string { return "List all available tools" }
func (c *ToolsCommand) Usage() string       { return "/tools" }

func (c *ToolsCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	tools := ctx.Runtime.ListTools()
	if len(tools) == 0 {
		msg := "No tools available."
		if ctx.Output != nil {
			ctx.Output(msg)
		}
		return commands.CommandResult{Message: msg}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tools (%d):\n", len(tools)))
	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("  %s — %s\n", tool.Name, tool.Description))
	}
	msg := sb.String()
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
