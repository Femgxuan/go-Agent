package builtin

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fengxuan/go-agent/commands"
)

type UpdateSkillCommand struct{}

func NewUpdateSkill() *UpdateSkillCommand { return &UpdateSkillCommand{} }

func (c *UpdateSkillCommand) Name() string       { return "update-skill" }
func (c *UpdateSkillCommand) Aliases() []string   { return []string{"us"} }
func (c *UpdateSkillCommand) Description() string { return "Update an existing skill" }
func (c *UpdateSkillCommand) Usage() string {
	return "/update-skill --name=<existing-name> --desc=<description> [--tags=t1,t2] [--keywords=k1,k2] [--priority=N] [--body=<body>]"
}

func (c *UpdateSkillCommand) Execute(ctx commands.CommandContext) (commands.CommandResult, error) {
	flags := parseFlags(ctx.Args)

	name := flags["name"]
	if name == "" {
		return commands.CommandResult{}, fmt.Errorf("missing required --name flag.\nUsage: %s", c.Usage())
	}

	req := commands.CreateSkillRequest{
		Name:        name,
		Description: flags["desc"],
		Body:        flags["body"],
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

	result, err := ctx.Runtime.UpdateSkill(name, req)
	if err != nil {
		return commands.CommandResult{}, err
	}

	return commands.CommandResult{
		Message: result.Message,
	}, nil
}
