# metrics package

JSONL event logging and analysis for analytics and debugging.

## Purpose

Log search events to JSONL file and analyze logs for operational insights. Supports real-time logging and offline analysis.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Logger` | Thread-safe JSONL writer | `logger.go:15-18` |
| `Analyzer` | Log parsing/analysis | `analyzer.go:14-16` |
| `Summary` | Analysis results | `analyzer.go:18-28` |
| `QueryCount` | Query frequency | `analyzer.go:30-33` |

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

## Analysis

`Analyzer` parses JSONL logs for insights:

```go
analyzer := metrics.NewAnalyzer(logPath)
summary, err := analyzer.Analyze(24 * time.Hour)  // Last 24 hours
zeroResults, err := analyzer.GetZeroResultQueries(24 * time.Hour)
topQueries, err := analyzer.GetTopQueries(24 * time.Hour, 10)
```

## Summary Fields

| Field | Description |
|-------|-------------|
| `TotalSearches` | Total search count |
| `SearchesByType` | Map of query_type → count |
| `AvgLatencyMs` | Average search latency |
| `CacheHitRate` | Cache hit percentage (0-1) |
| `ZeroResultRate` | Zero-result percentage (0-1) |

## CLI

```bash
code-indexer metrics --last 24h
code-indexer metrics --zero-results --last 7d
code-indexer metrics --json --last 1h
```

## Gotchas

1. **Thread-safe** - Uses mutex for concurrent writes
2. **Append-only** - File opened with O_APPEND flag
3. **Fire and forget** - Log methods don't return errors
4. **Time filtering** - Analyzer filters by `ts` field in JSONL
5. **Zero results** - Queries with `results: 0` tracked for search quality
