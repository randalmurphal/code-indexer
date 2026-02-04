// cmd/code-indexer/invalidate.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/randalmurphal/code-indexer/internal/cache"
	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/spf13/cobra"
)

var invalidateCmd = &cobra.Command{
	Use:   "invalidate-file [file-path]",
	Short: "Mark a file as needing re-indexing (used by Claude Code hooks)",
	Long: `Marks a file as stale in the index, triggering cache invalidation.
Called automatically by Claude Code PostToolUse hooks after file edits.

This increments the repo's index version to invalidate cached search results
and marks the specific file for re-indexing on the next index run.`,
	Args: cobra.ExactArgs(1),
	RunE: runInvalidateFile,
}

func init() {
	rootCmd.AddCommand(invalidateCmd)
}

func runInvalidateFile(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Make path absolute
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil // Silent fail
	}

	// Load config
	cfg := config.DefaultConfig()

	// Connect to Redis
	if cfg.Storage.RedisURL == "" {
		return nil // No Redis configured
	}

	redisCache, err := cache.NewRedisCache(cfg.Storage.RedisURL)
	if err != nil {
		return nil // Silent fail - don't break Claude's write
	}
	defer redisCache.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Infer repo from path
	repo := inferRepoFromPath(absPath)
	if repo == "" {
		return nil // Can't determine repo
	}

	// Increment index version to invalidate query cache
	newVersion, err := redisCache.IncrIndexVersion(ctx, repo)
	if err != nil {
		return nil // Silent fail
	}

	// Mark file as needing re-index (no expiry - persists until re-indexed)
	staleKey := "stale:" + absPath
	err = redisCache.Set(ctx, staleKey, time.Now().UTC().Format(time.RFC3339), 0)
	if err != nil {
		return nil // Silent fail
	}

	// Output to stderr (visible to Claude)
	fmt.Fprintf(os.Stderr, "[code-index] Marked %s for re-indexing (version: %d)\n", filepath.Base(filePath), newVersion)

	return nil
}

func inferRepoFromPath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	reposDir := filepath.Join(homeDir, "repos")

	// Check if path is under ~/repos
	if !strings.HasPrefix(path, reposDir) {
		return ""
	}

	// Extract repo name (first component after ~/repos/)
	rel, err := filepath.Rel(reposDir, path)
	if err != nil {
		return ""
	}

	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) == 0 || parts[0] == "" || parts[0] == "." {
		return ""
	}

	return parts[0]
}
