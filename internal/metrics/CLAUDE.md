# metrics package

JSONL event logging for analytics and debugging.

## Purpose

Log search events, context injections, file reads, and errors to a JSONL file for later analysis.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Logger` | Thread-safe JSONL writer | `logger.go:15-18` |

## Usage

```go
logger, err := metrics.NewLogger("~/.local/share/code-index/metrics.jsonl")
defer logger.Close()

logger.LogSearch("auth timeout", "concept", 5, 120, false)
logger.LogContextInject("auth.js", 3, 0.82)
logger.LogFileRead("sessionStore.js", true)
logger.LogIndexUpdate("r3", 10, 45)
logger.LogError("search", "connection timeout")
```

## Event Types

| Event | Fields |
|-------|--------|
| `search` | query, query_type, results, latency_ms, cache_hit |
| `context_inject` | file, suggestions, confidence |
| `file_read` | file, was_suggested |
| `index_update` | repo, files_changed, chunks_updated |
| `error` | operation, message |

## Output Format

One JSON object per line (JSONL):
```json
{"ts":"2026-02-04T12:00:00Z","event":"search","query":"auth","results":5,"latency_ms":120}
```

## File Location

Default: `~/.local/share/code-index/metrics.jsonl`

## Gotchas

1. **Thread-safe** - Uses mutex for concurrent writes
2. **Append-only** - File opened with O_APPEND flag
3. **Fire and forget** - Log methods don't return errors
