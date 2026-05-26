package builtin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type CreateSkillCommand struct{}

func NewCreateSkill() *CreateSkillCommand { return &CreateSkillCommand{} }

func (c *CreateSkillCommand) Name() string       { return "create-skill" }
func (c *CreateSkillCommand) Aliases() []string   { return []string{"cs"} }
func (c *CreateSkillCommand) Description() string { return "Create a new skill from scratch" }
func (c *CreateSkillCommand) Usage() string {
	return "/create-skill --name=<name> --desc=<description> [--tags=t1,t2] [--keywords=k1,k2] [--priority=N] --body=<body>"
}

func (c *CreateSkillCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	flags := parseFlags(ctx.Args)

	name := flags["name"]
	desc := flags["desc"]
	body := flags["body"]

	if name == "" || desc == "" || body == "" {
		return commands.CommandResult{}, fmt.Errorf("missing required flags.\nUsage: %s", c.Usage())
	}

	// Note: if --body value starts with @, future versions will read from file path.
	// For phase 1, the value is used as-is.

	req := commands.CreateSkillRequest{
		Name:        name,
		Description: desc,
		Body:        body,
	}

	if tags := flags["tags"]; tags != "" {
		req.Tags = strings.Split(tags, ",")
	}
	if keywords := flags["keywords"]; keywords != "" {
		req.Keywords = strings.Split(keywords, ",")
	}
	if p := flags["priority"]; p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			req.Priority = n
		}
	}
	if cat := flags["category"]; cat != "" {
		req.Category = cat
	}

	result, err := ctx.Runtime.CreateSkill(req)
	if err != nil {
		return commands.CommandResult{}, err
	}

	return commands.CommandResult{
		Message: result.Message,
	}, nil
}

// parseFlags parses --key=value arguments into a map.
func parseFlags(args []string) map[string]string {
	flags := make(map[string]string)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		arg = strings.TrimPrefix(arg, "--")
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			flags[parts[0]] = parts[1]
		}
	}
	return flags
}
