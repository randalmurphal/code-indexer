// cmd/code-indexer/index.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/graph"
	"github.com/randalmurphy/ai-devtools-admin/internal/indexer"
	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index [repo-name-or-path]",
	Short: "Index a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runIndex,
}

var (
	indexIncremental bool
)

func init() {
	indexCmd.Flags().BoolVar(&indexIncremental, "incremental", false, "Only index changed files")
	rootCmd.AddCommand(indexCmd)
}

func runIndex(cmd *cobra.Command, args []string) error {
	repoArg := args[0]

	// Resolve repo path
	repoPath := repoArg
	if !filepath.IsAbs(repoPath) {
		// Check if it's a registered repo name or relative path
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			// Try ~/repos/{name}
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("repository not found: %s (unable to check ~/repos)", repoPath)
			}
			repoPath = filepath.Join(homeDir, "repos", repoArg)
		}
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("repository not found: %s", absPath)
	}

	// Load configs
	globalCfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		return fmt.Errorf("failed to load global config: %w", err)
	}

	repoCfg, err := config.LoadRepoConfig(absPath)
	if err != nil {
		return fmt.Errorf("failed to load repo config: %w\nRun 'code-indexer init %s' first", err, absPath)
	}

	// Get API key
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY environment variable not set")
	}

	// Create indexer
	idx, err := indexer.NewIndexer(globalCfg, voyageKey)
	if err != nil {
		return fmt.Errorf("failed to create indexer: %w", err)
	}

	ctx := context.Background()

	// Connect to Neo4j for relationship storage and incremental indexing (optional)
	var graphStore *graph.Neo4jStore
	if globalCfg.Storage.Neo4jURL != "" {
		neo4jUser := os.Getenv("NEO4J_USER")
		if neo4jUser == "" {
			neo4jUser = "neo4j"
		}
		neo4jPass := os.Getenv("NEO4J_PASSWORD")
		if neo4jPass != "" {
			graphStore, err = graph.NewNeo4jStore(globalCfg.Storage.Neo4jURL, neo4jUser, neo4jPass)
			if err != nil {
				fmt.Printf("Warning: Neo4j unavailable, relationships will not be stored: %v\n", err)
			} else {
				// Ensure schema exists for relationship storage
				if schemaErr := graphStore.EnsureSchema(ctx); schemaErr != nil {
					fmt.Printf("Warning: Failed to ensure Neo4j schema: %v\n", schemaErr)
				}
			}
		} else if indexIncremental {
			fmt.Printf("Warning: NEO4J_PASSWORD not set, falling back to full indexing\n")
		}
	}

	// Run indexing

	if indexIncremental {
		fmt.Printf("Incremental indexing %s (%s)...\n", repoCfg.Name, absPath)
	} else {
		fmt.Printf("Indexing %s (%s)...\n", repoCfg.Name, absPath)
	}

	result, err := idx.IndexWithOptions(ctx, absPath, repoCfg, indexer.IndexOptions{
		Incremental: indexIncremental,
		GraphStore:  graphStore,
	})
	if err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	if graphStore != nil {
		graphStore.Close(ctx)
	}

	// Report results
	fmt.Printf("\nIndexing complete:\n")
	fmt.Printf("  Files processed: %d\n", result.FilesProcessed)
	fmt.Printf("  Chunks created:  %d\n", result.ChunksCreated)

	if len(result.Errors) > 0 {
		fmt.Printf("  Errors: %d\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("    - %v\n", e)
		}
	}

	return nil
}

func getGlobalConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory config
		return ".code-index-config.yaml"
	}
	return filepath.Join(homeDir, ".config", "code-index", "config.yaml")
}
