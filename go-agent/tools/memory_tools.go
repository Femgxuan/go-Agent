package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fengxuan/go-agent/memory"
)

// MemoryAppendTool appends content to SOUL.md, MEMORY.md or USER.md.
type MemoryAppendTool struct {
	memoryDir string
}

func NewMemoryAppendTool(memoryDir string) *MemoryAppendTool {
	return &MemoryAppendTool{memoryDir: memoryDir}
}

func (t *MemoryAppendTool) Name() string        { return "memory_append" }
func (t *MemoryAppendTool) Description() string  { return "Append content to SOUL.md, MEMORY.md, or USER.md" }
func (t *MemoryAppendTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target":  map[string]any{"type": "string", "enum": []string{"soul", "memory", "user"}, "description": "Which file to append to"},
			"content": map[string]any{"type": "string", "description": "Content to append"},
		},
		"required": []string{"target", "content"},
	}
}

func (t *MemoryAppendTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	target, _ := params["target"].(string)
	content, _ := params["content"].(string)

	if content == "" {
		return "", fmt.Errorf("content is required")
	}

	// Security scan
	if warnings := memory.ScanForInjection(content); len(warnings) > 0 {
		var msgs []string
		for _, w := range warnings {
			msgs = append(msgs, fmt.Sprintf("[%s] %s (line %d)", w.Type, w.Detail, w.LineNum))
		}
		return "Security warnings detected:\n" + strings.Join(msgs, "\n"), nil
	}

	filePath := t.getFilePath(target)
	if filePath == "" {
		return "", fmt.Errorf("invalid target: %s (use 'soul', 'memory', or 'user')", target)
	}

	// Capacity check
	limit := t.getLimit(target)
	current, _ := os.ReadFile(filePath)
	if limit > 0 && len(current)+len(content) > limit {
		return fmt.Sprintf("Error: content would exceed size limit (%d chars). Current: %d, Adding: %d, Limit: %d",
			limit, len(current), len(content), limit), nil
	}

	// Backup
	backupPath := filePath + ".bak"
	os.WriteFile(backupPath, current, 0o644)

	// Append
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if len(current) > 0 && !strings.HasSuffix(string(current), "\n") {
		f.WriteString("\n")
	}
	f.WriteString(content + "\n")

	return "Content appended. Changes will take effect in next session (frozen snapshot).", nil
}

func (t *MemoryAppendTool) getFilePath(target string) string {
	switch target {
	case "soul":
		return filepath.Join(t.memoryDir, "SOUL.md")
	case "memory":
		return filepath.Join(t.memoryDir, "MEMORY.md")
	case "user":
		return filepath.Join(t.memoryDir, "USER.md")
	default:
		return ""
	}
}

func (t *MemoryAppendTool) getLimit(target string) int {
	switch target {
	case "soul":
		return 0
	case "memory":
		return 2200
	case "user":
		return 1375
	default:
		return 0
	}
}

// MemoryReplaceTool replaces content in SOUL.md, MEMORY.md, or USER.md.
type MemoryReplaceTool struct {
	memoryDir string
}

func NewMemoryReplaceTool(memoryDir string) *MemoryReplaceTool {
	return &MemoryReplaceTool{memoryDir: memoryDir}
}

func (t *MemoryReplaceTool) Name() string        { return "memory_replace" }
func (t *MemoryReplaceTool) Description() string  { return "Replace content in SOUL.md, MEMORY.md, or USER.md" }
func (t *MemoryReplaceTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target":      map[string]any{"type": "string", "enum": []string{"soul", "memory", "user"}},
			"old_content": map[string]any{"type": "string", "description": "Content to find and replace"},
			"new_content": map[string]any{"type": "string", "description": "Replacement content"},
		},
		"required": []string{"target", "old_content", "new_content"},
	}
}

func (t *MemoryReplaceTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	target, _ := params["target"].(string)
	oldContent, _ := params["old_content"].(string)
	newContent, _ := params["new_content"].(string)

	// Security scan
	if warnings := memory.ScanForInjection(newContent); len(warnings) > 0 {
		var msgs []string
		for _, w := range warnings {
			msgs = append(msgs, fmt.Sprintf("[%s] %s", w.Type, w.Detail))
		}
		return "Security warnings:\n" + strings.Join(msgs, "\n"), nil
	}

	filePath := filepath.Join(t.memoryDir, map[string]string{"soul": "SOUL.md", "memory": "MEMORY.md", "user": "USER.md"}[target])
	current, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(current)
	if !strings.Contains(content, oldContent) {
		return "Error: old_content not found in file", nil
	}

	updated := strings.Replace(content, oldContent, newContent, 1)

	// Capacity check
	limit := map[string]int{"soul": 0, "memory": 2200, "user": 1375}[target]
	if limit > 0 && len(updated) > limit {
		return fmt.Sprintf("Error: replacement would exceed limit (%d chars)", limit), nil
	}

	// Backup
	os.WriteFile(filePath+".bak", current, 0o644)

	if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return "Content replaced. Changes will take effect in next session.", nil
}

// MemoryDeleteTool deletes content from SOUL.md, MEMORY.md, or USER.md.
type MemoryDeleteTool struct {
	memoryDir string
}

func NewMemoryDeleteTool(memoryDir string) *MemoryDeleteTool {
	return &MemoryDeleteTool{memoryDir: memoryDir}
}

func (t *MemoryDeleteTool) Name() string        { return "memory_delete" }
func (t *MemoryDeleteTool) Description() string  { return "Delete content from SOUL.md, MEMORY.md, or USER.md" }
func (t *MemoryDeleteTool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target":  map[string]any{"type": "string", "enum": []string{"soul", "memory", "user"}},
			"content": map[string]any{"type": "string", "description": "Content to remove"},
		},
		"required": []string{"target", "content"},
	}
}

func (t *MemoryDeleteTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	target, _ := params["target"].(string)
	content, _ := params["content"].(string)

	filePath := filepath.Join(t.memoryDir, map[string]string{"soul": "SOUL.md", "memory": "MEMORY.md", "user": "USER.md"}[target])
	current, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	old := string(current)
	if !strings.Contains(old, content) {
		return "Error: content not found in file", nil
	}

	updated := strings.Replace(old, content, "", 1)
	updated = strings.TrimSpace(updated) + "\n"

	// Backup
	os.WriteFile(filePath+".bak", current, 0o644)

	if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return "Content deleted. Changes will take effect in next session.", nil
}
