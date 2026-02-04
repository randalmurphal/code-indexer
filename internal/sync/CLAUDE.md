# sync package

Background synchronization daemon for automatic re-indexing.

## Purpose

Watch repositories for changes (new commits) and trigger re-indexing automatically.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `Daemon` | Background sync controller | `daemon.go:18-26` |
| `RepoWatch` | Repository to watch | `daemon.go:28-32` |

## How It Works

1. Daemon starts with list of repos and check interval
2. On each tick: get current `git HEAD` hash
3. Compare with cached hash
4. If different: trigger full re-index
5. Update cached hash on success

## HEAD Detection

`getGitHead()` in `daemon.go:101-130`:
1. Try `git rev-parse HEAD` (most reliable)
2. Fallback: read `.git/HEAD` directly
3. Resolve refs if HEAD points to branch

## Usage

```go
daemon := sync.NewDaemon(repos, interval, indexer, logger)

ctx, cancel := context.WithCancel(context.Background())
go daemon.Run(ctx)

// Later: cancel() to stop
```

## CLI Command

```bash
code-indexer watch --repos r3,m32rimm --interval 60s
```

**Flags**:
- `--repos`: Comma-separated repo names (looks in `~/repos/<name>`)
- `--interval`: Check interval (default: 60s)

## Signal Handling

Daemon handles `SIGINT`/`SIGTERM` for graceful shutdown:
```go
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh
cancel()
```

## Gotchas

1. **Initial sync**: Runs immediately on startup, then on interval
2. **Full re-index**: No incremental support yet, re-indexes entire repo
3. **Repo path**: Assumes `~/repos/<repo-name>` structure
4. **Config fallback**: Uses default patterns if `.ai-devtools.yaml` missing
5. **Error handling**: Logs errors but continues checking other repos
