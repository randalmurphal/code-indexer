# internal/ packages

Core implementation packages for code-indexer. Each package has a single responsibility.

## Package Overview

| Package | Responsibility | Key File |
|---------|---------------|----------|
| `config` | Config loading | `config.go` |
| `parser` | AST extraction | `parser.go`, `python.go`, `javascript.go` |
| `chunk` | Chunk model | `chunk.go`, `extractor.go` |
| `embedding` | Vector generation | `voyage.go` |
| `store` | Qdrant storage | `qdrant.go` |
| `indexer` | Pipeline orchestration | `indexer.go`, `walker.go` |

## Dependency Graph

```
cmd/code-indexer
    └── indexer
        ├── config
        ├── chunk
        │   └── parser
        ├── embedding
        └── store
            └── chunk (types only)
```

## Conventions

- **Error handling**: Wrap errors with context using `fmt.Errorf("operation: %w", err)`
- **Context**: All I/O operations accept `context.Context` as first parameter
- **Testing**: Unit tests alongside code, integration tests skip without env vars
- **Logging**: Use `slog.Default()` for structured logging

## Adding New Languages

1. Add language constant in `parser/parser.go`
2. Create `parser/<lang>.go` with `get<Lang>Language()` and `extract<Lang>Symbols()`
3. Update `DetectLanguage()` with file extensions
4. Add tree-sitter grammar: `go get github.com/smacker/go-tree-sitter/<lang>`

## Adding New Storage Backends

1. Create `store/<backend>.go` implementing same interface as `QdrantStore`
2. Methods needed: `EnsureCollection`, `UpsertChunks`, `Search`, `CollectionInfo`
3. Update `indexer.NewIndexer()` to select backend based on config
