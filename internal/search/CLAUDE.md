# search package

MCP search handler with query classification and pagination.

## Purpose

Handle `search_code` tool calls from Claude Code. Classifies queries, routes to appropriate search strategy, applies pagination.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Handler` | MCP tool handler | `handler.go:25-34` |
| `Classifier` | Query type detection | `classifier.go:30-33` |
| `QueryType` | Enum: symbol/concept/relationship/flow/pattern | `classifier.go:9-16` |
| `RetrievalStrategy` | Search routing config | `classifier.go:18-28` |
| `Cursor` | Pagination state | `pagination.go:14-18` |
| `SuggestionGenerator` | Empty result suggestions | `suggestions.go:10-13` |

## Query Classification

| Type | Example | Strategy |
|------|---------|----------|
| `symbol` | "UserService class" | Symbol index first |
| `concept` | "authentication flow" | Semantic search |
| `relationship` | "what calls validateToken" | Graph expansion |
| `flow` | "how does login work" | Broader semantic |
| `pattern` | "importer pattern" | Pattern index |

Classification order in `classifier.go:50-85`:
1. Pattern regex → pattern words → relationship words → flow words → identifiers

## Search Flow

```
Query → Classify → Route → Search → Paginate → Format
                     │
    ┌────────────────┼────────────────┐
    ↓                ↓                ↓
 Symbol          Semantic         Pattern
(exact match)   (vector sim)   (filter kind)
```

## Pagination

- Cursor: base64-encoded JSON with query hash, offset, timestamp
- Expiry: 10 minutes (`pagination.go:43`)
- Offset-based: cursor contains offset, limit applied per-page

## Empty Results

`SuggestionGenerator` provides:
- Synonym suggestions (auth → authentication, login, session)
- Partial matches against known terms
- Hints about repo filtering

## Usage

```go
handler, err := search.NewHandler(cfg, voyageKey, logger)
result, err := handler.CallTool(ctx, "search_code", map[string]interface{}{
    "query": "authentication flow",
    "repo":  "my-repo",
    "limit": 10,
})
```

## Gotchas

1. **Query type ordering**: Pattern checks before symbol to catch "importer pattern"
2. **Word boundaries**: `containsWord()` prevents "use" matching in "UserService"
3. **Cursor expiry**: 10 minutes, returns error if expired
4. **Cache key**: Includes index version for invalidation
