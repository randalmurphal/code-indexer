# Code Indexing System for Claude Code

**Date:** 2026-02-04
**Status:** Ready for Implementation
**Target Repos:** ~/repos/r3 (JS/TS), ~/repos/m32rimm (Python)

## Overview

A semantic code indexing system that integrates with Claude Code via MCP, focused on two problems LSP can't solve:

1. **Semantic discovery** - Find code by concept when you don't know the symbol names
2. **Automatic context expansion** - When Claude finds one file, surface related files it should also read

Built on existing infrastructure: graphrag library (Go), Neo4j, Qdrant, Redis, Voyage AI embeddings.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Claude Code                                  │
│                              │                                       │
│                    MCP Protocol (stdio)                              │
│                              │                                       │
│                  ┌───────────▼───────────┐                          │
│                  │   code-index-mcp      │                          │
│                  │   (Go binary)         │                          │
│                  └───────────┬───────────┘                          │
│                              │                                       │
│         ┌────────────────────┼────────────────────┐                 │
│         ▼                    ▼                    ▼                 │
│   ┌──────────┐        ┌──────────┐        ┌──────────┐             │
│   │  Qdrant  │        │  Neo4j   │        │  Redis   │             │
│   │ vectors  │        │  graph   │        │  cache   │             │
│   └──────────┘        └──────────┘        └──────────┘             │
│         ▲                    ▲                    ▲                 │
│         └────────────────────┼────────────────────┘                 │
│                              │                                       │
│                  ┌───────────▴───────────┐                          │
│                  │   code-indexer        │                          │
│                  │   (Go CLI)            │                          │
│                  └───────────────────────┘                          │
└─────────────────────────────────────────────────────────────────────┘
```

**Two binaries built on graphrag library:**

| Component | Purpose |
|-----------|---------|
| `code-indexer` | CLI that parses codebases, extracts AST, builds graph + vectors |
| `code-index-mcp` | MCP server exposing search + context to Claude Code |

**Infrastructure (already available):**

| Service | Purpose | Config |
|---------|---------|--------|
| Qdrant | Vector storage for semantic search | localhost:6333 |
| Neo4j | Graph storage for relationships | localhost:7687 |
| Redis | Caching (Merkle trees, embeddings, queries) | localhost:6379 |
| Voyage AI | Embeddings (voyage-4-large) | API key |

---

## Indexing Pipeline

### Tree-sitter Parsing

Extracts three things from source files:

1. **Chunks** - Semantic units (functions, classes, methods) with context
2. **Symbols** - Definitions and references
3. **Relationships** - Import graphs, call graphs, inheritance

```
Source File
    │
    ▼
┌─────────────────┐
│  Tree-sitter    │──► AST
│  (per-language) │
└─────────────────┘
    │
    ├──► Chunk Extractor ──► Semantic chunks with context
    │
    ├──► Symbol Extractor ──► Definitions + References
    │
    └──► Relationship Extractor ──► Graph edges
```

**Language support:**
- `tree-sitter-javascript` / `tree-sitter-typescript` for r3
- `tree-sitter-python` for m32rimm

### Hierarchical Chunking Strategy

Different strategies based on file/element size:

| File Size | Strategy |
|-----------|----------|
| < 500 tokens | Single chunk |
| 500-2000 tokens | Chunk by top-level definitions |
| > 2000 tokens | Chunk by methods, inject class context header |
| Class > 50 methods | Create class summary chunk + individual method chunks |

**Large file chunking (method-level with context):**

```
┌─────────────────────────────────────────┐
│ # File: fisio/common/activity_logger.py │
│ # Class: ActivityLogger                 │
│ # Related: log_login, log_logout        │
│                                         │
│ def log_permission_change(self, ...):   │
│     '''Log when user permissions...'''  │
│     ...actual method body...            │
└─────────────────────────────────────────┘
```

### Documentation Chunking

Documentation (`.md`, `.rst`) chunks by heading structure:

```
# Architecture Overview          → Chunk 1 (includes heading path)
## Authentication Flow           → Chunk 2 (path: "Architecture > Auth")
### Token Validation             → Chunk 3 (path: "Architecture > Auth > Tokens")
```

Each doc chunk includes:
- Heading hierarchy as context
- Section content
- Links to code symbols mentioned

---

## Storage Schema

### Neo4j Graph Model

```cypher
// Repository and module hierarchy
(:Repository {name: "m32rimm", path: "~/repos/m32rimm"})
  -[:CONTAINS]->
