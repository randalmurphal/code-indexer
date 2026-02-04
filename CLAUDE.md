# code-indexer

Semantic code indexing CLI for Claude Code integration. Indexes Python/JS/TS codebases for semantic search and context-aware retrieval.

## Quick Reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Test (unit) | `make test-unit` |
| Test (all) | `make test` |
| Lint | `make lint` |
| Start infra | `make infra-up` |

## Architecture

```
cmd/code-indexer/     CLI entry point (cobra)
internal/
├── config/           Config loading (global + per-repo)
├── parser/           Tree-sitter AST parsing (Python, JS/TS)
├── chunk/            Chunk model + symbol extraction
├── embedding/        Voyage AI embedding client
├── store/            Qdrant vector storage
└── indexer/          Pipeline orchestration + file walker
```

## Data Flow

1. **Walk** → `indexer.Walker` traverses repo with glob patterns
2. **Parse** → `parser.Parser` extracts symbols via tree-sitter
3. **Chunk** → `chunk.Extractor` creates indexable chunks with metadata
4. **Embed** → `embedding.VoyageClient` generates vectors (batched)
5. **Store** → `store.QdrantStore` upserts to Qdrant collection

## Configuration

**Global config**: `~/.config/code-index/config.yaml`
```yaml
embedding:
  provider: voyage
  model: voyage-4-large
storage:
  qdrant_url: localhost:6334
  neo4j_url: bolt://localhost:7687
  redis_url: redis://localhost:6379
```

**Per-repo config**: `.ai-devtools.yaml` in repo root
```yaml
code-index:
  name: my-repo
  default_branch: main
  include: ["**/*.py", "**/*.ts"]
  exclude: ["**/vendor/**"]
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `VOYAGE_API_KEY` | Yes (indexing) | Voyage AI API key |
| `QDRANT_URL` | No | Qdrant gRPC endpoint (default: localhost:6334) |

## CLI Commands

| Command | Description |
|---------|-------------|
| `code-indexer init <path>` | Create `.ai-devtools.yaml` with auto-detected patterns |
| `code-indexer index <repo>` | Index repository (requires `VOYAGE_API_KEY`) |
| `code-indexer status` | Show index statistics |
| `code-indexer version` | Print version |

## Testing

```bash
make test-unit          # No external deps
make test-integration   # Requires Qdrant + Voyage API key
make test-coverage      # Generate coverage.html
```

**Test file conventions**:
- Unit tests: `*_test.go` alongside implementation
- Integration tests: Skip with `t.Skip()` when env vars missing
- E2E tests: `test/e2e/`

## Code Style

- Go 1.21+
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Context propagation: All I/O functions take `context.Context`
- Logging: `slog.Default()` for structured logs

## Key Packages

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `parser` | AST extraction | `Parser`, `Symbol`, `Language` |
| `chunk` | Indexable units | `Chunk`, `Extractor`, `ChunkType` |
| `embedding` | Vector generation | `VoyageClient` |
| `store` | Vector storage | `QdrantStore`, `CollectionInfo` |
| `indexer` | Pipeline | `Indexer`, `Walker`, `IndexResult` |

## Common Gotchas

1. **Qdrant URL format**: Use `localhost:6334` (gRPC), not `http://localhost:6333` (REST)
2. **TypeScript**: Uses JavaScript parser - advanced TS syntax may not extract fully
3. **Test weights**: Test files get `RetrievalWeight: 0.5` vs `1.0` for production code
4. **Module paths**: `fisio/fisio/x` → `fisio.x` (duplicate prefix removed)

## Infrastructure

Requires services from `~/repos/graphrag`:
```bash
cd ~/repos/graphrag && docker compose up -d
# Qdrant: localhost:6334 (gRPC), localhost:6333 (REST)
# Neo4j:  localhost:7687
# Redis:  localhost:6379
```

## Implementation Status

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | ✅ Complete | Core indexing pipeline |
| 2 | 📋 Planned | MCP server + Claude Code hooks |
| 3 | 📋 Planned | Query classification + pattern detection |
| 4 | 📋 Planned | Hierarchical chunking + polish |

**Design doc**: `docs/plans/2026-02-04-code-indexing-design.md`
**Phase plans**: `docs/plans/2026-02-04-code-indexing-phase*.md`
