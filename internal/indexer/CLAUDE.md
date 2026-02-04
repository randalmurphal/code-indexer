# indexer package

Pipeline orchestration and file walking for code indexing.

## Purpose

Coordinate the full indexing pipeline: walk files → extract chunks → generate embeddings → store in Qdrant. Handles batching and error collection.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Indexer` | Pipeline coordinator | `indexer.go:19-25` |
| `Walker` | File traversal | `walker.go:12-15` |
| `IndexResult` | Indexing stats | `indexer.go:46-50` |

## Usage

```go
idx, err := indexer.NewIndexer(cfg, voyageAPIKey)
result, err := idx.Index(ctx, "/path/to/repo", repoCfg)
// result.FilesProcessed, result.ChunksCreated, result.Errors
```

## Walker

Traverses directories with glob pattern support:

```go
walker := indexer.NewWalker(
    []string{"**/*.py"},           // includes
    []string{"**/node_modules/**"}, // excludes
)
walker.Walk(root, func(path string) error {
    // called for each matching file
})
```

**Default includes**: `**/*.py`, `**/*.js`, `**/*.ts`, `**/*.tsx`, `**/*.jsx`, `**/*.go`

**Default excludes**: `.git`, `__pycache__`, `node_modules`, `venv`, `.venv`, `dist`, `build`, `.idea`, `.vscode`, minified JS

## Pipeline Stages

| Stage | Batch Size | Description |
|-------|------------|-------------|
| Walk | 1 file | Process files sequentially |
| Extract | 1 file | Parse + chunk extraction |
| Embed | 64 texts | Voyage API batching |
| Store | 100 chunks | Qdrant upsert batching |

## Error Handling

- File errors are collected, not fatal
- Pipeline continues after individual file failures
- `IndexResult.Errors` contains all non-fatal errors

## Module Path Inference

`inferModulePath` converts file paths:
- Input: `fisio/fisio/imports/aws.py`
- Output: `fisio.imports`
- Duplicate prefixes removed automatically

## Gotchas

1. **Go files walked but not parsed** - Walker includes `*.go` but parser doesn't support it yet
2. **Embedding text** - Combines `ContextHeader + Docstring + Content` for better vectors
3. **Collection name** - Hardcoded to `"chunks"`
4. **Batch sizes** - 64 for embeddings (API limit 128), 100 for Qdrant