(:Module {
    repo: "m32rimm",
    path: "fisio.imports",
    fs_path: "fisio/fisio/imports/",
    description: "180+ data source importers"
})

// Files and symbols
(:File {
    path: "app/server/api/users.js",
    repo: "r3",
    module_root: "app",
    hash: "abc123...",
    last_indexed: timestamp
})

(:Symbol {
    name: "getUserById",
    kind: "function",
    repo: "r3",
    file_path: "app/server/api/users.js",
    start_line: 45,
    end_line: 72,
    signature: "async function getUserById(id)"
})

// Relationships
(:File)-[:IMPORTS]->(:File)
(:Symbol)-[:CALLS]->(:Symbol)
(:Symbol)-[:EXTENDS]->(:Symbol)
(:File)-[:CONTAINS]->(:Symbol)
(:Module)-[:DEPENDS_ON]->(:Module)

// Documentation links
(:AgentsDoc)-[:DESCRIBES]->(:Module)
(:AgentsDoc)-[:MENTIONS]->(:Symbol)
(:DocChunk)-[:REFERENCES]->(:Symbol)

// Patterns
(:Pattern {
    name: "importer",
    module: "fisio.imports",
    canonical_file: "aws_import.py",
    member_count: 183
})-[:FOLLOWED_BY]->(:File)
```

### Qdrant Collections

**Unified chunks collection:**

```
Collection: "chunks"
├── type: string (code | doc)           # Filterable
├── repo: string                        # Filterable
├── module_path: string                 # Full path: "fisio.imports.aws"
├── module_root: string                 # Top-level: "fisio"
├── submodule: string                   # Second level: "imports"
├── file_path: string
├── symbol_name: string?                # For code
├── kind: string?                       # function|class|method|pattern
├── heading_path: string?               # For docs: "Architecture > Auth"
├── start_line: int
├── end_line: int
├── content: string                     # Stored, may be redacted
├── has_secrets: bool                   # If secrets were redacted
├── follows_pattern: string?            # Pattern name if applicable
├── is_test: bool                       # Test file (reduced retrieval weight)
├── retrieval_weight: float             # 1.0 for production, 0.5 for tests
└── vector: float[1024]                 # voyage-4-large
```

---

## Module Hierarchy

Codebases are indexed with full module hierarchy, not as flat collections.

**Example for m32rimm:**

```
m32rimm/
├── fisio/                    # module_root: "fisio"
│   ├── imports/              # submodule: "imports"
│   ├── common/               # submodule: "common"
│   ├── aggregations/         # submodule: "aggregations"
│   └── exports/              # submodule: "exports"
├── fortress_api/             # module_root: "fortress_api"
├── fis_common/               # module_root: "fis_common"
└── clustereng_tools/         # module_root: "clustereng_tools"
```

**Configuration:**

```yaml
# .ai-devtools.yaml
code-index:
  name: m32rimm
  modules:
    fisio:
      description: "Main ETL engine"
      submodules:
        imports: "180+ data source importers"
        common: "Shared utilities (112 files)"
        aggregations: "Post-import data aggregation"
        exports: "Data export handlers"
    fortress_api:
      description: "REST API layer"
    fis_common:
      description: "Cross-package shared utilities"
    clustereng_tools:
      description: "Celery async task processing"
```

**Auto-detection fallback:**

If no config, infer from `__init__.py` locations and import statements.

---

## Pattern Recognition

For directories with many similar files (e.g., 180 importers), detect and index patterns.

**Detection:**

1. Parse all files in directory
2. Extract structural signature (AST shape)
3. Cluster by similarity
4. If cluster > 5 files with >80% structural similarity → pattern

**What gets indexed:**

```
1. Pattern chunk (high weight):
   "Importer Pattern: All importers in fisio.imports inherit from
    BaseImporter and implement fetch_data(), transform(), upsert().
    Uses @retry decorator for resilience.
    Canonical example: aws_import.py"

2. Individual files with reduced weight:
   - Only index deviations from pattern
   - Store follows_pattern metadata
```

**Configuration:**

```yaml
code-index:
  pattern_detection:
    enabled: true
    min_cluster_size: 5
    similarity_threshold: 0.8
    directories:
      - "fisio/fisio/imports/"
      - "fisio/fisio/aggregations/"
