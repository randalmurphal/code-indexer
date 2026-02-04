package mcp

import (
	"context"
	"fmt"
)

// StubHandler is a placeholder handler that returns not-implemented errors.
// Replace with search.Handler in Task 2.
type StubHandler struct{}

// NewStubHandler creates a new stub handler.
func NewStubHandler() *StubHandler {
	return &StubHandler{}
}

// ListTools returns an empty tool list.
func (h *StubHandler) ListTools() []Tool {
	return []Tool{
		{
			Name:        "search_code",
			Description: "Search code using semantic similarity (not yet implemented)",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"query": {
						Type:        "string",
						Description: "Natural language search query",
					},
				},
				Required: []string{"query"},
			},
		},
	}
}

// CallTool returns a not-implemented error.
func (h *StubHandler) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	return nil, fmt.Errorf("tool %q not implemented - complete Task 2 to enable search", name)
}

// ListResources returns an empty resource list.
func (h *StubHandler) ListResources() []Resource {
	return []Resource{}
}

// ReadResource returns a not-implemented error.
func (h *StubHandler) ReadResource(ctx context.Context, uri string) (*ReadResourceResult, error) {
	return nil, fmt.Errorf("resource %q not implemented", uri)
}
