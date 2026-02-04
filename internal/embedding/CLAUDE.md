# embedding package

Voyage AI embedding client for vector generation.

## Purpose

Generate embeddings via Voyage AI API with batching support. Used to convert code chunks into vectors for semantic search.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `VoyageClient` | API client | `voyage.go:17-21` |

## Usage

```go
client := embedding.NewVoyageClient(apiKey, "voyage-4-large")
vectors, err := client.Embed(ctx, []string{"def foo(): pass"})
// vectors[0] is 1024-dimensional float32 slice
```

## Methods

| Method | Description |
|--------|-------------|
| `Embed(ctx, texts)` | Generate embeddings for texts |
| `EmbedBatched(ctx, texts, batchSize)` | Batch large inputs (default: 128) |
| `Dimension()` | Vector dimension for model |

## Model Dimensions

| Model | Dimensions |
|-------|------------|
| `voyage-4-large` | 1024 |
| `voyage-code-3` | 1024 |
| `voyage-4-lite` | 512 |
| `voyage-3-lite` | 512 |

## API Details

- **Endpoint**: `https://api.voyageai.com/v1/embeddings`
- **Auth**: Bearer token via `Authorization` header
- **Input type**: `document` (optimized for retrieval)
- **Timeout**: 60 seconds

## Batching

Voyage API has a max batch size of 128. `EmbedBatched` handles this:
```go
vectors, err := client.EmbedBatched(ctx, largeTexts, 64)
// Splits into batches of 64, concatenates results
```

## Environment

| Variable | Description |
|----------|-------------|
| `VOYAGE_API_KEY` | Required for embedding generation |

## Gotchas

1. **API key in header** - Never logged or exposed in errors
2. **Index ordering** - Response preserves input order via `Index` field
3. **Empty input** - Returns `nil, nil` (not an error)
4. **Normalized vectors** - Magnitude ≈ 1.0 (cosine similarity ready)
