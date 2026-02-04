# code-indexer

Semantic code indexing for Claude Code. Indexes Python/JS/TS codebases for semantic search via MCP server.

## Quick Navigation

| Need to... | Go to... |
|------------|----------|
| Add a language | `internal/parser/CLAUDE.md` |
| Understand chunking | `internal/chunk/CLAUDE.md` |
| Debug search | `internal/search/CLAUDE.md` |
| Add storage backend | `internal/store/CLAUDE.md` |
| Understand patterns | `internal/pattern/CLAUDE.md` |
| Add metrics | `internal/metrics/CLAUDE.md` |

## Commands

```bash
# Build
go build -o bin/code-indexer ./cmd/code-indexer
go build -o bin/code-index-mcp ./cmd/code-index-mcp

# Test
go test ./...                           # All tests
go test ./internal/...                  # Unit tests only
go test ./test/e2e/... -v               # E2E (needs VOYAGE_API_KEY, QDRANT_URL)

# Run
code-indexer init ~/repos/my-repo       # Create .ai-devtools.yaml
code-indexer index my-repo              # Index repository
code-indexer status                     # Show statistics
code-indexer metrics --last 7d          # Usage analytics
code-indexer watch --repos r3,m32rimm   # Background sync daemon
```

## Project Structure

```
cmd/
├── code-indexer/          CLI (cobra)
│   ├── main.go            Root command
│   ├── init.go            Init repo config
│   ├── index.go           Index repository
│   ├── status.go          Show stats
│   ├── metrics.go         Usage analytics
│   └── watch.go           Background sync
└── code-index-mcp/        MCP server for Claude Code
    └── main.go

internal/
├── config/                Global + per-repo config
├── parser/                Tree-sitter AST extraction
├── chunk/                 Chunk model + extraction + hierarchy
├── embedding/             Voyage AI vectors
├── store/                 Qdrant vector storage
├── indexer/               Pipeline + walker + modules
├── search/                Query handler + classification + pagination
├── pattern/               Code pattern detection
├── security/              Secret detection + redaction
├── sync/                  Background sync daemon
├── cache/                 Redis query caching
├── metrics/               JSONL logging + analytics
├── mcp/                   MCP protocol types + server
└── docs/                  AGENTS.md/CLAUDE.md parsing

test/e2e/                  End-to-end tests
docs/plans/                Design + implementation plans
```

## Data Flow

```
Walk → Parse → Chunk → Embed → Store
  │       │       │       │       │
  │       │       │       │       └── store/qdrant.go
  │       │       │       └── embedding/voyage.go
  │       │       └── chunk/extractor.go (+ hierarchy.go, security redaction)
  │       └── parser/parser.go (python.go, javascript.go)
  └── indexer/walker.go

Search: Query → Classify → Embed → Search → Paginate → Response
           │        │         │        │         │
           │        │         │        │         └── search/pagination.go
           │        │         │        └── store/qdrant.go
           │        │         └── embedding/voyage.go
           │        └── search/classifier.go
           └── search/handler.go
```

## Key Types

| Package | Type | Purpose | Location |
|---------|------|---------|----------|
| `chunk` | `Chunk` | Indexable code unit | `chunk.go:12-49` |
| `chunk` | `Extractor` | Symbol→Chunk converter | `extractor.go:12-17` |
| `chunk` | `HierarchicalChunker` | Large file splitting | `hierarchy.go:16-21` |
| `parser` | `Symbol` | Parsed code symbol | `parser.go:32-42` |
| `parser` | `Parser` | Tree-sitter wrapper | `parser.go:44-49` |
| `indexer` | `Indexer` | Pipeline coordinator | `indexer.go:22-29` |
| `indexer` | `ModuleResolver` | Path→module mapping | `module.go:12-16` |
| `search` | `Handler` | MCP search handler | `handler.go:25-34` |
| `search` | `Classifier` | Query type detection | `classifier.go:30-33` |
| `pattern` | `Detector` | Pattern clustering | `detector.go:20-25` |
| `store` | `QdrantStore` | Vector operations | `qdrant.go:14-17` |
| `security` | `SecretDetector` | Credential redaction | `secrets.go:18-23` |
| `sync` | `Daemon` | Background indexer | `daemon.go:18-26` |

## Configuration

**Global**: `~/.config/code-index/config.yaml`
```yaml
embedding:
  model: voyage-4-large
storage:
  qdrant_url: http://localhost:6333
  redis_url: redis://localhost:6379
cache:
  query_ttl_minutes: 10
```

**Per-repo**: `.ai-devtools.yaml`
```yaml
code-index:
  name: my-repo
  include: ["**/*.py", "**/*.ts"]
  exclude: ["**/node_modules/**"]
```

## Environment Variables

| Variable | Required | Default |
|----------|----------|---------|
| `VOYAGE_API_KEY` | Yes (indexing/search) | - |
| `QDRANT_URL` | No | `http://localhost:6333` |
| `REDIS_URL` | No | `redis://localhost:6379` |

## Testing Conventions

- **Unit tests**: `*_test.go` alongside code, no external deps
- **Integration tests**: Skip when env vars missing (`t.Skip()`)
- **E2E tests**: `test/e2e/`, require full infrastructure
- **Test file detection**: Patterns in `chunk/extractor.go:19-30`

## Code Style

- Go 1.21+
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Context propagation: All I/O functions take `context.Context`
- Logging: `slog.Default()` for structured logs
- Batch sizes: 64 for embeddings, 100 for Qdrant upserts

## Common Gotchas

1. **Qdrant URL**: Use `http://localhost:6333` (REST) not `:6334` (gRPC)
2. **TypeScript**: Uses JS parser - interfaces/type annotations not fully extracted
3. **Test weights**: Test files get `RetrievalWeight: 0.5`
4. **Module paths**: `fisio/fisio/x` → `fisio.x` (duplicate prefix removed)
5. **Large classes**: >50 methods triggers hierarchical chunking
6. **Secret detection**: Placeholder patterns (your-*, example) skipped
7. **Cursor expiry**: Pagination cursors expire after 10 minutes
8. **HEAD detection**: Daemon uses `git rev-parse HEAD` for change detection

## Boundaries

**Always do:**
- Run `go test ./...` before committing
- Wrap errors with context
- Pass `context.Context` to I/O functions
- Use batch operations for embeddings/storage

**Ask first:**
- Schema changes to `Chunk` struct (affects stored data)
- Changes to MCP protocol types
- New external dependencies

**Never do:**
- Commit API keys or credentials
- Skip secret redaction in chunk content
- Use blocking I/O without context

## Implementation Status

| Phase | Status | Features |
|-------|--------|----------|
| 1 | ✅ | Core pipeline: walk, parse, chunk, embed, store |
| 2 | ✅ | MCP server, Claude Code integration, caching |
| 3 | ✅ | Query classification, pattern detection, AGENTS.md parsing |
| 4 | ✅ | Hierarchical chunking, secrets, pagination, metrics, sync |

## Related Documentation

- Design doc: `docs/plans/2026-02-04-code-indexing-design.md`
- Phase plans: `docs/plans/2026-02-04-code-indexing-phase*.md`
- Package docs: `internal/*/CLAUDE.md`
