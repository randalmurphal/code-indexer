// Package sync provides background synchronization for code indexing.
package sync

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/randalmurphal/code-indexer/internal/indexer"
)

// Daemon watches repositories and syncs on changes.
type Daemon struct {
	repos    []RepoWatch
	interval time.Duration
	indexer  *indexer.Indexer
	logger   *slog.Logger
	headHash map[string]string // repo name -> last known HEAD hash
}

// RepoWatch defines a repository to watch.
type RepoWatch struct {
	Name   string
	Path   string
	Config *config.RepoConfig
}

// NewDaemon creates a new sync daemon.
func NewDaemon(repos []RepoWatch, interval time.Duration, idx *indexer.Indexer, logger *slog.Logger) *Daemon {
	return &Daemon{
		repos:    repos,
		interval: interval,
		indexer:  idx,
		logger:   logger,
		headHash: make(map[string]string),
	}
}

// Run starts the daemon.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("starting sync daemon", "interval", d.interval, "repos", len(d.repos))

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Initial sync
	d.syncAll(ctx)

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("daemon shutting down")
			return ctx.Err()
		case <-ticker.C:
			d.syncAll(ctx)
		}
	}
}

func (d *Daemon) syncAll(ctx context.Context) {
	for _, repo := range d.repos {
		if err := d.syncRepo(ctx, repo); err != nil {
			d.logger.Error("sync failed", "repo", repo.Name, "error", err)
		}
	}
}

func (d *Daemon) syncRepo(ctx context.Context, repo RepoWatch) error {
	d.logger.Debug("checking repo", "name", repo.Name)

	// Get current HEAD hash
	currentHead, err := d.getGitHead(repo.Path)
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Compare with cached HEAD
	cachedHead := d.headHash[repo.Name]
	if currentHead == cachedHead {
		d.logger.Debug("repo unchanged", "name", repo.Name)
		return nil
	}

	d.logger.Info("repo changed, syncing", "name", repo.Name, "old_head", truncateHash(cachedHead), "new_head", truncateHash(currentHead))

	// Run index
	result, err := d.indexer.Index(ctx, repo.Path, repo.Config)
	if err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}

	d.logger.Info("sync complete",
		"repo", repo.Name,
		"files", result.FilesProcessed,
		"chunks", result.ChunksCreated,
	)

	// Update cached HEAD
	d.headHash[repo.Name] = currentHead

	return nil
}

// getGitHead returns the current HEAD commit hash.
func (d *Daemon) getGitHead(repoPath string) (string, error) {
	// Try git rev-parse first (most reliable)
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	// Fallback: read .git/HEAD directly
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	headData, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(string(headData))

	// If HEAD points to a ref, resolve it
	if strings.HasPrefix(content, "ref: ") {
		refPath := strings.TrimPrefix(content, "ref: ")
		refFile := filepath.Join(repoPath, ".git", refPath)
		refData, err := os.ReadFile(refFile)
		if err != nil {
			// Might be a packed ref, hash the ref name as fallback
			h := sha256.Sum256([]byte(content))
			return fmt.Sprintf("%x", h[:8]), nil
		}
		return strings.TrimSpace(string(refData)), nil
	}

	// Detached HEAD, content is the hash
	return content, nil
}

func truncateHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
