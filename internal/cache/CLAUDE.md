# cache package

Redis caching for query results and index versioning.

## Purpose

Cache search results to reduce embedding API calls. Track index versions for cache invalidation.

## Key Types

| Type | Description | Location |
|------|-------------|----------|
| `RedisCache` | Redis client wrapper | `redis.go:12-14` |

## Cache Keys

| Pattern | Purpose | Example |
|---------|---------|---------|
| `query:<hash>` | Cached search results | `query:abc123def456` |
| `version:<repo>` | Index version | `version:my-repo` |

## Query Cache Key

`QueryCacheKey()` combines repo, query, and version:
```go
key := cache.QueryCacheKey(repo, query, version)
// "query:sha256(repo:query:version)[:16]"
```

Version ensures cache invalidation on re-index.

## Usage

```go
cache, err := cache.NewRedisCache("redis://localhost:6379")
defer cache.Close()

// Get/Set with TTL
value, err := cache.Get(ctx, key)
err = cache.Set(ctx, key, value, 10*time.Minute)

// Index version
version, err := cache.GetIndexVersion(ctx, repo)
err = cache.SetIndexVersion(ctx, repo, newVersion)
```

## TTL

Query cache TTL configured in `config.yaml`:
```yaml
cache:
  query_ttl_minutes: 10
```

Applied in `search/handler.go:263`:
```go
ttl := time.Duration(h.config.Cache.QueryTTLMinutes) * time.Minute
h.cache.Set(ctx, cacheKey, response, ttl)
```

## Gotchas

1. **Optional**: Cache unavailable doesn't fail searches
2. **Version invalidation**: Index updates should bump version
3. **TTL**: Short TTL (10 min) to balance freshness vs. API costs
4. **Connection**: Uses go-redis, supports Redis URL format
