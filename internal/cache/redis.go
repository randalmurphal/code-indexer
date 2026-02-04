// Package cache provides caching implementations.
package cache

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache provides caching via Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache.
func NewRedisCache(url string) (*RedisCache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// Get retrieves a value from cache. Returns empty string if key not found.
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Set stores a value in cache with TTL.
func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

// Delete removes a value from cache.
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// DeletePattern removes all keys matching pattern.
func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// GetIndexVersion retrieves the current index version for a repo.
func (c *RedisCache) GetIndexVersion(ctx context.Context, repo string) (int64, error) {
	val, err := c.client.Get(ctx, "index:version:"+repo).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// IncrIndexVersion increments the index version.
func (c *RedisCache) IncrIndexVersion(ctx context.Context, repo string) (int64, error) {
	return c.client.Incr(ctx, "index:version:"+repo).Result()
}

// Close closes the Redis connection.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// QueryCacheKey generates a cache key for a search query.
func QueryCacheKey(repo, query string, version int64) string {
	h := sha256.Sum256([]byte(query))
	return fmt.Sprintf("query:%s:%x:%d", repo, h[:8], version)
}

// EmbeddingCacheKey generates a cache key for an embedding.
func EmbeddingCacheKey(model, contentHash string) string {
	return fmt.Sprintf("embed:%s:%s", model, contentHash)
}
