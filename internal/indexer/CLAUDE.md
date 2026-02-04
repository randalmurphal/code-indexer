# indexer package

Pipeline orchestration and file walking for code indexing.

## Purpose

Coordinate the full indexing pipeline: walk files → extract chunks → generate embeddings → store in Qdrant. Handles batching, error collection, module inference, and navigation doc indexing.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Indexer` | Pipeline coordinator | `indexer.go:19-25` |
| `Walker` | File traversal | `walker.go:12-15` |
| `IndexResult` | Indexing stats | `indexer.go:46-50` |
| `ModuleInferrer` | Module path resolver | `module.go:10-14` |

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

`ModuleInferrer` resolves paths using repo config:

```go
inferrer := NewModuleInferrer(repoConfig)
modulePath, moduleRoot, submodule := inferrer.InferModule(filePath)
```

| Input | ModulePath | ModuleRoot | Submodule |
|-------|------------|------------|-----------|
| `fisio/imports/aws.py` | `fisio.imports` | `fisio` | `imports` |
| `src/utils/helpers.py` | `src.utils` | `src` | `utils` |

Config-based: Uses `modules` map in `.ai-devtools.yaml` for descriptions.

## Navigation Doc Indexing

`indexNavigationDocs()` indexes AGENTS.md/CLAUDE.md files:
1. Walker finds `AGENTS.md` and `CLAUDE.md` files
2. Parse with `docs.ParseAgentsMD()`
3. Convert to chunks with `RetrievalWeight: 1.5` (boosted)
4. Include in batch embedding/storage

## Gotchas

1. **Go files walked but not parsed** - Walker includes `*.go` but parser doesn't support it yet
2. **Embedding text** - Combines `ContextHeader + Docstring + Content` for better vectors
3. **Collection name** - Hardcoded to `"chunks"`
4. **Batch sizes** - 64 for embeddings (API limit 128), 100 for Qdrant
5. **Nav docs boosted** - 1.5x retrieval weight ensures docs surface in searches
