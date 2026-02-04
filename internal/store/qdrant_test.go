package store

import (
	"context"
	"os"
	"testing"

	"github.com/randalmurphal/code-indexer/internal/chunk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQdrantStore(t *testing.T) {
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set, skipping integration test")
	}

	ctx := context.Background()
	store, err := NewQdrantStore(os.Getenv("QDRANT_URL"))
	require.NoError(t, err)

	// Clean up test collection
	collectionName := "test_chunks"
	_ = store.DeleteCollection(ctx, collectionName)

	// Create collection
	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	// Insert chunk
	testChunk := chunk.Chunk{
		ID:              "test-001",
		Repo:            "test-repo",
		FilePath:        "test.py",
		StartLine:       1,
		EndLine:         10,
		Type:            chunk.ChunkTypeCode,
		Kind:            "function",
		ModulePath:      "test.module",
		ModuleRoot:      "test",
		Submodule:       "module",
		SymbolName:      "test_func",
		Content:         "def test_func(): pass",
		IsTest:          false,
		RetrievalWeight: 1.0,
		Vector:          make([]float32, 1024), // Zero vector for test
	}

	err = store.UpsertChunks(ctx, collectionName, []chunk.Chunk{testChunk})
	require.NoError(t, err)

	// Search (will return our chunk since it's the only one)
	results, err := store.Search(ctx, collectionName, make([]float32, 1024), 10, nil)
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "test-001", results[0].ID)
	assert.Equal(t, "test_func", results[0].SymbolName)

	// Clean up
	err = store.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}

func TestQdrantStoreWithFilter(t *testing.T) {
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set, skipping integration test")
	}

	ctx := context.Background()
	store, err := NewQdrantStore(os.Getenv("QDRANT_URL"))
	require.NoError(t, err)

	collectionName := "test_filter_chunks"
	_ = store.DeleteCollection(ctx, collectionName)

	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	// Insert multiple chunks
	chunks := []chunk.Chunk{
		{
			ID:              "chunk-001",
			Repo:            "repo-a",
			FilePath:        "main.py",
			Type:            chunk.ChunkTypeCode,
			SymbolName:      "func_a",
			IsTest:          false,
			RetrievalWeight: 1.0,
			Vector:          make([]float32, 1024),
		},
		{
			ID:              "chunk-002",
			Repo:            "repo-b",
			FilePath:        "test_main.py",
			Type:            chunk.ChunkTypeCode,
			SymbolName:      "test_func",
			IsTest:          true,
			RetrievalWeight: 0.5,
			Vector:          make([]float32, 1024),
		},
	}

	err = store.UpsertChunks(ctx, collectionName, chunks)
	require.NoError(t, err)

	// Search with repo filter
	results, err := store.Search(ctx, collectionName, make([]float32, 1024), 10, map[string]interface{}{
		"repo": "repo-a",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "chunk-001", results[0].ID)

	// Search with boolean filter
	results, err = store.Search(ctx, collectionName, make([]float32, 1024), 10, map[string]interface{}{
		"is_test": true,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "chunk-002", results[0].ID)

	// Clean up
	err = store.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}

func TestQdrantStoreCollectionInfo(t *testing.T) {
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set, skipping integration test")
	}

	ctx := context.Background()
	store, err := NewQdrantStore(os.Getenv("QDRANT_URL"))
	require.NoError(t, err)

	collectionName := "test_info_chunks"
	_ = store.DeleteCollection(ctx, collectionName)

	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	info, err := store.CollectionInfo(ctx, collectionName)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.PointsCount)
	assert.Equal(t, 1024, info.VectorSize)

	// Clean up
	err = store.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}

func TestQdrantStoreEnsureCollectionIdempotent(t *testing.T) {
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set, skipping integration test")
	}

	ctx := context.Background()
	store, err := NewQdrantStore(os.Getenv("QDRANT_URL"))
	require.NoError(t, err)

	collectionName := "test_idempotent"
	_ = store.DeleteCollection(ctx, collectionName)

	// Create collection twice - should not error
	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	err = store.EnsureCollection(ctx, collectionName, 1024)
	require.NoError(t, err)

	// Clean up
	err = store.DeleteCollection(ctx, collectionName)
	require.NoError(t, err)
}
