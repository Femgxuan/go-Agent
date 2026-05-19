package builtin

import "github.com/fengxuan/go-agent/commands"

// ReloadCommand reloads skills and rules from disk.
type ReloadCommand struct{}

func NewReload() *ReloadCommand { return &ReloadCommand{} }

func (c *ReloadCommand) Name() string        { return "reload" }
func (c *ReloadCommand) Aliases() []string   { return []string{"r"} }
func (c *ReloadCommand) Description() string { return "Reload skills and rules from disk" }
func (c *ReloadCommand) Usage() string       { return "/reload" }

func (c *ReloadCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	if err := ctx.Runtime.ReloadSkillsAndRules(); err != nil {
		return commands.CommandResult{}, err
	}
	msg := "Skills and rules reloaded."
	if ctx.Output != nil {
		ctx.Output(msg)
	}
	return commands.CommandResult{Message: msg}, nil
}
