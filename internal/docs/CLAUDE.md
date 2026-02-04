# docs package

AGENTS.md and CLAUDE.md parsing for navigation documentation.

## Purpose

Parse AGENTS.md/CLAUDE.md files and convert to indexable chunks for semantic search. Enables Claude Code to find relevant documentation alongside code.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `AgentDoc` | Parsed document | `agents.go:10-16` |
| `DocSection` | Document section | `agents.go:18-24` |

## Parsing

`ParseAgentsMD()` extracts:
- **Title**: First h1 heading
- **Description**: First non-empty line after h1
- **Sections**: Hierarchical heading paths with content
- **Entry points**: Links and file references
- **Mentioned symbols**: Code identifiers

## Heading Path

Sections track full heading hierarchy:
- `# Config` → path: `["Config"]`
- `## Storage` under `# Config` → path: `["Config", "Storage"]`

## ToChunks()

Converts parsed doc to indexable chunks:
- `kind: "navigation"`
- `RetrievalWeight: 1.5` (boosted for search relevance)
- `HeadingPath`: Joined with " > "

## Usage

```go
doc, err := docs.ParseAgentsMD(content, "AGENTS.md")
chunks := doc.ToChunks("my-repo", "path/to/AGENTS.md", "docs")
```

## Integration

Called in `indexer/indexer.go` via `indexNavigationDocs()`:
1. Walk finds AGENTS.md/CLAUDE.md files
2. Parse with `ParseAgentsMD()`
3. Convert to chunks with `ToChunks()`
4. Include in batch embedding/storage

## Gotchas

1. **Retrieval weight**: 1.5x boost ensures docs surface in searches
2. **Description capture**: First non-empty line after h1, uses `justSawH1` flag
3. **Heading reset**: h1 resets path, h2+ extends path
4. **Empty sections**: Skipped (no content to index)
