package commands

import (
	"fmt"
	"strings"
)

type CommandInfo struct {
	Name        string
	Aliases     []string
	Description string
	HasArgs     bool
}

type Registry struct {
	commands map[string]Command
	aliases  map[string]string
	order    []string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
		order:    []string{},
	}
}

func (r *Registry) Register(cmd Command) {
	name := cmd.Name()
	if _, exists := r.commands[name]; !exists {
		r.order = append(r.order, name)
	}
	r.commands[name] = cmd
	for _, alias := range cmd.Aliases() {
		r.aliases[alias] = name
	}
}

func (r *Registry) Get(nameOrAlias string) (Command, bool) {
	if cmd, ok := r.commands[nameOrAlias]; ok {
		return cmd, true
	}
	if canonical, ok := r.aliases[nameOrAlias]; ok {
		if cmd, ok := r.commands[canonical]; ok {
			return cmd, true
		}
	}
	return nil, false
}

func (r *Registry) List() []CommandInfo {
	infos := make([]CommandInfo, 0, len(r.order))
	for _, name := range r.order {
		cmd := r.commands[name]
		infos = append(infos, CommandInfo{
			Name:        cmd.Name(),
			Aliases:     cmd.Aliases(),
			Description: cmd.Description(),
			HasArgs:     strings.Contains(cmd.Usage(), "<"),
		})
	}
	return infos
}

func (r *Registry) Execute(input string, ctx CommandContext) (CommandResult, error) {
	// Strip leading slash
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "/") {
		input = input[1:]
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return CommandResult{}, fmt.Errorf("empty command")
	}

	name := parts[0]
	args := parts[1:]

	cmd, ok := r.Get(name)
	if !ok {
		return CommandResult{}, fmt.Errorf("unknown command: %s", name)
	}

	ctx.Args = args
	return cmd.Execute(ctx)
}
