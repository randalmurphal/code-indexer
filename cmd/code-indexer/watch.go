package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/indexer"
	"github.com/randalmurphy/ai-devtools-admin/internal/sync"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch repositories and sync on changes",
	Long:  `Run a background daemon that watches repositories for changes and syncs the index.`,
	RunE:  runWatch,
}

var (
	watchRepos    string
	watchInterval string
)

func init() {
	watchCmd.Flags().StringVar(&watchRepos, "repos", "", "Comma-separated repo names to watch (e.g., r3,m32rimm)")
	watchCmd.Flags().StringVar(&watchInterval, "interval", "60s", "Check interval (e.g., 30s, 5m)")
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	if watchRepos == "" {
		return fmt.Errorf("--repos is required")
	}

	interval, err := time.ParseDuration(watchInterval)
	if err != nil {
		return fmt.Errorf("invalid interval: %w", err)
	}

	// Setup logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load global config
	homeDir, _ := os.UserHomeDir()
	globalConfigPath := filepath.Join(homeDir, ".config", "code-index", "config.yaml")
	cfg, err := config.LoadConfig(globalConfigPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Get API key
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY not set")
	}

	// Create indexer
	idx, err := indexer.NewIndexer(cfg, voyageKey)
	if err != nil {
		return fmt.Errorf("failed to create indexer: %w", err)
	}

	// Build repo list
	repoNames := strings.Split(watchRepos, ",")
	var repos []sync.RepoWatch

	for _, name := range repoNames {
		name = strings.TrimSpace(name)
		repoPath := filepath.Join(homeDir, "repos", name)

		// Check repo exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			logger.Warn("repo path not found", "repo", name, "path", repoPath)
			continue
		}

		repoCfg, err := config.LoadRepoConfig(repoPath)
		if err != nil {
			// Use default config if not found
			repoCfg = &config.RepoConfig{
				Name:    name,
				Include: []string{"**/*.py", "**/*.js", "**/*.ts", "**/*.go"},
				Exclude: []string{"**/node_modules/**", "**/venv/**", "**/.git/**"},
			}
			logger.Warn("using default repo config", "repo", name)
		}

		repos = append(repos, sync.RepoWatch{
			Name:   name,
			Path:   repoPath,
			Config: repoCfg,
		})
	}

	if len(repos) == 0 {
		return fmt.Errorf("no valid repos found")
	}

	// Create and run daemon
	daemon := sync.NewDaemon(repos, interval, idx, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	return daemon.Run(ctx)
}
