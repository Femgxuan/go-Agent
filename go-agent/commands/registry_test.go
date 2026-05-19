package commands

import (
	"fmt"
	"testing"
)

// testCommand is a simple Command implementation for testing.
type testCommand struct {
	name        string
	aliases     []string
	description string
	usage       string
	executeFunc func(ctx CommandContext) (CommandResult, error)
}

func (c *testCommand) Name() string        { return c.name }
func (c *testCommand) Aliases() []string   { return c.aliases }
func (c *testCommand) Description() string { return c.description }
func (c *testCommand) Usage() string       { return c.usage }
func (c *testCommand) Execute(ctx CommandContext) (CommandResult, error) {
	if c.executeFunc != nil {
		return c.executeFunc(ctx)
	}
	return CommandResult{Message: "executed " + c.name}, nil
}

func newTestCmd(name string, aliases []string, usage string) *testCommand {
	return &testCommand{
		name:        name,
		aliases:     aliases,
		description: "desc for " + name,
		usage:       usage,
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	cmd := newTestCmd("help", nil, "/help")
	r.Register(cmd)

	got, ok := r.Get("help")
	if !ok {
		t.Fatal("expected to find command 'help'")
	}
	if got.Name() != "help" {
		t.Errorf("expected name 'help', got '%s'", got.Name())
	}
}

func TestRegistryAlias(t *testing.T) {
	r := NewRegistry()
	cmd := newTestCmd("help", []string{"h", "?"}, "/help")
	r.Register(cmd)

	got, ok := r.Get("h")
	if !ok {
		t.Fatal("expected to find command via alias 'h'")
	}
	if got.Name() != "help" {
		t.Errorf("expected name 'help', got '%s'", got.Name())
	}

	got2, ok2 := r.Get("?")
	if !ok2 {
		t.Fatal("expected to find command via alias '?'")
	}
	if got2.Name() != "help" {
		t.Errorf("expected name 'help', got '%s'", got2.Name())
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Register(newTestCmd("alpha", nil, "/alpha"))
	r.Register(newTestCmd("beta", []string{"b"}, "/beta <arg>"))
	r.Register(newTestCmd("gamma", nil, "/gamma"))

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(list))
	}
	if list[0].Name != "alpha" {
		t.Errorf("expected first command 'alpha', got '%s'", list[0].Name)
	}
	if list[1].Name != "beta" {
		t.Errorf("expected second command 'beta', got '%s'", list[1].Name)
	}
	if list[2].Name != "gamma" {
		t.Errorf("expected third command 'gamma', got '%s'", list[2].Name)
	}
	if list[1].HasArgs != true {
		t.Error("expected beta to have args (usage contains '<')")
	}
	if list[0].HasArgs != false {
		t.Error("expected alpha to not have args")
	}
}

func TestRegistryParseAndExecute(t *testing.T) {
	r := NewRegistry()
	var capturedArgs []string
	cmd := &testCommand{
		name:    "help",
		aliases: []string{"h"},
		usage:   "/help",
		executeFunc: func(ctx CommandContext) (CommandResult, error) {
			capturedArgs = ctx.Args
			return CommandResult{Message: "help output"}, nil
		},
	}
	r.Register(cmd)

	result, err := r.Execute("/help", CommandContext{Output: func(s string) {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message != "help output" {
		t.Errorf("expected 'help output', got '%s'", result.Message)
	}
	if len(capturedArgs) != 0 {
		t.Errorf("expected no args, got %v", capturedArgs)
	}
}

func TestRegistryExecuteWithAlias(t *testing.T) {
	r := NewRegistry()
	called := false
	cmd := &testCommand{
		name:    "help",
		aliases: []string{"h"},
		usage:   "/help",
		executeFunc: func(ctx CommandContext) (CommandResult, error) {
			called = true
			return CommandResult{Message: "help via alias"}, nil
		},
	}
	r.Register(cmd)

	result, err := r.Execute("/h", CommandContext{Output: func(s string) {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected command to be called via alias")
	}
	if result.Message != "help via alias" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	r := NewRegistry()

	_, err := r.Execute("/nonexistent", CommandContext{Output: func(s string) {}})
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	expected := fmt.Sprintf("unknown command: nonexistent")
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}
