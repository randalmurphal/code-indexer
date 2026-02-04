package docs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentsMD(t *testing.T) {
	content := `# Fisio

Main ETL engine for data imports.

## Entry Points

- ` + "`fisio/main.py`" + ` - CLI entry point
- ` + "`fisio/imports/command.py`" + ` - Import commands

## Key Patterns

### Import Pattern

All importers inherit from BaseImporter and implement:
- fetch_data()
- transform()
- upsert()

## Important Classes

- ` + "`BOUpserter`" + ` - Business object upserter
- ` + "`RetryManager`" + ` - Handles retries with backoff
`

	doc, err := ParseAgentsMD([]byte(content), "fisio/AGENTS.md", "m32rimm")
	require.NoError(t, err)

	assert.Equal(t, "fisio/AGENTS.md", doc.Path)
	assert.Equal(t, "Fisio", doc.Title)
	assert.Contains(t, doc.Description, "ETL engine")

	// Check entry points
	require.Len(t, doc.EntryPoints, 2)
	assert.Equal(t, "fisio/main.py", doc.EntryPoints[0])
	assert.Equal(t, "fisio/imports/command.py", doc.EntryPoints[1])

	// Check mentioned symbols
	assert.Contains(t, doc.MentionedSymbols, "BOUpserter")
	assert.Contains(t, doc.MentionedSymbols, "RetryManager")

	// Check sections - should have Entry Points, Key Patterns, Import Pattern, Important Classes
	require.GreaterOrEqual(t, len(doc.Sections), 3)
}

func TestParseAgentsMDHeadingPath(t *testing.T) {
	content := `# Root

## Section A

### Subsection A1

Content here.

### Subsection A2

More content.

## Section B

Different section.
`

	doc, err := ParseAgentsMD([]byte(content), "AGENTS.md", "repo")
	require.NoError(t, err)

	// Find the Subsection A1 section
	var foundA1, foundB bool
	for _, s := range doc.Sections {
		if s.Heading == "Subsection A1" {
			foundA1 = true
			assert.Equal(t, "Root > Section A > Subsection A1", s.HeadingPath)
			assert.Equal(t, 3, s.Level)
		}
		if s.Heading == "Section B" {
			foundB = true
			assert.Equal(t, "Root > Section B", s.HeadingPath)
			assert.Equal(t, 2, s.Level)
		}
	}
	assert.True(t, foundA1, "should find Subsection A1")
	assert.True(t, foundB, "should find Section B")
}

func TestParseAgentsMDExtractsFiles(t *testing.T) {
	content := `# Test

See ` + "`path/to/file.py`" + ` and ` + "`another/file.js`" + `.
`

	doc, err := ParseAgentsMD([]byte(content), "AGENTS.md", "repo")
	require.NoError(t, err)

	assert.Contains(t, doc.MentionedFiles, "path/to/file.py")
	assert.Contains(t, doc.MentionedFiles, "another/file.js")
}

func TestToChunks(t *testing.T) {
	content := `# Test Module

Overview.

## Section One

Content one.

## Section Two

Content two.
`

	doc, err := ParseAgentsMD([]byte(content), "module/AGENTS.md", "repo")
	require.NoError(t, err)

	chunks := doc.ToChunks()
	require.GreaterOrEqual(t, len(chunks), 2)

	// Check chunk properties
	for _, c := range chunks {
		assert.Equal(t, "repo", c.Repo)
		assert.Equal(t, "module/AGENTS.md", c.FilePath)
		assert.Equal(t, "doc", string(c.Type))
		assert.Equal(t, "navigation", c.Kind)
		assert.Equal(t, float32(1.5), c.RetrievalWeight)
	}
}

func TestIsFilePath(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"path/to/file.py", true},
		{"file.js", true},
		{"module.ts", true},
		{"handler.go", true},
		{"MyClass", false},
		{"some_function", false},
		{"config", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isFilePath(tt.input))
		})
	}
}

func TestIsSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"MyClass", true},
		{"UserService", true},
		{"some_function", true},
		{"get_user_by_id", true},
		{"path/to/file", false}, // Has slash
		{"file.py", false},      // File extension triggers path detection first
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSymbol(tt.input))
		})
	}
}