```

---

## Caching Strategy

### Layer 1: Incremental Indexing (Merkle Trees)

```
repo_root/
├── hash: sha256(children)
├── app/
│   ├── hash: sha256(children)
│   └── server/
│       ├── hash: sha256(children)
│       └── auth.js: sha256(content)
```

On re-index:
1. Rebuild Merkle tree from filesystem
2. Compare against stored tree in Redis (`index:merkle:{repo}`)
3. Only re-parse files where hashes changed
4. Propagate updates: re-embed changed chunks, update graph edges

### Layer 2: Embedding Cache

```
Redis key: embed:{model}:{content_hash}
Value: float[1024] vector
TTL: 30 days
```

Identical code across repos shares cached embeddings.

### Layer 3: Query Cache

```
Redis key: query:{hash}:{index_version}
Value: {results: [...], timestamp: ...}
TTL: 10 minutes
```

Invalidated when files in result set are re-indexed.

### Index Versioning

```
Redis key: index:version:{repo}
Value: {version: 42, last_updated: "2026-02-04T10:00:00Z"}
```

Incremented on any index change. Ensures idempotent queries within same version.

---

## Query Classification and Routing

Different query types use different retrieval strategies:

| Type | Example | Strategy | Cost |
|------|---------|----------|------|
| Symbol lookup | "what is validateToken?" | Neo4j symbol index | Low |
| Concept search | "where do we handle auth?" | Qdrant semantic + expansion | Medium |
| Relationship | "what calls validateToken?" | Neo4j graph traversal | Medium |
| Flow | "data flow from API to DB?" | Multi-hop graph + semantic | High |
| Pattern | "how do importers work?" | Pattern index lookup | Low |

**Classification (heuristic, no LLM):**

```go
func classifyQuery(query string) QueryType {
    if hasQuotedTerm(query) || hasIdentifier(query) {
        return SymbolLookup
    }
    if containsRelationshipWord(query) {  // "calls", "uses", "imports"
        return Relationship
    }
    if containsFlowWord(query) {  // "flow", "path from"
        return Flow
    }
    if containsPatternWord(query) {  // "how do X work", "pattern"
        return Pattern
    }
    return ConceptSearch  // Default
}
```

---

## MCP Server Interface

### Tool: search_code

Semantic search by concept or natural language.

```json
{
  "name": "search_code",
  "description": "Find code by concept when you don't know exact symbol names",
  "parameters": {
    "query": "string - describe what you're looking for",
    "repo": "string? - r3|m32rimm|all (default: inferred from cwd)",
    "module": "string? - filter to module (e.g., 'fisio.imports')",
    "include_tests": "string? - include|exclude|only (default: include with lower weight)",
    "cursor": "string? - pagination cursor for more results",
    "limit": "int? - max results (default: 10)"
  }
}
```

**Response:**

```json
{
  "query_type": "concept_search",
  "results": [
    {
      "file_path": "fisio/fisio/common/auth.py",
      "module": "fisio.common",
      "start_line": 45,
      "end_line": 89,
      "score": 0.85,
      "content": "def validate_token(token): ...",
      "follows_pattern": null
    }
  ],
  "total_count": 23,
  "cursor": "eyJvZmZzZXQiOjEwfQ==",
  "has_more": true,
  "index_version": 42
}
```

### Resource: codeindex://relevant

Ambient context based on conversation. Claude can read to get likely-relevant code.

```json
{
  "resources": [{
    "uri": "codeindex://relevant",
    "name": "Contextually relevant code",
    "mimeType": "text/markdown"
  }]
}
```

Server behavior:
1. Extract key terms from recent conversation
2. Run semantic search
3. Expand via graph (1-hop, weighted by importance)
4. Dedupe against files Claude already read this session
5. Return condensed context

### Graceful Empty Results

Never return just "not found". Always provide alternatives:

```json
{
  "results": [],
  "query_type": "concept_search",
  "message": "No direct matches for 'kafka consumer throttling'",
  "suggestions": [
    "Try: 'message queue' (12 results)",
    "Try: 'celery rate limit' (8 results)"
  ],
  "related_areas": [
    {"module": "clustereng_tools", "reason": "handles async processing"}
  ],
  "hint": "m32rimm may not use Kafka - check AGENTS.md for messaging architecture"
}
```

**Synonym table for suggestions:**

```yaml
# ~/.config/code-index/synonyms.yaml
synonyms:
  kafka: [queue, message, celery, async]
  authentication: [auth, login, session, token]
  database: [db, mongo, sql, storage]
