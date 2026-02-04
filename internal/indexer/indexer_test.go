package indexer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalker(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create a Python file
	pyContent := `
def hello():
    """Say hello."""
    return "Hello"
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.py"), []byte(pyContent), 0644)
	require.NoError(t, err)

	// Create a test file (should still be included - test filtering is separate)
	testContent := `
def test_hello():
    assert hello() == "Hello"
`
	err = os.WriteFile(filepath.Join(tmpDir, "test_hello.py"), []byte(testContent), 0644)
	require.NoError(t, err)

	// Create __pycache__ (should be excluded)
	err = os.MkdirAll(filepath.Join(tmpDir, "__pycache__"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "__pycache__", "test.pyc"), []byte("binary"), 0644)
	require.NoError(t, err)

	// Walk and count files
	walker := NewWalker([]string{"**/*.py"}, nil)

	var files []string
	err = walker.Walk(tmpDir, func(path string) error {
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)

	// Should find 2 Python files, not the .pyc
	require.Len(t, files, 2)
}

func TestWalkerDefaultExcludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be excluded by default
	excludedDirs := []string{".git", "node_modules", "venv", ".venv", "dist", "build"}
	for _, dir := range excludedDirs {
		err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, dir, "file.py"), []byte("# excluded"), 0644)
		require.NoError(t, err)
	}

	// Create a file that should be included
	err := os.WriteFile(filepath.Join(tmpDir, "main.py"), []byte("# included"), 0644)
	require.NoError(t, err)

	walker := NewWalker([]string{"**/*.py"}, nil)

	var files []string
	err = walker.Walk(tmpDir, func(path string) error {
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)

	// Should only find main.py
	require.Len(t, files, 1)
	require.Contains(t, files[0], "main.py")
}

func TestWalkerCustomExcludes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files
	err := os.WriteFile(filepath.Join(tmpDir, "main.py"), []byte("# main"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "generated.py"), []byte("# generated"), 0644)
	require.NoError(t, err)

	// Exclude generated files
	walker := NewWalker([]string{"**/*.py"}, []string{"**/generated.py"})

	var files []string
	err = walker.Walk(tmpDir, func(path string) error {
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)

	require.Len(t, files, 1)
	require.Contains(t, files[0], "main.py")
}

func TestWalkerNestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	err := os.MkdirAll(filepath.Join(tmpDir, "src", "pkg", "sub"), 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "src", "main.py"), []byte("# main"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "src", "pkg", "util.py"), []byte("# util"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "src", "pkg", "sub", "deep.py"), []byte("# deep"), 0644)
	require.NoError(t, err)

	walker := NewWalker([]string{"**/*.py"}, nil)

	var files []string
	err = walker.Walk(tmpDir, func(path string) error {
		files = append(files, path)
		return nil
	})
	require.NoError(t, err)

	require.Len(t, files, 3)
}

func TestInferModulePath(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		expected string
	}{
		{
			name:     "simple path",
			relPath:  "pkg/util.py",
			expected: "pkg",
		},
		{
			name:     "nested path",
			relPath:  "src/pkg/sub/deep.py",
			expected: "src.pkg.sub",
		},
		{
			name:     "duplicated prefix",
			relPath:  "fisio/fisio/imports/aws.py",
			expected: "fisio.imports",
		},
		{
			name:     "root file",
			relPath:  "main.py",
			expected: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferModulePath(tt.relPath, nil)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestComputeFileHash(t *testing.T) {
	// Test that the same content produces the same hash
	content1 := []byte("def hello():\n    return 'Hello'\n")
	content2 := []byte("def hello():\n    return 'Hello'\n")
	content3 := []byte("def goodbye():\n    return 'Goodbye'\n")

	hash1 := computeFileHash(content1)
	hash2 := computeFileHash(content2)
	hash3 := computeFileHash(content3)

	// Same content should produce same hash
	require.Equal(t, hash1, hash2)

	// Different content should produce different hash
	require.NotEqual(t, hash1, hash3)

	// Hash should be 64 hex characters (SHA-256 = 32 bytes = 64 hex)
	require.Len(t, hash1, 64)

	// Hash should be valid hex
	for _, c := range hash1 {
		require.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"hash should be lowercase hex")
	}
}
