// cmd/code-indexer/status.go
package main

import (
	"context"
	"fmt"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index status",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadConfig(getGlobalConfigPath())
	if err != nil {
		fmt.Println("No global config found, using defaults")
		cfg = config.DefaultConfig()
	}

	// Connect to Qdrant
	qdrantStore, err := store.NewQdrantStore(cfg.Storage.QdrantURL)
	if err != nil {
		return fmt.Errorf("failed to connect to Qdrant at %s: %w", cfg.Storage.QdrantURL, err)
	}

	ctx := context.Background()

	// Get collection info
	info, err := qdrantStore.CollectionInfo(ctx, "chunks")
	if err != nil {
		fmt.Println("No index found. Run 'code-indexer index <repo>' to create one.")
		return nil
	}

	fmt.Println("Index Status:")
	fmt.Printf("  Collection: chunks\n")
	fmt.Printf("  Points:     %d\n", info.PointsCount)
	fmt.Printf("  Vectors:    %d dimensions\n", info.VectorSize)
	fmt.Printf("  Status:     %s\n", info.Status)

	return nil
}
