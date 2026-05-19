package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes content to a file on disk.
type WriteFile struct{}

// NewWriteFile creates a new WriteFile tool.
func NewWriteFile() *WriteFile {
	return &WriteFile{}
}

func (w *WriteFile) Name() string        { return "write_file" }
func (w *WriteFile) Description() string { return "Write content to a file." }
func (w *WriteFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file.",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (w *WriteFile) Execute(ctx context.Context, params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("write_file: 'path' parameter is required and must be a string")
	}
	content, ok := params["content"].(string)
	if !ok {
		return "", fmt.Errorf("write_file: 'content' parameter is required and must be a string")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("write_file: failed to create directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write_file: failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}