```

---

## Claude Code Integration

### MCP Server Registration

```json
// ~/.claude/settings.json
{
  "mcpServers": {
    "code-index": {
      "command": "code-index-mcp",
      "args": ["serve"],
      "env": {
        "QDRANT_URL": "http://localhost:6333",
        "NEO4J_URL": "bolt://localhost:7687",
        "REDIS_URL": "redis://localhost:6379",
        "VOYAGE_API_KEY": "${VOYAGE_API_KEY}"
      }
    }
  }
}
```

### Claude Code Hooks

**Read-time context injection:**

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Read",
        "hooks": [{
          "type": "command",
          "command": "code-indexer suggest-context \"$CLAUDE_FILE_PATH\""
        }]
      }
    ]
  }
}
```

Output (to stderr, visible to Claude):

```
[code-index] Related files for app/server/auth.js:
  - app/server/sessionStore.js (imported, high usage)
  - app/models/user.js (shares 4 symbols)
```

**Write-time index invalidation:**

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [{
          "type": "command",
          "command": "code-indexer invalidate-file \"$CLAUDE_FILE_PATH\""
        }]
      }
    ]
  }
}
```

Marks file stale, triggers lazy re-index on next query.

### Background Sync Daemon

For changes outside Claude Code:

```bash
code-indexer watch --repos r3,m32rimm --interval 60s
```

Compares Merkle trees every 60s, re-indexes changed files.

---

## AGENTS.md Integration

Existing AGENTS.md files are indexed with special handling:

1. **Parse structure** - extract sections, entry points, patterns
2. **Link to code** - create graph edges for mentioned files/symbols
3. **High retrieval weight** - boost in results for architectural queries
4. **Navigation hints** - return AGENTS.md first for "how does X work?" queries

**Graph edges:**

```cypher
(:AgentsDoc {path: "fisio/AGENTS.md"})
  -[:DESCRIBES]->(:Module {path: "fisio"})

(:AgentsDoc)-[:ENTRY_POINT]->(:File {path: "fisio/main.py"})
(:AgentsDoc)-[:MENTIONS]->(:Symbol {name: "BOUpserter"})
```

**Configuration:**

```yaml
code-index:
  navigation_docs:
    - "**/AGENTS.md"
    - "**/README.md"
    - "docs/**/*.md"
  navigation_doc_boost: 1.5
```

---

## Exclusion Defaults

### Always Excluded

```yaml
exclude_always:
  - "**/.git/**"
  - "**/.svn/**"
  - "**/.idea/**"
  - "**/.vscode/**"
  - "**/*.swp"
  - "**/.DS_Store"
```

### Language-Specific (auto-applied)

```yaml
python_exclude:
  - "**/__pycache__/**"
  - "**/*.pyc"
  - "**/*.egg-info/**"
  - "**/venv/**"
  - "**/.venv/**"
  - "**/.mypy_cache/**"
  - "**/.pytest_cache/**"
  - "**/dist/**"
  - "**/build/**"

javascript_exclude:
  - "**/node_modules/**"
  - "**/dist/**"
  - "**/build/**"
  - "**/*.min.js"
  - "**/*.bundle.js"
  - "**/coverage/**"
```

### Per-Repo Override

```yaml
# .ai-devtools.yaml
code-index:
  exclude:
    - "**/archive/**"
    - "**/migrations/**"
    - "**/tests/**/data_files/**"

  include_override:
    - "**/test_*.py"  # Force include tests
```

---

## Secret Detection

### Detection Patterns

```yaml
secret_patterns:
  - name: api_key
    pattern: '(?i)(api[_-]?key|apikey)\s*[=:]\s*["\']?[\w-]{20,}'
  - name: password
    pattern: '(?i)(password|passwd|pwd)\s*[=:]\s*["\']?[^\s"'']{8,}'
  - name: aws_key
    pattern: 'AKIA[0-9A-Z]{16}'
  - name: private_key
    pattern: '-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----'
  - name: connection_string
    pattern: '(?i)(mongodb|postgres|mysql|redis):\/\/[^\s]+'
