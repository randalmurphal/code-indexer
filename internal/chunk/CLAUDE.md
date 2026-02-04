# chunk package

Chunk model and extraction for indexable code units.

## Purpose

Transform parsed symbols into indexable chunks with metadata for semantic search. Handles test file detection, module path inference, and context injection.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Chunk` | Indexable unit | `chunk.go:12-46` |
| `ChunkType` | code or doc | `chunk.go:7-10` |
| `Extractor` | Symbol→Chunk converter | `extractor.go:12-14` |

## Chunk Fields

| Field | Description |
|-------|-------------|
| `ID` | SHA256 hash of repo+path+symbol+line |
| `Repo`, `FilePath` | Source location |
| `Type` | `code` or `doc` |
| `Kind` | function, class, method, pattern |
| `ModulePath` | Hierarchical module (e.g., `fisio.imports.aws`) |
| `ModuleRoot`, `Submodule` | Split module path |
| `Content` | Full source text |
| `ContextHeader` | Injected context for methods |
| `IsTest` | True for test files |
| `RetrievalWeight` | 1.0 normal, 0.5 for tests |
| `Vector` | Embedding (populated later) |

## Usage

```go
extractor := chunk.NewExtractor()
chunks, err := extractor.Extract(source, "file.py", "my-repo", "module.path")
```

## Test File Detection

Files matching these patterns get `IsTest=true` and `RetrievalWeight=0.5`:

| Pattern | Example |
|---------|---------|
| `test_` | `test_user.py` |
| `_test.py` | `user_test.py` |
| `_test.go` | `user_test.go` |
| `.test.js` | `user.test.js` |
| `.spec.ts` | `user.spec.ts` |
| `/tests/` | `tests/test_user.py` |
| `/__tests__/` | `__tests__/user.test.js` |

## Context Headers

Methods get context headers injected for better embeddings:
```
# File: path/to/file.py
# Class: UserService
```

## Module Path Inference

File paths convert to module paths:
- `fisio/fisio/imports/aws.py` → `fisio.imports` (duplicate prefix removed)
- `src/utils/helpers.py` → `src.utils`

## Gotchas

1. **RetrievalWeight** affects search ranking - test code ranks lower
2. **ID is deterministic** - same input always produces same ID
3. **TokenEstimate** is rough (~4 chars/token)
