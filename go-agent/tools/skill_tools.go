package tools

import (
	"context"
	"fmt"

	"github.com/fengxuan/go-agent/memory"
)

// ReadSkillTool reads a skill's SKILL.md content.
type ReadSkillTool struct {
	skillStore memory.SkillStore
}

func NewReadSkillTool(store memory.SkillStore) *ReadSkillTool {
	return &ReadSkillTool{skillStore: store}
}

func (t *ReadSkillTool) Name() string        { return "read_skill" }
func (t *ReadSkillTool) Description() string  { return "Read the full content of a skill" }
func (t *ReadSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]string{"type": "string", "description": "Skill name"},
		},
		"required": []string{"name"},
	}
}

func (t *ReadSkillTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}

	content, err := t.skillStore.Read(ctx, name)
	if err != nil {
		return "", fmt.Errorf("read skill: %w", err)
	}

	return content, nil
}

// SkillManageTool performs CRUD operations on skills.
type SkillManageTool struct {
	skillStore memory.SkillStore
}

func NewSkillManageTool(store memory.SkillStore) *SkillManageTool {
	return &SkillManageTool{skillStore: store}
}

func (t *SkillManageTool) Name() string        { return "skill_manage" }
func (t *SkillManageTool) Description() string  { return "Create, edit, patch, or delete skills" }
func (t *SkillManageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":  map[string]string{"type": "string", "enum": `["create", "patch", "edit", "delete"]`, "description": "Action to perform"},
			"name":    map[string]string{"type": "string", "description": "Skill name"},
			"content": map[string]string{"type": "string", "description": "Skill content (for create/edit/patch)"},
		},
		"required": []string{"action", "name"},
	}
}

func (t *SkillManageTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	action, _ := params["action"].(string)
	name, _ := params["name"].(string)
	content, _ := params["content"].(string)

	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}

	skillAction := memory.SkillAction(action)
	if err := t.skillStore.Manage(ctx, skillAction, name, content); err != nil {
		return "", fmt.Errorf("skill manage: %w", err)
	}

	return fmt.Sprintf("Skill %q %sed successfully.", name, action), nil
}