```

### Behavior

1. **During indexing:** Detect patterns, redact in stored content, flag for review
2. **In search results:** Show redacted content with warning
3. **Review command:** `code-indexer secrets --list`

**Redaction example:**

```python
# Original
DATABASE_URL = "mongodb://admin:supersecret@prod.db.com:27017/mydb"

# Stored
DATABASE_URL = "mongodb://[REDACTED]@prod.db.com:27017/mydb"
```

**False positive management:**

```yaml
code-index:
  secrets:
    ignore:
      - "config/settings.example.py:15"
      - "**/*.example"
```

---

## Metrics and Logging

### Metrics (JSONL)

```
~/.local/share/code-index/metrics.jsonl
```

Events logged:

```json
{"ts": "...", "event": "search", "query": "auth timeout", "query_type": "concept", "results": 5, "latency_ms": 120}
{"ts": "...", "event": "context_inject", "file": "auth.js", "suggestions": 3, "confidence": 0.82}
{"ts": "...", "event": "file_read", "file": "sessionStore.js", "was_suggested": true}
```

**CLI commands:**

```bash
# Summary stats
code-indexer metrics --last 7d

# Debug specific query
code-indexer metrics --query "auth timeout" --last 1h

# Find indexing gaps
code-indexer metrics --zero-results --last 30d

# Compare before/after changes
code-indexer metrics --compare --before 2026-01-15 --after 2026-01-20
```

### Application Logs

```
~/.local/share/code-index/logs/
├── mcp-server.log
├── indexer.log
└── metrics.jsonl
```

**Configuration:**

```yaml
logging:
  level: info  # error | warn | info | debug | trace
  max_size_mb: 50
  max_files: 3
```

Critical errors also go to stderr for Claude Code visibility.

---

## CLI Commands

```bash
# Initial setup
code-indexer init ~/repos/m32rimm    # Interactive config generation
code-indexer init ~/repos/r3

# Indexing
code-indexer index m32rimm           # Full index
code-indexer index r3 --incremental  # Only changed files
code-indexer sync                    # Sync all configured repos

# Background daemon
code-indexer watch --repos r3,m32rimm --interval 60s

# Cache management
code-indexer invalidate-file <path>  # Mark file stale
code-indexer invalidate m32rimm      # Clear repo cache
code-indexer cache stats             # Show cache hit rates

# Diagnostics
code-indexer status                  # Index health, staleness
code-indexer metrics --last 7d       # Usage analytics
code-indexer secrets --list          # Review detected secrets
code-indexer suggest-context <file>  # Test context suggestions

# MCP server
code-index-mcp serve                 # Start MCP server (stdio)
```

---

## Configuration Reference

### Global Config

```yaml
# ~/.config/code-index/config.yaml
embedding:
  provider: voyage
  model: voyage-4-large

storage:
  qdrant_url: http://localhost:6333
  neo4j_url: bolt://localhost:7687
  redis_url: redis://localhost:6379

logging:
  level: info
  max_size_mb: 50
  max_files: 3

pattern_detection:
  enabled: true
  min_cluster_size: 5
  similarity_threshold: 0.8
```

### Per-Repo Config

```yaml
# ~/repos/m32rimm/.ai-devtools.yaml
code-index:
  name: m32rimm
  default_branch: develop  # Branch to track for indexing

  modules:
    fisio:
      description: "Main ETL engine"
      submodules:
        imports: "180+ data source importers"
        common: "Shared utilities"
    # ...

  include:
    - "**/*.py"

  exclude:
    - "**/archive/**"
    - "**/migrations/**"
    - "**/tests/**/data_files/**"

  include_override:
    - "**/test_*.py"

  navigation_docs:
    - "**/AGENTS.md"
    - "**/README.md"
    - "docs/**/*.md"

  pattern_detection:
    directories:
      - "fisio/fisio/imports/"
      - "fisio/fisio/aggregations/"

  secrets:
    ignore:
      - "config/settings.example.py:15"
