package tools

import (
	"context"
	"testing"
)

type mockTool struct {
	name        string
	description string
	schema      map[string]any
}

func (m *mockTool) Name() string              { return m.name }
func (m *mockTool) Description() string       { return m.description }
func (m *mockTool) Schema() map[string]any    { return m.schema }
func (m *mockTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	return "mock result", nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", description: "A test tool"}
	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("expected to find tool 'test_tool', got false")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent tool, got true")
	}
}

func TestRegistryToolSchemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name:        "tool_one",
		description: "First tool",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})
	r.Register(&mockTool{
		name:        "tool_two",
		description: "Second tool",
		schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	})

	schemas := r.ToolSchemas()
	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}

	for _, s := range schemas {
		if s.Type != "function" {
			t.Errorf("expected Type='function', got %q", s.Type)
		}
	}

	if schemas[0].Function.Name != "tool_one" {
		t.Errorf("expected first schema name 'tool_one', got %q", schemas[0].Function.Name)
	}
	if schemas[1].Function.Name != "tool_two" {
		t.Errorf("expected second schema name 'tool_two', got %q", schemas[1].Function.Name)
	}
}
