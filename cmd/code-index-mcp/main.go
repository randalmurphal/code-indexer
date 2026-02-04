// cmd/code-index-mcp/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/randalmurphy/ai-devtools-admin/internal/config"
	"github.com/randalmurphy/ai-devtools-admin/internal/mcp"
	"github.com/randalmurphy/ai-devtools-admin/internal/search"
	"github.com/spf13/cobra"
)

const (
	serverName    = "code-index-mcp"
	serverVersion = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "code-index-mcp",
	Short: "MCP server for semantic code search",
	Long:  `An MCP (Model Context Protocol) server that provides semantic code search tools for Claude Code.`,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the MCP server",
	Long:  `Start the MCP server listening on stdin/stdout for JSON-RPC messages.`,
	RunE:  runServe,
}

var (
	logFile string
)

func init() {
	serveCmd.Flags().StringVar(&logFile, "log-file", "", "Log file path (defaults to ~/.cache/code-index-mcp/server.log)")
	rootCmd.AddCommand(serveCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	// Set up logging to file (NOT stdout - that's for MCP protocol)
	logger, cleanup, err := setupLogging()
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer cleanup()

	logger.Info("starting MCP server", "name", serverName, "version", serverVersion)

	// Load configuration
	cfg := config.DefaultConfig()

	// Get Voyage API key from environment
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY environment variable is required")
	}

	// Create search handler
	handler, err := search.NewHandler(cfg, voyageKey, logger)
	if err != nil {
		return fmt.Errorf("failed to create handler: %w", err)
	}
	defer handler.Close()

	// Create server
	server := mcp.NewServer(serverName, serverVersion, handler, logger)

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Run server with stdin/stdout
	if err := server.Run(ctx, os.Stdin, os.Stdout); err != nil {
		if err == context.Canceled {
			logger.Info("server stopped")
			return nil
		}
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func setupLogging() (*slog.Logger, func(), error) {
	path := logFile
	if path == "" {
		// Default to ~/.cache/code-index-mcp/server.log
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = "/tmp"
		}
		logDir := filepath.Join(cacheDir, "code-index-mcp")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
		}
		path = filepath.Join(logDir, "server.log")
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cleanup := func() {
		file.Close()
	}

	return logger, cleanup, nil
}
