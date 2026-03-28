package tool

import "context"

type Result struct {
	Content string
	IsError bool
}

type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Run(ctx context.Context, args map[string]interface{}) (Result, error)
}

type Registry struct {
	tools map[string]Tool
	names []string // preserves registration order for deterministic tool definitions sent to the LLM
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	name := t.Name()
	r.tools[name] = t
	r.names = append(r.names, name)
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) Definitions() []map[string]interface{} {
	defs := make([]map[string]interface{}, 0, len(r.tools))
	for _, name := range r.names {
		t := r.tools[name]
		defs = append(defs, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  t.Parameters(),
			},
		})
	}
	return defs
}

// Schema helpers for building OpenAI function parameter definitions.

func ObjectSchema(properties map[string]map[string]interface{}, required ...string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func StringParam(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

func IntParam(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "integer",
		"description": description,
	}
}
