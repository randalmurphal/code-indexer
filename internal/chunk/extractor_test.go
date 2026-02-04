package chunk

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractChunksFromPython(t *testing.T) {
	code := `
"""Module for user management."""

def get_user(user_id: int) -> dict:
    """Fetch user by ID."""
    return {"id": user_id}

class UserService:
    """Service for user operations."""

    def __init__(self, db):
        self.db = db

    def create(self, name: str) -> dict:
        """Create a new user."""
        return self.db.insert({"name": name})
`

	extractor := NewExtractor()
	chunks, err := extractor.Extract([]byte(code), "users.py", "m32rimm", "fisio.common")
	require.NoError(t, err)

	// Should have: get_user, UserService, __init__, create
	require.Len(t, chunks, 4)

	// Check function chunk
	funcChunk := findChunkByName(chunks, "get_user")
	require.NotNil(t, funcChunk)
	assert.Equal(t, ChunkTypeCode, funcChunk.Type)
	assert.Equal(t, "function", funcChunk.Kind)
	assert.Equal(t, "m32rimm", funcChunk.Repo)
	assert.Equal(t, "fisio.common", funcChunk.ModulePath)
	assert.False(t, funcChunk.IsTest)
	assert.Equal(t, float32(1.0), funcChunk.RetrievalWeight)

	// Check method chunk has parent context
	createChunk := findChunkByName(chunks, "create")
	require.NotNil(t, createChunk)
	assert.Equal(t, "method", createChunk.Kind)
	assert.Contains(t, createChunk.ContextHeader, "UserService")
}

func TestExtractChunksFromTest(t *testing.T) {
	code := `
def test_get_user():
    result = get_user(1)
    assert result["id"] == 1
`

	extractor := NewExtractor()
	chunks, err := extractor.Extract([]byte(code), "test_users.py", "m32rimm", "fisio.tests")
	require.NoError(t, err)

	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsTest)
	assert.Equal(t, float32(0.5), chunks[0].RetrievalWeight)
}

func TestIsTestFile(t *testing.T) {
	extractor := NewExtractor()

	tests := []struct {
		filePath string
		isTest   bool
	}{
		{"test_users.py", true},
		{"users_test.py", true},
		{"users_test.go", true},
		{"users.test.js", true},
		{"users.test.ts", true},
		{"users.spec.js", true},
		{"users.spec.ts", true},
		{"/path/to/tests/users.py", true},
		{"/path/to/__tests__/users.js", true},
		{"users.py", false},
		{"users.go", false},
		{"users.js", false},
		{"testing_utils.py", false},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			assert.Equal(t, tt.isTest, extractor.isTestFile(tt.filePath))
		})
	}
}

func TestParseModulePath(t *testing.T) {
	tests := []struct {
		modulePath   string
		expectedRoot string
		expectedSub  string
	}{
		{"fisio.common", "fisio", "common"},
		{"fisio.imports.aws", "fisio", "imports.aws"},
		{"mymodule", "mymodule", ""},
		{"a.b.c.d", "a", "b.c.d"},
	}

	for _, tt := range tests {
		t.Run(tt.modulePath, func(t *testing.T) {
			root, sub := parseModulePath(tt.modulePath)
			assert.Equal(t, tt.expectedRoot, root)
			assert.Equal(t, tt.expectedSub, sub)
		})
	}
}

func TestGenerateChunkID(t *testing.T) {
	// IDs should be deterministic
	id1 := generateChunkID("repo", "file.py", "func", 10)
	id2 := generateChunkID("repo", "file.py", "func", 10)
	assert.Equal(t, id1, id2)

	// Different inputs should produce different IDs
	id3 := generateChunkID("repo", "file.py", "func", 11)
	assert.NotEqual(t, id1, id3)

	// ID should be 16 hex chars (8 bytes)
	assert.Len(t, id1, 16)
}

func TestChunkTokenEstimate(t *testing.T) {
	chunk := Chunk{
		Content: "1234567890123456", // 16 chars
	}
	// ~4 chars per token
	assert.Equal(t, 4, chunk.TokenEstimate())
}

func TestExtractUnsupportedFile(t *testing.T) {
	extractor := NewExtractor()
	_, err := extractor.Extract([]byte(""), "file.txt", "repo", "module")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported file type")
}

func findChunkByName(chunks []Chunk, name string) *Chunk {
	for i := range chunks {
		if chunks[i].SymbolName == name {
			return &chunks[i]
		}
	}
	return nil
}
