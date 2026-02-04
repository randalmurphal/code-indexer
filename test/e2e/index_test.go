package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexEndToEnd(t *testing.T) {
	if os.Getenv("VOYAGE_API_KEY") == "" {
		t.Skip("VOYAGE_API_KEY not set")
	}
	if os.Getenv("QDRANT_URL") == "" {
		t.Skip("QDRANT_URL not set")
	}

	// Build CLI
	projectRoot := getProjectRoot()
	cmd := exec.Command("go", "build", "-o", "bin/code-indexer", "./cmd/code-indexer")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", output)

	// Create test repo
	tmpDir := t.TempDir()
	testRepo := filepath.Join(tmpDir, "test-repo")
	require.NoError(t, os.MkdirAll(testRepo, 0755))

	// Add test file
	pyCode := `
def greet(name: str) -> str:
    """Greet someone."""
    return f"Hello, {name}!"

class Greeter:
    """A greeter class."""

    def __init__(self, prefix: str):
        self.prefix = prefix

    def greet(self, name: str) -> str:
        return f"{self.prefix} {name}!"
`
	require.NoError(t, os.WriteFile(filepath.Join(testRepo, "greeter.py"), []byte(pyCode), 0644))

	// Initialize repo
	cliPath := filepath.Join(projectRoot, "bin", "code-indexer")

	initCmd := exec.Command(cliPath, "init", testRepo)
	initCmd.Env = os.Environ()
	output, err = initCmd.CombinedOutput()
	require.NoError(t, err, "init failed: %s", output)

	// Verify config was created
	configPath := filepath.Join(testRepo, ".ai-devtools.yaml")
	_, err = os.Stat(configPath)
	require.NoError(t, err, "config file should exist")

	// Index repo
	indexCmd := exec.Command(cliPath, "index", testRepo)
	indexCmd.Env = os.Environ()
	output, err = indexCmd.CombinedOutput()
	require.NoError(t, err, "index failed: %s", output)

	// Verify output mentions chunks created
	require.Contains(t, string(output), "Chunks created:")

	// Check status
	statusCmd := exec.Command(cliPath, "status")
	statusCmd.Env = os.Environ()
	output, err = statusCmd.CombinedOutput()
	require.NoError(t, err, "status failed: %s", output)
	require.Contains(t, string(output), "Points:")
}

func getProjectRoot() string {
	// Walk up until we find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}
