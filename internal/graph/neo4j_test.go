package graph

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeo4jStore_Integration(t *testing.T) {
	// Skip if no Neo4j available
	neo4jURL := os.Getenv("NEO4J_URL")
	if neo4jURL == "" {
		t.Skip("NEO4J_URL not set, skipping integration test")
	}

	username := os.Getenv("NEO4J_USER")
	if username == "" {
		username = "neo4j"
	}
	password := os.Getenv("NEO4J_PASSWORD")
	if password == "" {
		password = "password"
	}

	ctx := context.Background()

	store, err := NewNeo4jStore(neo4jURL, username, password)
	require.NoError(t, err)
	defer store.Close(ctx)

	// Ensure schema
	err = store.EnsureSchema(ctx)
	require.NoError(t, err)

	// Clean up test data
	_ = store.DeleteRepository(ctx, "test-repo")

	// Test repository operations
	t.Run("UpsertRepository", func(t *testing.T) {
		err := store.UpsertRepository(ctx, Repository{
			Name: "test-repo",
			Path: "/test/repo",
		})
		assert.NoError(t, err)
	})

	// Test module operations
	t.Run("UpsertModule", func(t *testing.T) {
		err := store.UpsertModule(ctx, Module{
			Repo:        "test-repo",
			Path:        "core.utils",
			FSPath:      "core/utils/",
			Description: "Core utilities",
		})
		assert.NoError(t, err)
	})

	// Test file operations
	t.Run("UpsertFile", func(t *testing.T) {
		err := store.UpsertFile(ctx, File{
			Path:        "core/utils/helpers.py",
			Repo:        "test-repo",
			ModuleRoot:  "core",
			Hash:        "abc123",
			LastIndexed: time.Now(),
		})
		assert.NoError(t, err)
	})

	t.Run("GetFileHash", func(t *testing.T) {
		hash, err := store.GetFileHash(ctx, "test-repo", "core/utils/helpers.py")
		assert.NoError(t, err)
		assert.Equal(t, "abc123", hash)
	})

	// Test symbol operations
	t.Run("UpsertSymbol", func(t *testing.T) {
		err := store.UpsertSymbol(ctx, Symbol{
			Name:      "processData",
			Kind:      "function",
			Repo:      "test-repo",
			FilePath:  "core/utils/helpers.py",
			StartLine: 10,
			EndLine:   25,
			Signature: "def processData(data: dict) -> dict",
		})
		assert.NoError(t, err)

		err = store.UpsertSymbol(ctx, Symbol{
			Name:      "validateInput",
			Kind:      "function",
			Repo:      "test-repo",
			FilePath:  "core/utils/helpers.py",
			StartLine: 30,
			EndLine:   45,
			Signature: "def validateInput(input: str) -> bool",
		})
		assert.NoError(t, err)
	})

	t.Run("FindSymbolByName", func(t *testing.T) {
		symbols, err := store.FindSymbolByName(ctx, "test-repo", "processData")
		assert.NoError(t, err)
		assert.Len(t, symbols, 1)
		assert.Equal(t, "function", symbols[0].Kind)
		assert.Equal(t, 10, symbols[0].StartLine)
	})

	// Test call relationships
	t.Run("CreateCallRelationship", func(t *testing.T) {
		caller := Symbol{
			Name:      "processData",
			FilePath:  "core/utils/helpers.py",
			StartLine: 10,
		}
		callee := Symbol{
			Name: "validateInput",
		}
		err := store.CreateCallRelationship(ctx, "test-repo", caller, callee)
		assert.NoError(t, err)
	})

	t.Run("FindCallers", func(t *testing.T) {
		callers, err := store.FindCallers(ctx, "test-repo", "validateInput")
		assert.NoError(t, err)
		assert.Len(t, callers, 1)
		assert.Equal(t, "processData", callers[0].Name)
	})

	t.Run("FindCallees", func(t *testing.T) {
		callees, err := store.FindCallees(ctx, "test-repo", "processData")
		assert.NoError(t, err)
		assert.Len(t, callees, 1)
		assert.Equal(t, "validateInput", callees[0].Name)
	})

	// Test related files
	t.Run("FindRelatedFiles", func(t *testing.T) {
		// Add another file that imports
		err := store.UpsertFile(ctx, File{
			Path:        "core/main.py",
			Repo:        "test-repo",
			ModuleRoot:  "core",
			Hash:        "def456",
			LastIndexed: time.Now(),
		})
		require.NoError(t, err)

		err = store.CreateImportRelationship(ctx, "test-repo", "core/main.py", "core/utils/helpers.py")
		require.NoError(t, err)

		related, err := store.FindRelatedFiles(ctx, "test-repo", "core/utils/helpers.py", 10)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(related), 1)
	})

	// Test GetAllFileHashes
	t.Run("GetAllFileHashes", func(t *testing.T) {
		hashes, err := store.GetAllFileHashes(ctx, "test-repo")
		assert.NoError(t, err)
		assert.Contains(t, hashes, "core/utils/helpers.py")
		assert.Equal(t, "abc123", hashes["core/utils/helpers.py"])
	})

	// Clean up
	t.Run("DeleteRepository", func(t *testing.T) {
		err := store.DeleteRepository(ctx, "test-repo")
		assert.NoError(t, err)
	})
}

func TestNeo4jStore_ConnectionFailure(t *testing.T) {
	ctx := context.Background()
	_, err := NewNeo4jStore("bolt://nonexistent:7687", "user", "pass")
	assert.Error(t, err)
	_ = ctx
}
