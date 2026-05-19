package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ReadFile reads a file from disk and returns its content as a string.
type ReadFile struct {
	maxSize int64
}

// NewReadFile creates a new ReadFile tool.
// maxSize is the maximum allowed file size in bytes.
func NewReadFile(maxSize int64) *ReadFile {
	return &ReadFile{maxSize: maxSize}
}

func (r *ReadFile) Name() string        { return "read_file" }
func (r *ReadFile) Description() string { return "Read the contents of a file." }
func (r *ReadFile) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute path to the file to read.",
			},
		},
		"required": []string{"path"},
	}
}

func (r *ReadFile) Execute(ctx context.Context, params map[string]any) (string, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("read_file: 'path' parameter is required and must be a string")
	}

	if strings.Contains(path, "..") {
		return "", fmt.Errorf("read_file: path traversal detected in %q", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read_file: cannot stat file: %w", err)
	}
	if info.Size() > r.maxSize {
		return "", fmt.Errorf("read_file: file size %d exceeds limit %d", info.Size(), r.maxSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read_file: cannot read file: %w", err)
	}
	return string(data), nil
}
