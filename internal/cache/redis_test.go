// Package cache provides caching implementations.
package cache

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisCache(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	cache, err := NewRedisCache(redisURL)
	if err != nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()

	// Test set and get
	key := "test:query:abc123"
	value := `{"results": []}`

	err = cache.Set(ctx, key, value, 1*time.Minute)
	require.NoError(t, err)

	got, err := cache.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	// Test invalidation
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	got, err = cache.Get(ctx, key)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRedisCacheIndexVersion(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	cache, err := NewRedisCache(redisURL)
	if err != nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()
	repo := "test-repo-version"

	// Clean up first
	_ = cache.Delete(ctx, "index:version:"+repo)

	// First call should return 0
	version, err := cache.GetIndexVersion(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, int64(0), version)

	// Increment and verify
	newVersion, err := cache.IncrIndexVersion(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, int64(1), newVersion)

	version, err = cache.GetIndexVersion(ctx, repo)
	require.NoError(t, err)
	assert.Equal(t, int64(1), version)

	// Clean up
	_ = cache.Delete(ctx, "index:version:"+repo)
}

func TestRedisCacheDeletePattern(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	cache, err := NewRedisCache(redisURL)
	if err != nil {
		t.Skip("Redis not available")
	}

	ctx := context.Background()

	// Set multiple keys
	_ = cache.Set(ctx, "test:pattern:a", "1", time.Minute)
	_ = cache.Set(ctx, "test:pattern:b", "2", time.Minute)
	_ = cache.Set(ctx, "test:other:c", "3", time.Minute)

	// Delete pattern
	err = cache.DeletePattern(ctx, "test:pattern:*")
	require.NoError(t, err)

	// Verify pattern keys deleted
	got, _ := cache.Get(ctx, "test:pattern:a")
	assert.Empty(t, got)
	got, _ = cache.Get(ctx, "test:pattern:b")
	assert.Empty(t, got)

	// Verify other key not deleted
	got, _ = cache.Get(ctx, "test:other:c")
	assert.Equal(t, "3", got)

	// Clean up
	_ = cache.Delete(ctx, "test:other:c")
}

func TestQueryCacheKey(t *testing.T) {
	key := QueryCacheKey("test-repo", "hello world", 42)
	assert.Contains(t, key, "query:")
	assert.Contains(t, key, "test-repo")
	assert.Contains(t, key, ":42")

	// Same inputs produce same key
	key2 := QueryCacheKey("test-repo", "hello world", 42)
	assert.Equal(t, key, key2)

	// Different query produces different key
	key3 := QueryCacheKey("test-repo", "goodbye world", 42)
	assert.NotEqual(t, key, key3)

	// Different version produces different key
	key4 := QueryCacheKey("test-repo", "hello world", 43)
	assert.NotEqual(t, key, key4)
}
