// cmd/code-indexer/suggest.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/randalmurphy/ai-devtools-admin/internal/chunk"
	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/embedding"
	"github.com/randalmurphy/ai-devtools-admin/internal/graph"
	"github.com/randalmurphy/ai-devtools-admin/internal/store"
	"github.com/spf13/cobra"
)

var suggestCmd = &cobra.Command{
	Use:   "suggest-context [file-path]",
	Short: "Suggest related files for context (used by Claude Code hooks)",
	Long: `Analyzes the given file and suggests semantically related files that
may be relevant context. Output goes to stderr so Claude can see it.

This command is designed to be called by Claude Code PreToolUse hooks
when reading files. It fails silently to avoid breaking Claude's operations.`,
	Args: cobra.ExactArgs(1),
	RunE: runSuggestContext,
}

var suggestLimit int

func init() {
	suggestCmd.Flags().IntVar(&suggestLimit, "limit", 3, "Maximum suggestions to show")
	rootCmd.AddCommand(suggestCmd)
}

func runSuggestContext(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Make path absolute
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil // Silent fail
	}

	// Check file exists and is readable
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil // Silent fail - file might not exist yet
	}

	// Skip if file is too small (likely not code)
	if len(content) < 50 {
		return nil
	}

	// Get Voyage API key
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return nil // Silent fail - no API key
	}

	// Load config
	cfg := config.DefaultConfig()

	// Connect to Qdrant
	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return nil // Silent fail - Qdrant not available
	}
	defer qdrantStore.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Generate embedding for the file content (use first 2000 chars to stay within limits)
	queryText := string(content)
	if len(queryText) > 2000 {
		queryText = queryText[:2000]
	}

	embedder := embedding.NewVoyageClient(voyageKey, cfg.Embedding.Model)
	vectors, err := embedder.Embed(ctx, []string{queryText})
	if err != nil {
		return nil // Silent fail
	}

	// Deduplicate by file and exclude current file
	seen := make(map[string]bool)
	seen[absPath] = true
	seen[filePath] = true
	suggestions := []relatedFile{}

	// First, try to find related files via graph relationships
	graphRelated := findRelatedFilesViaGraph(ctx, cfg, filePath, suggestLimit)
	for _, rel := range graphRelated {
		normalizedPath := normalizePath(rel.Path)
		if seen[normalizedPath] || seen[rel.Path] {
			continue
		}
		seen[normalizedPath] = true
		seen[rel.Path] = true
		suggestions = append(suggestions, rel)
	}

	// If we still need more suggestions, use semantic search
	if len(suggestions) < suggestLimit {
		// Search for similar chunks
		related, err := qdrantStore.Search(ctx, "chunks", vectors[0], suggestLimit*5, nil)
		if err == nil {
			for _, c := range related {
				normalizedPath := normalizePath(c.FilePath)
				if seen[normalizedPath] || seen[c.FilePath] {
					continue
				}
				seen[normalizedPath] = true
				seen[c.FilePath] = true

				reason := inferRelationReason(absPath, c)
				suggestions = append(suggestions, relatedFile{
					Path:   c.FilePath,
					Reason: reason,
				})

				if len(suggestions) >= suggestLimit {
					break
				}
			}
		}
	}

	if len(suggestions) == 0 {
		return nil
	}

	// Output to stderr (visible to Claude)
	fmt.Fprintf(os.Stderr, "[code-index] Related files for %s:\n", filepath.Base(filePath))
	for _, s := range suggestions {
		fmt.Fprintf(os.Stderr, "  - %s (%s)\n", s.Path, s.Reason)
	}

	return nil
}

type relatedFile struct {
	Path   string
	Reason string
}

func normalizePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// findRelatedFilesViaGraph uses Neo4j to find files related via imports, calls, or extends.
func findRelatedFilesViaGraph(ctx context.Context, cfg *config.Config, filePath string, limit int) []relatedFile {
	if cfg.Storage.Neo4jURL == "" {
		return nil
	}

	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		return nil // No password configured
	}

	graphStore, err := graph.NewNeo4jStore(cfg.Storage.Neo4jURL, neo4jUser, neo4jPass)
	if err != nil {
		return nil // Silent fail - Neo4j not available
	}
	defer graphStore.Close(ctx)

	// Infer repo from file path (assumes ~/repos/<repo>/... structure)
	repo := inferRepoFromPath(filePath)
	if repo == "" {
		return nil
	}

	// Make path relative to repo
	homeDir, _ := os.UserHomeDir()
	repoPath := filepath.Join(homeDir, "repos", repo)
	relPath, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return nil
	}

	// Find related files via graph
	related, err := graphStore.FindRelatedFiles(ctx, repo, relPath, limit)
	if err != nil {
		return nil
	}

	var results []relatedFile
	for _, f := range related {
		results = append(results, relatedFile{
			Path:   f.Path,
			Reason: "imports/calls relationship",
		})
	}

	return results
}

func inferRelationReason(sourcePath string, target chunk.Chunk) string {
	sourceDir := filepath.Dir(sourcePath)
	targetDir := filepath.Dir(target.FilePath)

	// Same directory
	if sourceDir == targetDir {
		return "same directory"
	}

	// Same module
	if target.ModulePath != "" {
		sourceBase := filepath.Base(filepath.Dir(sourcePath))
		if strings.Contains(target.ModulePath, sourceBase) {
			return "same module"
		}
	}

	// Related by kind
	if target.Kind != "" {
		return fmt.Sprintf("similar %s", target.Kind)
	}

	return "semantically related"
}
