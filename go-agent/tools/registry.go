package tools

import (
	"context"

	"github.com/fengxuan/go-agent/client"
)

// Tool is the interface all tools must implement.
type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	Execute(ctx context.Context, params map[string]any) (string, error)
}

// Registry holds registered tools and preserves registration order.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. If a tool with the same name already
// exists it is replaced (order preserved for the first registration).
func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// Get returns the tool with the given name, or false if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// ToolSchemas converts all registered tools to the OpenAI function-calling
// ToolSchema format, preserving registration order.
func (r *Registry) ToolSchemas() []client.ToolSchema {
	schemas := make([]client.ToolSchema, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		schemas = append(schemas, client.ToolSchema{
			Type: "function",
			Function: client.FunctionSchema{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}
	return schemas
}
