// cmd/code-indexer/init.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initCmd = &cobra.Command{
	Use:   "init [repo-path]",
	Short: "Initialize indexing configuration for a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	repoPath := args[0]

	// Resolve to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if repo exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	}

	// Check for existing config
	configPath := filepath.Join(absPath, ".ai-devtools.yaml")
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		return nil
	}

	// Detect project type and create config
	repoName := filepath.Base(absPath)
	defaultBranch := detectDefaultBranch(absPath)

	config := map[string]interface{}{
		"code-index": map[string]interface{}{
			"name":           repoName,
			"default_branch": defaultBranch,
			"include":        detectIncludes(absPath),
			"exclude":        []string{},
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Review and customize the config file\n")
	fmt.Printf("  2. Run: code-indexer index %s\n", repoName)

	return nil
}

func detectDefaultBranch(repoPath string) string {
	// Try to read from git
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	if data, err := os.ReadFile(headPath); err == nil {
		// Parse "ref: refs/heads/main" or similar
		content := string(data)
		if strings.HasPrefix(content, "ref: refs/heads/") {
			branch := strings.TrimPrefix(content, "ref: refs/heads/")
			return strings.TrimSpace(branch)
		}
	}
	return "main"
}

func detectIncludes(repoPath string) []string {
	includes := []string{}

	// Check for Python
	if hasFiles(repoPath, "*.py") {
		includes = append(includes, "**/*.py")
	}

	// Check for Go
	if hasFiles(repoPath, "*.go") {
		includes = append(includes, "**/*.go")
	}

	// Check for JavaScript/TypeScript
	if hasFiles(repoPath, "*.js") || hasFiles(repoPath, "*.ts") {
		includes = append(includes, "**/*.js", "**/*.ts", "**/*.jsx", "**/*.tsx")
	}

	if len(includes) == 0 {
		// Default
		includes = []string{"**/*.py", "**/*.go", "**/*.js", "**/*.ts"}
	}

	return includes
}

func hasFiles(dir string, pattern string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, pattern))
	if len(matches) > 0 {
		return true
	}
	// Check one level down
	matches, _ = filepath.Glob(filepath.Join(dir, "*", pattern))
	return len(matches) > 0
}
