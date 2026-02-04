package sync

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/randalmurphal/code-indexer/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonGetGitHead(t *testing.T) {
	// Create temp git repo
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Configure git for the test
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Create a file and commit
	testFile := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Test getGitHead
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	daemon := &Daemon{logger: logger, headHash: make(map[string]string)}

	head, err := daemon.getGitHead(tmpDir)
	require.NoError(t, err)
	assert.Len(t, head, 40, "HEAD should be 40 char hash")
}

func TestDaemonDetectsChange(t *testing.T) {
	// Create temp git repo
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Initial commit
	testFile := filepath.Join(tmpDir, "test.py")
	require.NoError(t, os.WriteFile(testFile, []byte("def foo(): pass"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Get initial HEAD
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	daemon := &Daemon{logger: logger, headHash: make(map[string]string)}

	head1, err := daemon.getGitHead(tmpDir)
	require.NoError(t, err)

	// Make another commit
	require.NoError(t, os.WriteFile(testFile, []byte("def foo(): return 1"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "update")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	// Get new HEAD
	head2, err := daemon.getGitHead(tmpDir)
	require.NoError(t, err)

	assert.NotEqual(t, head1, head2, "HEAD should change after commit")
}

func TestNewDaemon(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	repos := []RepoWatch{
		{Name: "test-repo", Path: "/tmp/test", Config: &config.RepoConfig{}},
	}

	// Note: We can't fully test NewDaemon without a real indexer,
	// so we just verify structure
	daemon := NewDaemon(repos, time.Minute, nil, logger)

	assert.Len(t, daemon.repos, 1)
	assert.Equal(t, time.Minute, daemon.interval)
	assert.NotNil(t, daemon.headHash)
}

func TestTruncateHash(t *testing.T) {
	assert.Equal(t, "abc12345", truncateHash("abc12345678901234567890"))
	assert.Equal(t, "short", truncateHash("short"))
	assert.Equal(t, "", truncateHash(""))
}

func TestDaemonRunCancellation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	daemon := NewDaemon([]RepoWatch{}, time.Hour, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// Run should return quickly due to cancellation
	done := make(chan error)
	go func() {
		done <- daemon.Run(ctx)
	}()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("daemon did not stop after cancellation")
	}
}
