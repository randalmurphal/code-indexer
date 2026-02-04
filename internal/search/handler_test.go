// Package search provides the semantic code search handler.
package search

import (
	"context"
	"os"
	"testing"

	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerGetTools(t *testing.T) {
	// Can test without external services
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	tools := handler.ListTools()

	require.Len(t, tools, 1)
	assert.Equal(t, "search_code", tools[0].Name)
	assert.Contains(t, tools[0].Description, "semantic")

	// Verify required params
	assert.Contains(t, tools[0].InputSchema.Required, "query")
}

func TestHandlerListResources(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	resources := handler.ListResources()

	require.Len(t, resources, 1)
	assert.Equal(t, "codeindex://relevant", resources[0].URI)
}

func TestHandlerCallToolUnknown(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	ctx := context.Background()
	_, err := handler.CallTool(ctx, "unknown_tool", nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

func TestHandlerCallToolMissingQuery(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	ctx := context.Background()
	result, err := handler.CallTool(ctx, "search_code", map[string]interface{}{})

	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "query parameter is required")
}

func TestHandlerInferRepo(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	// Test that inferRepo returns empty string when not in a repo dir
	// (since we don't want to test with actual filesystem state)
	repo := handler.inferRepo()
	// Result depends on current working directory, so just verify it doesn't panic
	_ = repo
}

func TestHandlerSearchIntegration(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" || os.Getenv("QDRANT_URL") == "" {
		t.Skip("Integration test requires VOYAGE_API_KEY and QDRANT_URL")
	}

	cfg := config.DefaultConfig()
	cfg.Storage.QdrantURL = os.Getenv("QDRANT_URL")

	handler, err := NewHandler(cfg, os.Getenv("VOYAGE_API_KEY"), nil)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := handler.CallTool(ctx, "search_code", map[string]interface{}{
		"query": "hello world",
		"limit": float64(5),
	})
	require.NoError(t, err)

	assert.False(t, result.IsError)
	assert.NotEmpty(t, result.Content)
}

func TestFormatEmptyResponse(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := &Handler{config: cfg}

	response := handler.formatEmptyResponse("test query", "my-repo")

	assert.Contains(t, response, "No direct matches")
	assert.Contains(t, response, "test query")
	assert.Contains(t, response, "my-repo")
}
