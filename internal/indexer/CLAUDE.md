# indexer package

Pipeline orchestration and file walking for code indexing.

## Purpose

Coordinate the full indexing pipeline: walk files → extract chunks → generate embeddings → store in Qdrant. Handles batching, error collection, module inference, and navigation doc indexing.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Indexer` | Pipeline coordinator | `indexer.go:26-34` |
| `Walker` | File traversal | `walker.go:12-15` |
| `IndexResult` | Indexing stats | `indexer.go:65-70` |
| `IndexOptions` | Indexing options | `indexer.go:73-76` |
| `ModuleResolver` | Module path resolver | `module.go:10-14` |

## Usage

```go
idx, err := indexer.NewIndexer(cfg, voyageAPIKey)

// Full indexing
result, err := idx.Index(ctx, "/path/to/repo", repoCfg)

// Incremental indexing (only changed files)
result, err := idx.IndexWithOptions(ctx, "/path/to/repo", repoCfg, indexer.IndexOptions{
    Incremental: true,
    GraphStore:  graphStore, // Neo4j for file hashes
})
// result.FilesProcessed, result.FilesSkipped, result.ChunksCreated, result.Errors
```

## Incremental Indexing

Uses SHA-256 file hashes stored in Neo4j to skip unchanged files:

1. Fetch existing hashes: `graphStore.GetAllFileHashes(ctx, repo)`
2. For each file, compute hash with `computeFileHash(content)`
3. Skip if hash matches stored value
4. After indexing, update Neo4j: `graphStore.UpsertFile(ctx, file)`

**CLI**: `code-indexer index <repo> --incremental`

**Requirements**: Neo4j configured with `NEO4J_PASSWORD` env var

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
6. **Incremental requires Neo4j** - Falls back to full index if Neo4j unavailable
7. **Hierarchical chunking enabled** - Large classes (>50 methods) split into summary + method chunks
