package embedding

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVoyageEmbed(t *testing.T) {
	apiKey := os.Getenv("VOYAGE_API_KEY")
	if apiKey == "" {
		t.Skip("VOYAGE_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	client := NewVoyageClient(apiKey, "voyage-4-large")

	texts := []string{
		"def hello(): return 'world'",
		"function greet() { return 'hi'; }",
	}

	vectors, err := client.Embed(ctx, texts)
	require.NoError(t, err)

	require.Len(t, vectors, 2)
	assert.Len(t, vectors[0], 1024) // voyage-4-large dimension
	assert.Len(t, vectors[1], 1024)

	// Vectors should be normalized (magnitude ~1)
	magnitude := float32(0)
	for _, v := range vectors[0] {
		magnitude += v * v
	}
	assert.InDelta(t, 1.0, magnitude, 0.01)
}

func TestVoyageEmbedEmpty(t *testing.T) {
	client := NewVoyageClient("dummy-key", "voyage-4-large")

	vectors, err := client.Embed(context.Background(), []string{})
	require.NoError(t, err)
	assert.Nil(t, vectors)
}

func TestVoyageDimension(t *testing.T) {
	tests := []struct {
		model    string
		expected int
	}{
		{"voyage-4-large", 1024},
		{"voyage-3-large", 1024},
		{"voyage-code-3", 1024},
		{"voyage-4", 1024},
		{"voyage-3", 1024},
		{"voyage-4-lite", 512},
		{"voyage-3-lite", 512},
		{"unknown-model", 1024}, // default
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			client := NewVoyageClient("dummy", tt.model)
			assert.Equal(t, tt.expected, client.Dimension())
		})
	}
}
