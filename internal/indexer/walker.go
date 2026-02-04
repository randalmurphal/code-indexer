// Package indexer provides the file walker and indexing pipeline.
package indexer

import (
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

// Walker traverses directories respecting include/exclude patterns.
type Walker struct {
	includes []string
	excludes []string
}

// NewWalker creates a new file walker with the given include and exclude patterns.
// If no includes are specified, defaults to common code file extensions.
func NewWalker(includes, excludes []string) *Walker {
	if len(includes) == 0 {
		includes = []string{
			"**/*.py",
			"**/*.js",
			"**/*.ts",
			"**/*.tsx",
			"**/*.jsx",
			"**/*.go",
		}
	}

	// Default excludes for common non-source directories
	defaultExcludes := []string{
		"**/.git/**",
		"**/__pycache__/**",
		"**/*.pyc",
		"**/node_modules/**",
		"**/venv/**",
		"**/.venv/**",
		"**/dist/**",
		"**/build/**",
		"**/.idea/**",
		"**/.vscode/**",
		"**/*.min.js",
		"**/*.bundle.js",
	}
	excludes = append(defaultExcludes, excludes...)

	return &Walker{
		includes: includes,
		excludes: excludes,
	}
}

// Walk traverses the directory tree rooted at root, calling fn for each file
// that matches the include patterns and does not match the exclude patterns.
func (w *Walker) Walk(root string, fn func(path string) error) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// Normalize to forward slashes for pattern matching
		relPath = filepath.ToSlash(relPath)

		if d.IsDir() {
			// Check if directory should be excluded
			if w.shouldExcludeDir(relPath) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check excludes first
		if w.isExcluded(relPath) {
			return nil
		}

		// Check includes
		if w.isIncluded(relPath) {
			return fn(path)
		}

		return nil
	})
}

func (w *Walker) shouldExcludeDir(relPath string) bool {
	// Check directory exclusion patterns (with trailing slash)
	dirPath := relPath + "/"
	for _, pattern := range w.excludes {
		matched, _ := doublestar.Match(pattern, dirPath)
		if matched {
			return true
		}
		// Also check if the dir itself matches (e.g., "**/.git/**" should match ".git")
		matched, _ = doublestar.Match(pattern, relPath)
		if matched {
			return true
		}
	}
	return false
}

func (w *Walker) isExcluded(relPath string) bool {
	for _, pattern := range w.excludes {
		matched, _ := doublestar.Match(pattern, relPath)
		if matched {
			return true
		}
	}
	return false
}

func (w *Walker) isIncluded(relPath string) bool {
	for _, pattern := range w.includes {
		matched, _ := doublestar.Match(pattern, relPath)
		if matched {
			return true
		}
	}
	return false
}
