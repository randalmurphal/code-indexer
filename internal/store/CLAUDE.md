# store package

Qdrant vector storage for chunk persistence and search.

## Purpose

Store and retrieve code chunks in Qdrant vector database. Handles collection management, upserts, and similarity search with filtering.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `QdrantStore` | Qdrant client wrapper | `qdrant.go:14-16` |
| `CollectionInfo` | Collection metadata | `qdrant.go:120-124` |

## Usage

```go
store, err := store.NewQdrantStore("localhost:6334")
err = store.EnsureCollection(ctx, "chunks", 1024)
err = store.UpsertChunks(ctx, "chunks", chunks)
results, err := store.Search(ctx, "chunks", queryVector, 10, nil)
```

## Methods

| Method | Description |
|--------|-------------|
| `NewQdrantStore(url)` | Create client (gRPC) |
| `EnsureCollection(ctx, name, dim)` | Create if not exists |
| `DeleteCollection(ctx, name)` | Remove collection |
| `UpsertChunks(ctx, coll, chunks)` | Insert/update chunks |
| `Search(ctx, coll, vec, limit, filter)` | Vector similarity search |
| `SearchByFilter(ctx, coll, filter, limit)` | Filter-only search (no vector) |
| `CollectionInfo(ctx, name)` | Get collection stats |

## Payload Fields

All `Chunk` fields stored as Qdrant payload:

| Field | Qdrant Type |
|-------|-------------|
| `repo`, `file_path`, `kind` | keyword |
| `start_line`, `end_line` | integer |
| `is_test`, `has_secrets` | bool |
| `retrieval_weight` | double |
| `content`, `docstring` | text |

## Filtering

Search supports string and boolean filters:
```go
filter := map[string]interface{}{
    "repo":    "my-repo",
    "is_test": false,
}
results, err := store.Search(ctx, "chunks", vec, 10, filter)
```

## Connection

| Protocol | Port | Usage |
|----------|------|-------|
| gRPC | 6334 | Go client (this package) |
| REST | 6333 | Debug/admin |

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `QDRANT_URL` | `localhost:6334` | Qdrant gRPC endpoint |

## SearchByFilter

Non-vector search using payload filters only:
```go
filter := map[string]interface{}{
    "kind": "pattern",
    "repo": "my-repo",
}
results, err := store.SearchByFilter(ctx, "chunks", filter, 100)
```

Used by pattern detection queries that don't need semantic matching.

## Gotchas

1. **URL format**: Use `localhost:6334` not `http://localhost:6333`
2. **Vectors cleared on search** - Results have `Vector: nil` to save memory
3. **EnsureCollection is idempotent** - Safe to call multiple times
4. **Cosine distance** - Collection uses cosine similarity by default
5. **SearchByFilter** - No scoring, returns by internal ID order
