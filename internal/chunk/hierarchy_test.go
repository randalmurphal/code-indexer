package chunk

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHierarchicalChunking(t *testing.T) {
	// Simulate a large class with many methods
	var methods []string
	for i := 0; i < 60; i++ {
		letter := string(rune('a' + i%26))
		digit := string(rune('0' + i/26))
		methods = append(methods, `
    def method_`+letter+digit+`(self):
        """Method `+letter+digit+` does something."""
        return "result"`)
	}

	code := `
class LargeClass:
    """A class with many methods."""

    def __init__(self):
        self.value = 0
` + strings.Join(methods, "\n")

	extractor := NewExtractor()
	extractor.SetHierarchicalChunking(true)

	chunks, err := extractor.Extract([]byte(code), "large.py", "test", "test.module")
	require.NoError(t, err)

	// Should have:
	// - 1 class summary chunk
	// - Multiple method chunks with context headers
	assert.True(t, len(chunks) > 50, "should have many chunks")

	// Find class summary chunk
	var summaryChunk *Chunk
	for i := range chunks {
		if chunks[i].Kind == "class_summary" {
			summaryChunk = &chunks[i]
			break
		}
	}
	require.NotNil(t, summaryChunk, "should have class summary")
	assert.Contains(t, summaryChunk.Content, "LargeClass")
	assert.Contains(t, summaryChunk.Content, "Methods:") // Should list methods

	// Check method chunks have context headers
	for _, chunk := range chunks {
		if chunk.Kind == "method" {
			assert.NotEmpty(t, chunk.ContextHeader, "methods should have context header")
			assert.Contains(t, chunk.ContextHeader, "LargeClass")
		}
	}
}

func TestChunkSizeEstimation(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		maxTokens   int
		shouldSplit bool
	}{
		{"short content", "short content", 500, false},
		{"many words", strings.Repeat("word ", 600), 500, true}, // 3000 chars / 4 = 750 tokens
		{"long string", strings.Repeat("x", 2100), 500, true},   // 2100 chars / 4 = 525 tokens > 500
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := Chunk{Content: tt.content}
			tokens := chunk.TokenEstimate()

			if tt.shouldSplit {
				assert.True(t, tokens > tt.maxTokens, "should exceed max tokens, got %d", tokens)
			} else {
				assert.True(t, tokens <= tt.maxTokens, "should fit in max tokens, got %d", tokens)
			}
		})
	}
}

func TestHierarchicalChunkingSmallClass(t *testing.T) {
	// Small class should NOT be split
	code := `
class SmallClass:
    """A small class."""

    def method_a(self):
        return "a"

    def method_b(self):
        return "b"
`

	extractor := NewExtractor()
	extractor.SetHierarchicalChunking(true)

	chunks, err := extractor.Extract([]byte(code), "small.py", "test", "test.module")
	require.NoError(t, err)

	// Should NOT have class_summary for small classes
	for _, chunk := range chunks {
		assert.NotEqual(t, "class_summary", chunk.Kind, "small class should not have summary")
	}
}
