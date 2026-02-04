# internal/ packages

Core implementation packages for code-indexer. Each package has single responsibility.

## Package Overview

| Package | Responsibility | Key Files |
|---------|---------------|-----------|
| `config` | Config loading | `config.go` |
| `parser` | AST extraction | `parser.go`, `python.go`, `javascript.go` |
| `chunk` | Chunk model + extraction | `chunk.go`, `extractor.go`, `hierarchy.go` |
| `embedding` | Vector generation | `voyage.go` |
| `store` | Qdrant storage | `qdrant.go` |
| `indexer` | Pipeline orchestration | `indexer.go`, `walker.go`, `module.go` |
| `search` | Query handling | `handler.go`, `classifier.go`, `pagination.go` |
| `pattern` | Pattern detection | `detector.go` |
| `security` | Secret redaction | `secrets.go` |
| `sync` | Background daemon | `daemon.go` |
| `cache` | Redis caching | `redis.go` |
| `metrics` | Analytics logging | `logger.go`, `analyzer.go` |
| `mcp` | Protocol types | `types.go`, `server.go` |
| `docs` | Doc parsing | `agents.go` |

## Dependency Graph

```
cmd/code-indexer
    └── indexer
        ├── config
        ├── chunk ← security (redaction)
        │   └── parser
        ├── embedding
        ├── pattern
        ├── docs
        └── store

cmd/code-index-mcp
    └── search
        ├── store
        ├── embedding
        ├── cache
        ├── metrics
        └── mcp
```

## Conventions

- **Error handling**: `fmt.Errorf("operation: %w", err)`
- **Context**: All I/O takes `context.Context` as first param
- **Testing**: Unit tests alongside code, integration tests skip without env
- **Logging**: `slog.Default()` for structured logs
- **Batching**: 64 for embeddings (API limit 128), 100 for Qdrant

## Adding New Languages

1. Add constant in `parser/parser.go:14-19`
2. Create `parser/<lang>.go` with `get<Lang>Language()` and `extract<Lang>Symbols()`
3. Update `DetectLanguage()` in `parser/parser.go:93-104`
4. Add grammar: `go get github.com/smacker/go-tree-sitter/<lang>`

## Adding Storage Backends

1. Create `store/<backend>.go` implementing:
   - `EnsureCollection(ctx, name, dim)`
   - `UpsertChunks(ctx, collection, chunks)`
   - `Search(ctx, collection, vector, limit, filter)`
   - `SearchByFilter(ctx, collection, filter, limit)`
   - `CollectionInfo(ctx, name)`
2. Update `indexer.NewIndexer()` to select based on config