```

---

## Implementation Phases

### Phase 1: Core Indexing
- Tree-sitter parsing for Python and JS/TS
- Basic chunking (function/class level)
- Qdrant storage with voyage-4-large embeddings
- Neo4j graph for file relationships
- CLI: init, index, status

### Phase 2: MCP Integration
- MCP server with search_code tool
- Claude Code hooks (read suggestions, write invalidation)
- Query cache with Redis
- Basic metrics logging

### Phase 3: Intelligence
- Query classification and routing
- Pattern detection for similar code
- AGENTS.md integration
- Graceful empty results with suggestions
- Module hierarchy support

### Phase 4: Polish
- Hierarchical chunking for large files
- Secret detection/redaction
- Pagination
- Metrics CLI and analysis
- Background sync daemon

---

## Branch Management

Index tracks the default branch only (e.g., `develop` for m32rimm, `main` for r3). Feature branches use the default branch's index.

**Rationale:** Feature branch code is actively being written - you know where it is. The index helps with the 99% of code you *didn't* write.

**Configuration:**

```yaml
code-index:
  name: m32rimm
  default_branch: develop  # Branch to track
```

**Triggers for re-indexing:**

| Event | Trigger | Action |
|-------|---------|--------|
| Merge to default branch | Git post-merge hook | Incremental re-index of changed files |
| Checkout to default branch | Git post-checkout hook | Sync if index is stale |
| Manual | `code-indexer sync` | Full Merkle tree comparison |

**Git hooks setup:**

```bash
# .git/hooks/post-merge
#!/bin/sh
code-indexer sync --incremental

# .git/hooks/post-checkout
#!/bin/sh
# $3 is 1 for branch checkout, 0 for file checkout
if [ "$3" = "1" ]; then
    current_branch=$(git rev-parse --abbrev-ref HEAD)
    default_branch=$(code-indexer config get default_branch)
    if [ "$current_branch" = "$default_branch" ]; then
        code-indexer sync --incremental
    fi
fi
```

**Index state tracking:**

```
Redis key: index:branch:{repo}
Value: {branch: "develop", commit: "abc123", indexed_at: "2026-02-04T10:00:00Z"}
```

On sync, compare current HEAD of default branch against stored commit. If different, incremental re-index.

---

## Test File Handling

Test files are indexed by default but with reduced retrieval weight.

**Rationale:** Tests show how code is actually used ("how do I call this function?"), but shouldn't dominate search results over production code.

**Configuration:**

```yaml
# Qdrant metadata for test files
is_test: true
retrieval_weight: 0.5  # Half weight of production code
```

**Detection patterns:**

```yaml
test_patterns:
  python:
    - "**/test_*.py"
    - "**/*_test.py"
    - "**/tests/**/*.py"
  javascript:
    - "**/*.test.js"
    - "**/*.spec.js"
    - "**/test/**/*.js"
    - "**/__tests__/**/*.js"
```

**Retrieval behavior:**

Test results appear below production code with equivalent semantic scores. A test with score 0.90 ranks below production code with score 0.85 (0.90 × 0.5 = 0.45 effective).

To explicitly search tests: `search_code("auth tests", include_tests: "only")`

---

## Embedding Model Strategy

Embedding model is version-pinned. Upgrades are manual and explicit.

**Current model:** `voyage-4-large`

**Upgrade process:**

```bash
# Check current model
code-indexer config get embedding.model
# voyage-4-large

# Upgrade (requires full re-index)
code-indexer upgrade-embeddings --model voyage-5-large

# This will:
# 1. Confirm cost estimate (~$X for Y chunks)
# 2. Create new Qdrant collection with new model
# 3. Re-embed all chunks
# 4. Swap active collection
# 5. Delete old collection (after confirmation)
```

**Rationale:** Embedding models don't improve dramatically month-to-month. Upgrade when there's a clear reason (new code-specific model, significant quality improvement), not automatically.

**Version tracking:**

```
Redis key: index:embedding:{repo}
Value: {model: "voyage-4-large", version: "2026-01"}
```

---

## Cross-Repository Search

Default: search current repository only (inferred from working directory).

Cross-repo search requires explicit `repo: "all"` parameter.

**Rationale:** r3 (JS/TS) and m32rimm (Python) are different tech stacks with different patterns. Cross-repo search is a deliberate action, not a default.

**Behavior:**

```json
// From within ~/repos/m32rimm
{"query": "auth handler"}
// → Searches m32rimm only

{"query": "auth handler", "repo": "all"}
// → Searches m32rimm and r3

{"query": "auth handler", "repo": "r3"}
// → Searches r3 only (even if cwd is m32rimm)
```

**Response includes repo context:**

```json
{
  "results": [
    {"file_path": "...", "repo": "m32rimm", ...},
    {"file_path": "...", "repo": "r3", ...}
  ],
  "searched_repos": ["m32rimm", "r3"]
}
```
