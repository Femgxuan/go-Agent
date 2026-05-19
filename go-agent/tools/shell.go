package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellExec runs shell commands via "sh -c".
type ShellExec struct {
	blockedCommands []string
}

// NewShellExec creates a new ShellExec tool.
// blockedCommands is a list of substrings; if the command contains any of them it is rejected.
func NewShellExec(blockedCommands []string) *ShellExec {
	return &ShellExec{blockedCommands: blockedCommands}
}

func (s *ShellExec) Name() string        { return "shell_exec" }
func (s *ShellExec) Description() string { return "Execute a shell command." }
func (s *ShellExec) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 30).",
			},
		},
		"required": []string{"command"},
	}
}

func (s *ShellExec) Execute(ctx context.Context, params map[string]any) (string, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("shell_exec: 'command' parameter is required and must be a string")
	}

	for _, blocked := range s.blockedCommands {
		if strings.Contains(command, blocked) {
			return "", fmt.Errorf("shell_exec: command contains blocked substring %q", blocked)
		}
	}

	timeout := 30
	if v, ok := params["timeout"]; ok {
		switch n := v.(type) {
		case int:
			timeout = n
		case float64:
			timeout = int(n)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("shell_exec: command failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}
